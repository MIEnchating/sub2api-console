package accountworkbench_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func TestTemplateSourceRejectsAccountResponseWithDifferentStableID(t *testing.T) {
	f := newImportFixture(t, nil)
	f.remote.accounts["101"] = exportAccount("102", "Wrong account")
	if _, err := f.service.TemplateFromAccount(context.Background(), "101"); err == nil {
		t.Fatal("template extraction accepted a different stable account ID")
	}
}

func sourceTemplateInput(source accountworkbench.TemplateSource) accountworkbench.TemplateInput {
	preferred := true
	return accountworkbench.TemplateInput{Name: "Source template", Config: source.Config, Match: source.Match, Priority: source.Priority, SourceAccountID: source.AccountID, SourceRevision: source.SourceRevision, Preferred: &preferred}
}

func TestTemplateSourcePreviewIsReadOnlyAndSavePreservesIdentityAndDecimalPrecision(t *testing.T) {
	f := newImportFixture(t, nil)
	f.remote.accounts["101"] = exportAccount("101", "Source account")
	f.remote.accounts["101"]["credentials"].(map[string]any)["plan_type"] = "plus"
	source, err := f.service.TemplateFromAccount(context.Background(), "101")
	if err != nil {
		t.Fatal(err)
	}
	if source.AccountID != "101" || source.Target != f.server.URL || len(source.SourceRevision) != 64 || source.SyncedAt == "" || source.Match.PlanType != "plus" || source.Priority != 10 {
		t.Fatalf("source metadata incomplete: %#v", source)
	}
	templates, err := f.service.Templates(context.Background())
	if err != nil || len(templates) != 0 {
		t.Fatal("source preview wrote a template")
	}
	saved, err := f.service.SaveTemplate(context.Background(), "", sourceTemplateInput(source))
	if err != nil {
		t.Fatal(err)
	}
	if saved.SourceAccountID != "101" || saved.SourceName != "Source account" || saved.SourceRevision != source.SourceRevision || saved.SourceSyncedAt == "" || saved.TargetURL != f.server.URL || !saved.Preferred {
		t.Fatal("saved template dropped its stable source or preferred state")
	}
	stored, err := f.private.WorkbenchTemplate(context.Background(), f.server.URL, saved.ID)
	if err != nil || string(stored.Config["rate_multiplier"]) != "0.123456789012345678901" {
		t.Fatalf("source decimal precision changed: %v", err)
	}
}

func TestTemplateSourceRefreshRejectsChangedSourceUntilFreshPreviewIsConfirmed(t *testing.T) {
	f := newImportFixture(t, nil)
	f.remote.accounts["101"] = exportAccount("101", "Source account")
	source, err := f.service.TemplateFromAccount(context.Background(), "101")
	if err != nil {
		t.Fatal(err)
	}
	saved, err := f.service.SaveTemplate(context.Background(), "", sourceTemplateInput(source))
	if err != nil {
		t.Fatal(err)
	}
	f.remote.mu.Lock()
	f.remote.accounts["101"]["rate_multiplier"] = json.Number("0.987654321098765432109")
	f.remote.mu.Unlock()
	stale := sourceTemplateInput(source)
	stale.Revision = saved.Revision
	if _, err := f.service.SaveTemplate(context.Background(), saved.ID, stale); err == nil {
		t.Fatal("stale source preview overwrote a template")
	}
	current, err := f.private.WorkbenchTemplate(context.Background(), f.server.URL, saved.ID)
	if err != nil || current.Revision != saved.Revision {
		t.Fatal("rejected source refresh changed template version")
	}
	fresh, err := f.service.TemplateFromAccount(context.Background(), "101")
	if err != nil {
		t.Fatal(err)
	}
	input := sourceTemplateInput(fresh)
	input.Revision = saved.Revision
	updated, err := f.service.SaveTemplate(context.Background(), saved.ID, input)
	if err != nil || updated.Revision != saved.Revision+1 || updated.SourceRevision == saved.SourceRevision {
		t.Fatalf("fresh source refresh failed: %v", err)
	}
	stored, err := f.private.WorkbenchTemplate(context.Background(), f.server.URL, saved.ID)
	if err != nil || string(stored.Config["rate_multiplier"]) != "0.987654321098765432109" {
		t.Fatal("fresh source refresh lost exact decimal configuration")
	}
}

func TestTemplateOrdinaryEditPreservesSourceWithoutFetchingItAgain(t *testing.T) {
	f := newImportFixture(t, nil)
	f.remote.accounts["101"] = exportAccount("101", "Source account")
	source, err := f.service.TemplateFromAccount(context.Background(), "101")
	if err != nil {
		t.Fatal(err)
	}
	saved, err := f.service.SaveTemplate(context.Background(), "", sourceTemplateInput(source))
	if err != nil {
		t.Fatal(err)
	}
	f.remote.mu.Lock()
	delete(f.remote.accounts, "101")
	f.remote.mu.Unlock()
	updated, err := f.service.SaveTemplate(context.Background(), saved.ID, accountworkbench.TemplateInput{Name: "Renamed", Revision: saved.Revision, Config: saved.Config, Match: saved.Match, Priority: saved.Priority})
	if err != nil || updated.SourceAccountID != saved.SourceAccountID || updated.SourceRevision != saved.SourceRevision || updated.SourceSyncedAt != saved.SourceSyncedAt || !updated.Preferred {
		t.Fatalf("ordinary edit changed source state: %v", err)
	}
}

func TestTemplateSourceRefreshCannotOverwriteConcurrentTemplateEdit(t *testing.T) {
	f := newImportFixture(t, nil)
	f.remote.accounts["101"] = exportAccount("101", "Source account")
	source, err := f.service.TemplateFromAccount(context.Background(), "101")
	if err != nil {
		t.Fatal(err)
	}
	saved, err := f.service.SaveTemplate(context.Background(), "", sourceTemplateInput(source))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.SetPreferredTemplate(context.Background(), saved.ID, saved.Revision, false); err != nil {
		t.Fatal(err)
	}
	input := sourceTemplateInput(source)
	input.Revision = saved.Revision
	if _, err := f.service.SaveTemplate(context.Background(), saved.ID, input); !errors.Is(err, configstore.ErrWorkbenchTemplateConflict) {
		t.Fatalf("stale template version overwrote concurrent edit: %v", err)
	}
}

func TestTemplateNewSourceRequiresPreviewAndRejectsCredentialContentInTransferableFields(t *testing.T) {
	f := newImportFixture(t, nil)
	if _, err := f.service.SaveTemplate(context.Background(), "", accountworkbench.TemplateInput{Name: "Unconfirmed", SourceAccountID: "101"}); err == nil {
		t.Fatal("new source binding did not require a preview")
	}
	f.remote.accounts["101"] = exportAccount("101", "Source account")
	f.remote.accounts["101"]["notes"] = "Copied access: export-access-private"
	source, err := f.service.TemplateFromAccount(context.Background(), "101")
	if err == nil || source.Config != nil || strings.Contains(err.Error(), "export-access-private") {
		t.Fatalf("source notes leaked credentials: %v", err)
	}
}

func TestTemplatePreferredDoesNotOverrideExplicitAutomaticMatching(t *testing.T) {
	f := newImportFixture(t, nil)
	preferred := true
	_, err := f.service.SaveTemplate(context.Background(), "", accountworkbench.TemplateInput{Name: "Preferred fallback", Preferred: &preferred, Config: configstore.WorkbenchTemplateConfig{}})
	if err != nil {
		t.Fatal(err)
	}
	rule, err := f.service.SaveTemplate(context.Background(), "", accountworkbench.TemplateInput{Name: "Plus rule", Match: configstore.WorkbenchTemplateMatch{PlanType: "plus"}})
	if err != nil {
		t.Fatal(err)
	}
	templates, err := f.private.WorkbenchTemplates(context.Background(), f.server.URL)
	if err != nil {
		t.Fatal(err)
	}
	matched, err := accountworkbench.MatchTemplate(accountworkbench.InputItem{PlanType: "plus"}, templates, "")
	if err != nil || matched == nil || matched.ID != rule.ID {
		t.Fatal("preferred template overrode automatic plan matching")
	}
}

func TestTemplateSourceNameNeverReturnsSourceOrManagementCredentials(t *testing.T) {
	f := newImportFixture(t, nil)
	f.remote.accounts["101"] = exportAccount("101", "export-access-private rt_export_private test-admin-key")
	source, err := f.service.TemplateFromAccount(context.Background(), "101")
	if err != nil {
		t.Fatal(err)
	}
	public, err := json.Marshal(source)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"export-access-private", "rt_export_private", "test-admin-key"} {
		if strings.Contains(string(public), secret) {
			t.Fatalf("template source leaked %s", secret)
		}
	}
}

func TestTemplateSourceTokenRotationAndRuntimeCountersKeepPreviewValid(t *testing.T) {
	f := newImportFixture(t, nil)
	f.remote.accounts["101"] = exportAccount("101", "Source account")
	source, err := f.service.TemplateFromAccount(context.Background(), "101")
	if err != nil {
		t.Fatal(err)
	}
	f.remote.mu.Lock()
	f.remote.accounts["101"]["total_requests"] = json.Number("999")
	f.remote.accounts["101"]["credentials"].(map[string]any)["access_token"] = "rotated-private"
	f.remote.mu.Unlock()
	updated, err := f.service.TemplateFromAccount(context.Background(), "101")
	if err != nil || updated.SourceRevision != source.SourceRevision {
		t.Fatal("runtime changes invalidated transferable source configuration")
	}
	if _, err := f.service.SaveTemplate(context.Background(), "", sourceTemplateInput(source)); err != nil {
		t.Fatal(err)
	}
}

func TestTemplateSourcePreviewCannotBeReusedAfterManagementIdentityChanges(t *testing.T) {
	f := newImportFixture(t, nil)
	f.remote.accounts["101"] = exportAccount("101", "Source account")
	source, err := f.service.TemplateFromAccount(context.Background(), "101")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.private.ConfigureTarget(context.Background(), f.server.URL, "changed-admin-key", 3); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.SaveTemplate(context.Background(), "", sourceTemplateInput(source)); err == nil {
		t.Fatal("source preview survived management identity replacement")
	}
}

func TestTemplatePreferenceSwitchPreservesSourceAndConfigurationWithoutFetchingSource(t *testing.T) {
	f := newImportFixture(t, nil)
	f.remote.accounts["101"] = exportAccount("101", "Source account")
	source, err := f.service.TemplateFromAccount(context.Background(), "101")
	if err != nil {
		t.Fatal(err)
	}
	saved, err := f.service.SaveTemplate(context.Background(), "", sourceTemplateInput(source))
	if err != nil {
		t.Fatal(err)
	}
	f.remote.mu.Lock()
	delete(f.remote.accounts, "101")
	f.remote.mu.Unlock()
	updated, err := f.service.SetPreferredTemplate(context.Background(), saved.ID, saved.Revision, false)
	if err != nil {
		t.Fatal(err)
	}
	expected := saved
	expected.Revision++
	expected.Preferred = false
	if !reflect.DeepEqual(updated, expected) {
		t.Fatal("preference mutation changed template source or configuration")
	}
}
