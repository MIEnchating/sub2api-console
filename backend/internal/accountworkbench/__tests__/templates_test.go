package accountworkbench_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

const sourceAccount = `{"id":41,"platform":"openai","type":"oauth","name":"来源","concurrency":0,"priority":10,"rate_multiplier":0.1234567890123456789,"upstream_billing_probe_enabled":true,"confirm_mixed_channel_risk":true,"credentials":{"access_token":"private-token","chatgpt_user_id":"user-one","chatgpt_account_id":"workspace-one","plan_type":"prolite","model_mapping":{"gpt-5":"gpt-5.6"}},"extra":{"codex_fingerprint_mode":"session","password":"private-password"},"group_ids":[7]}`

func TestTemplatePreservesUpstreamRFC3339ExpiryAsWriteTimestamp(t *testing.T) {
	service, _ := fixture(t, `{"data":`+strings.Replace(sourceAccount, `"priority":10`, `"priority":10,"expires_at":"2027-01-01T00:00:00Z"`, 1)+`}`)
	source, err := service.TemplateSource(context.Background(), "41")
	if err != nil {
		t.Fatal(err)
	}
	expected := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC).Unix()
	if source.Config.ExpiresAt == nil || *source.Config.ExpiresAt != expected {
		t.Fatal("upstream expiry lost")
	}
}

func TestTemplateCopiesReferenceConfigurationWithoutCredentials(t *testing.T) {
	service, _ := fixture(t, `{"data":`+sourceAccount+`}`)
	source, err := service.TemplateSource(context.Background(), "41")
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(source.Config)
	var config map[string]any
	_ = json.Unmarshal(raw, &config)
	if config["upstream_billing_probe_enabled"] != true || config["confirm_mixed_channel_risk"] != true {
		t.Fatal("reference account options missing")
	}
	if source.Config.Concurrency != 0 || source.Config.RateMultiplier != "0.1234567890123456789" || string(source.Config.CredentialExtras["plan_type"]) != `"prolite"` || string(source.Config.Extra["codex_fingerprint_mode"]) != `"session"` {
		t.Fatal("template config lost")
	}
	if strings.Contains(string(raw), "private-") || strings.Contains(string(raw), "chatgpt_user_id") {
		t.Fatal("identity leaked into template")
	}
}

func TestTemplateLibraryChecksSourceAndRevisionBeforeChangingSelection(t *testing.T) {
	service, _ := fixture(t, `{"data":`+sourceAccount+`}`)
	ctx := context.Background()
	source, err := service.TemplateSource(ctx, "41")
	if err != nil {
		t.Fatal(err)
	}
	input := accountworkbench.TemplateInput{Name: "常用", SourceID: "41", SourceVersion: source.SourceVersion}
	library, err := service.SaveTemplate(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if len(library.Items) != 1 || library.PreferredID != library.Items[0].ID || library.Revision != 1 {
		t.Fatal("save did not select template")
	}
	if _, err = service.ChangeTemplate(ctx, library.PreferredID, 0, true); !errors.Is(err, configstore.ErrWorkbenchVersion) {
		t.Fatal("stale delete accepted")
	}
	input.Revision = 1
	input.SourceVersion = "stale-source"
	if _, err = service.SaveTemplate(ctx, input); err == nil {
		t.Fatal("stale source accepted")
	}
	read, err := service.Templates(ctx)
	if err != nil || read.Revision != 1 || len(read.Items) != 1 {
		t.Fatal("failed mutation changed library")
	}
	library, err = service.ChangeTemplate(ctx, "", 1, false)
	if err != nil || library.PreferredID != "" {
		t.Fatal("automatic selection failed")
	}
	library, err = service.ChangeTemplate(ctx, library.Items[0].ID, 2, true)
	if err != nil || len(library.Items) != 0 {
		t.Fatal("delete failed")
	}
}

func TestTemplateLibraryDoesNotCrossManagementTargets(t *testing.T) {
	service, store := fixture(t, `{"data":`+sourceAccount+`}`)
	ctx := context.Background()
	source, err := service.TemplateSource(ctx, "41")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.SaveTemplate(ctx, accountworkbench.TemplateInput{Name: "本站", SourceID: "41", SourceVersion: source.SourceVersion}); err != nil {
		t.Fatal(err)
	}
	if err = store.ConfigureTarget(ctx, "https://other.invalid", "test-other", 10); err != nil {
		t.Fatal(err)
	}
	library, err := service.Templates(ctx)
	if err != nil || len(library.Items) != 0 {
		t.Fatal("template crossed management target")
	}
}

func TestTemplateRejectsMalformedOptionalNumericValue(t *testing.T) {
	var account map[string]any
	decoder := json.NewDecoder(strings.NewReader(sourceAccount))
	decoder.UseNumber()
	if err := decoder.Decode(&account); err != nil {
		t.Fatal(err)
	}
	account["rate_multiplier"] = map[string]any{"invalid": true}
	if _, err := accountworkbench.ExtractConfig(account); err == nil {
		t.Fatal("invalid multiplier silently defaulted")
	}
}
