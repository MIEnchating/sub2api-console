package accountworkbench_test

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func TestImportReportRedactsCredentialsInsideNestedAndTypedArrays(t *testing.T) {
	f := newImportFixture(t, func(context.Context, string, string, map[string]any, string, int) (map[string]any, error) {
		return map[string]any{"verdict": "INCONCLUSIVE", "access-new-private": "unsafe key", "responses": []any{"echo access-new-private", []string{"rt_new_private"}, []map[string]any{{"authorization": "access-new-private"}}}}, nil
	})
	f.start(t, f.preview(t, importedJSON, true))
	raw, err := json.Marshal(f.await(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, credential := range []string{"access-new-private", "rt_new_private"} {
		if strings.Contains(string(raw), credential) {
			t.Fatal("task report contains an OAuth credential")
		}
	}
}

func TestImportExistingAccountIgnoresTemplateCredentialExtras(t *testing.T) {
	f := newImportFixture(t, nil)
	f.existing()
	_, err := f.service.SaveTemplate(context.Background(), "", accountworkbench.TemplateInput{Name: "Template", Config: configstore.WorkbenchTemplateConfig{"credential_extras": json.RawMessage(`{"model_mapping":{"other":"other-model"}}`)}})
	if err != nil {
		t.Fatal(err)
	}
	f.start(t, f.preview(t, importedJSON, false))
	f.await(t)
	credentials := f.remote.accounts["101"]["credentials"].(map[string]any)
	if !reflect.DeepEqual(credentials["model_mapping"], map[string]any{"gpt-5": "gpt-5.6"}) {
		t.Fatal("existing account's credential configuration was replaced by the new-account template")
	}
}

func TestImportIsolationRequiresInactiveStatusAndEmptyGroups(t *testing.T) {
	for _, field := range []string{"status", "group_ids", "groups"} {
		t.Run(field, func(t *testing.T) {
			checked := false
			f := newImportFixture(t, func(context.Context, string, string, map[string]any, string, int) (map[string]any, error) {
				checked = true
				return map[string]any{"verdict": "SOL_CONSISTENT"}, nil
			})
			f.remote.mutated = func(_ string, account, _ map[string]any) {
				switch field {
				case "status":
					account[field] = "active"
				case "group_ids":
					account[field] = []any{json.Number("7")}
				case "groups":
					account[field] = []any{map[string]any{"id": json.Number("7")}}
				}
			}
			f.start(t, f.preview(t, importedJSON, true))
			task := f.await(t)
			if task.Status == "succeeded" || checked || f.remote.accounts["101"]["schedulable"] != false {
				t.Fatal("unconfirmed isolation advanced to behavioral checking or scheduling")
			}
		})
	}
}

func TestImportTemplateChangeDuringCheckKeepsAccountDisabled(t *testing.T) {
	var f *importFixture
	var template configstore.WorkbenchTemplate
	f = newImportFixture(t, func(ctx context.Context, _, _ string, _ map[string]any, _ string, _ int) (map[string]any, error) {
		_, err := f.service.SaveTemplate(ctx, template.ID, accountworkbench.TemplateInput{Name: "Changed", Revision: template.Revision})
		return map[string]any{"verdict": "SOL_CONSISTENT"}, err
	})
	var err error
	template, err = f.service.SaveTemplate(context.Background(), "", accountworkbench.TemplateInput{Name: "Initial"})
	if err != nil {
		t.Fatal(err)
	}
	f.start(t, f.preview(t, importedJSON, true))
	task := f.await(t)
	if task.Status == "succeeded" || f.remote.accounts["101"]["schedulable"] != false {
		t.Fatal("stale template enabled account")
	}
}

func TestImportPromotionReadbackRejectsIgnoredTemplateGroups(t *testing.T) {
	f := newImportFixture(t, func(context.Context, string, string, map[string]any, string, int) (map[string]any, error) {
		return map[string]any{"verdict": "SOL_CONSISTENT"}, nil
	})
	_, err := f.service.SaveTemplate(context.Background(), "", accountworkbench.TemplateInput{Name: "Template", Config: configstore.WorkbenchTemplateConfig{"group_ids": json.RawMessage(`[7]`)}})
	if err != nil {
		t.Fatal(err)
	}
	f.remote.mutated = func(_ string, account, body map[string]any) {
		if body["status"] == "active" {
			account["group_ids"] = []any{}
		}
	}
	f.start(t, f.preview(t, importedJSON, true))
	task := f.await(t)
	if task.Status == "succeeded" || f.remote.accounts["101"]["schedulable"] != false {
		t.Fatal("promotion enabled account before template settings were confirmed")
	}
}

func TestImportExistingErrorAccountClearsErrorAndActivatesAfterCheck(t *testing.T) {
	f := newImportFixture(t, func(context.Context, string, string, map[string]any, string, int) (map[string]any, error) {
		return map[string]any{"verdict": "SOL_CONSISTENT"}, nil
	})
	f.existing()
	f.remote.accounts["101"]["status"], f.remote.accounts["101"]["error_message"] = "error", "401 expired access token"
	f.start(t, f.preview(t, importedJSON, true))
	task := f.await(t)
	account := f.remote.accounts["101"]
	if task.Status != "succeeded" || account["schedulable"] != true || account["status"] != "active" || (account["error_message"] != nil && account["error_message"] != "") {
		t.Fatal("successful credential repair did not restore a usable account")
	}
}

func TestImportDetectsNameChangeDuringCheckBeforePromotion(t *testing.T) {
	var f *importFixture
	f = newImportFixture(t, func(context.Context, string, string, map[string]any, string, int) (map[string]any, error) {
		f.remote.mu.Lock()
		f.remote.accounts["101"]["name"] = "Edited by another administrator"
		f.remote.mu.Unlock()
		return map[string]any{"verdict": "SOL_CONSISTENT"}, nil
	})
	f.start(t, f.preview(t, importedJSON, true))
	f.await(t)
	if f.remote.accounts["101"]["schedulable"] != false {
		t.Fatal("check result overwrote concurrent account configuration")
	}
}

func TestImportExistingAccountResetsTemporaryUnschedulabilityAfterCheck(t *testing.T) {
	f := newImportFixture(t, func(context.Context, string, string, map[string]any, string, int) (map[string]any, error) {
		return map[string]any{"verdict": "SOL_CONSISTENT"}, nil
	})
	f.existing()
	f.remote.accounts["101"]["temp_unschedulable_reason"] = "access token expired"
	f.remote.accounts["101"]["temp_unschedulable_until"] = "2099-01-01T00:00:00Z"
	f.start(t, f.preview(t, importedJSON, true))
	task := f.await(t)
	if task.Status != "succeeded" || f.remote.accounts["101"]["temp_unschedulable_until"] != nil || f.remote.accounts["101"]["schedulable"] != true {
		t.Fatal("successful credential replacement did not reset temporary scheduling rejection")
	}
}

func TestImportExistingAccountChangedDuringPauseDoesNotOverwriteCredentials(t *testing.T) {
	f := newImportFixture(t, nil)
	f.existing()
	f.remote.mutated = func(path string, account, body map[string]any) {
		if path == "/accounts/101/schedulable" && body["schedulable"] == false {
			credentials := account["credentials"].(map[string]any)
			credentials["custom_pool_flag"] = "new administrator value"
		}
	}
	f.start(t, f.preview(t, importedJSON, false))
	f.await(t)
	credentials := f.remote.accounts["101"]["credentials"].(map[string]any)
	if credentials["access_token"] != "access-old-private" || credentials["custom_pool_flag"] != "new administrator value" {
		t.Fatal("credential update overwrote an account changed during pause")
	}
}
