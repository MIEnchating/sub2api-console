package accountworkbench_test

import (
	"context"
	"encoding/json"
	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"testing"
)

func TestManualTemplateSaveEditAndImportPreviewWithoutSourceAccount(t *testing.T) {
	service, store := fixture(t, `{"data":[]}`)
	var input accountworkbench.TemplateInput
	if err := json.Unmarshal([]byte(`{"name":"手动配置","config":{"concurrency":0,"priority":50,"rate_multiplier":"0.1234567890123456789","group_ids":["7"],"auto_pause_on_expired":true,"credential_extras":{"model_mapping":{"gpt-5":"gpt-5.6"},"plan_type":"plus"},"extra":{"codex_fingerprint_mode":"session"}}}`), &input); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	library, err := service.SaveTemplate(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	item := library.Items[0]
	if item.SourceID != "" || item.Config.RateMultiplier != "0.1234567890123456789" || item.Config.Concurrency != 0 || len(item.Summary.Groups) != 1 {
		t.Fatal("manual configuration lost")
	}
	input.ID, input.Revision, input.Name = item.ID, library.Revision, "更新配置"
	updated, err := service.SaveTemplate(ctx, input)
	if err != nil || updated.Items[0].Revision != 2 {
		t.Fatalf("edit: %v", err)
	}
	if _, err := service.SaveTemplate(ctx, input); err == nil {
		t.Fatal("stale version accepted")
	}
	owner, _ := previewOwner(t, store)
	preview, err := service.Preview(ctx, owner, accountworkbench.PreviewInput{Content: "rt_test-manual", Action: "import", TemplateID: item.ID, Model: "gpt-5.6-sol"})
	if err != nil || preview.Template == nil || preview.Template.Config.Concurrency != 0 {
		t.Fatalf("preview: %v", err)
	}
}

func TestManualTemplateRejectsUnsafeOrAmbiguousConfig(t *testing.T) {
	for _, payload := range []string{
		`{"source_id":"41","source_version":"v1","config":{"rate_multiplier":"1"}}`,
		`{"config":{"rate_multiplier":"-1"}}`,
		`{"config":{"rate_multiplier":"1","group_ids":["0"]}}`,
		`{"config":{"rate_multiplier":"1","credential_extras":{"access_token":"secret"}}}`,
		`{"config":{"rate_multiplier":"1","extra":{"password":"secret"}}}`,
	} {
		t.Run(payload, func(t *testing.T) {
			service, _ := fixture(t, `{"data":[]}`)
			var input accountworkbench.TemplateInput
			if err := json.Unmarshal([]byte(payload), &input); err != nil {
				t.Fatal(err)
			}
			input.Name = "无效配置"
			if _, err := service.SaveTemplate(context.Background(), input); err == nil {
				t.Fatal("invalid manual config accepted")
			}
			library, err := service.Templates(context.Background())
			if err != nil || len(library.Items) != 0 {
				t.Fatal("failed save changed library")
			}
		})
	}
}
