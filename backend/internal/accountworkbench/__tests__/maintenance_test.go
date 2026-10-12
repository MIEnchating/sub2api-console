package accountworkbench_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
)

func TestMaintenanceRefreshesStableAccountAndDoesNotReplayUnknownRefresh(t *testing.T) {
	for _, lost := range []bool{false, true} {
		name := "verified refresh clears error and enables"
		if lost {
			name = "unknown refresh is not replayed on next check"
		}
		t.Run(name, func(t *testing.T) {
			service, store := fixture(t, `{}`)
			owner, _ := previewOwner(t, store)
			runner, _ := runTasks(t, service)
			account := map[string]any{"id": json.Number("41"), "name": "maintained", "platform": "openai", "type": "oauth", "status": "error", "schedulable": false, "error_message": "HTTP 401: token expired", "credentials": map[string]any{"chatgpt_account_id": "workspace", "chatgpt_user_id": "user", "email": "owner@example.test", "access_token": "old-access", "refresh_token": "rt_original"}}
			inactive := map[string]any{"id": json.Number("42"), "platform": "openai", "type": "oauth", "status": "inactive", "error_message": "HTTP 401"}
			writes := []string{}
			service.UseTransport(transportFunc(func(request *http.Request) (*http.Response, error) {
				if request.URL.Host != "isolated.invalid" {
					return nil, errors.New("unexpected target")
				}
				path := strings.TrimPrefix(request.URL.Path, "/api/v1")
				if request.Method == "GET" {
					if path == "/admin/accounts" {
						return upstreamResponse(map[string]any{"data": map[string]any{"items": []any{account, inactive}, "total": 2}}), nil
					}
					if path != "/admin/accounts/41" {
						return nil, errors.New("wrong stable account")
					}
					return upstreamResponse(map[string]any{"data": account}), nil
				}
				writes = append(writes, path)
				switch path {
				case "/admin/openai/accounts/41/refresh":
					if lost {
						return nil, errors.New("connection lost")
					}
					credentials := account["credentials"].(map[string]any)
					credentials["access_token"] = "new-access"
					credentials["refresh_token"] = "rt_rotated"
					credentials["expires_at"] = time.Now().Add(time.Hour).Format(time.RFC3339)
				case "/admin/accounts/41/clear-error":
					account["status"] = "active"
					account["error_message"] = ""
				case "/admin/accounts/41/schedulable":
					account["schedulable"] = true
				default:
					return nil, errors.New("unexpected maintenance write")
				}
				return upstreamResponse(map[string]any{"data": account}), nil
			}))
			ctx := context.Background()
			settings := accountworkbench.MaintenanceSettings{Enabled: true, IntervalMinutes: 5, CooldownMinutes: 10, CheckAfterRepair: false, GroupIDs: []string{}}
			preview, err := service.PreviewMaintenance(ctx, owner, settings)
			if err != nil || len(preview.Accounts) != 1 || len(writes) != 0 {
				t.Fatalf("bad preview: %v %+v", err, preview)
			}
			configured, err := service.ConfigureMaintenance(ctx, owner, accountworkbench.RunConfirmation{ID: preview.ID, Revision: preview.Revision})
			if err != nil {
				t.Fatal(err)
			}
			if _, err = service.CheckMaintenance(ctx, owner, configured.Revision); err != nil {
				t.Fatal(err)
			}
			awaitRun(t, runner)
			result, err := service.Maintenance(ctx, owner)
			if err != nil || result.Running || len(result.Results) != 1 {
				t.Fatalf("maintenance incomplete: %v %+v", err, result)
			}
			if lost {
				if result.Results[0].Reason != "refresh_unconfirmed" {
					t.Fatalf("unknown refresh was lost: %+v", result)
				}
				if _, err = service.CheckMaintenance(ctx, owner, result.Revision); err != nil {
					t.Fatal(err)
				}
				awaitRun(t, runner)
				if len(writes) != 1 {
					t.Fatalf("unknown refresh replayed: %v", writes)
				}
			} else {
				if result.Results[0].Status != "repaired" || len(writes) != 3 || account["schedulable"] != true {
					t.Fatalf("maintenance did not recover: %+v %v", result, writes)
				}
			}
			raw, _ := json.Marshal(result)
			if strings.Contains(string(raw), "rt_") || strings.Contains(string(raw), "new-access") {
				t.Fatal("maintenance leaked private credentials")
			}
		})
	}
}

func TestMaintenanceConfirmationRejectsChangedScopeBeforeAnyWrite(t *testing.T) {
	service, store := fixture(t, `{"data":{"items":[{"id":41,"platform":"openai","type":"oauth","status":"active"}],"total":1}}`)
	owner, _ := previewOwner(t, store)
	preview, err := service.PreviewMaintenance(context.Background(), owner, accountworkbench.MaintenanceSettings{Enabled: true, IntervalMinutes: 5, CooldownMinutes: 10})
	if err != nil {
		t.Fatal(err)
	}
	service.UseTransport(transportFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method != "GET" {
			t.Fatal("preview confirmation changed upstream")
		}
		return upstreamResponse(map[string]any{"data": map[string]any{"items": []any{}, "total": 0}}), nil
	}))
	if _, err = service.ConfigureMaintenance(context.Background(), owner, accountworkbench.RunConfirmation{ID: preview.ID, Revision: preview.Revision}); err == nil {
		t.Fatal("changed maintenance scope accepted")
	}
}

func TestMaintenanceRestartPreservesSavedSchedule(t *testing.T) {
	service, store := fixture(t, `{"data":{"items":[],"total":0}}`)
	owner, _ := previewOwner(t, store)
	ctx := context.Background()
	preview, err := service.PreviewMaintenance(ctx, owner, accountworkbench.MaintenanceSettings{Enabled: true, IntervalMinutes: 5, CooldownMinutes: 10})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.ConfigureMaintenance(ctx, owner, accountworkbench.RunConfirmation{ID: preview.ID, Revision: preview.Revision}); err != nil {
		t.Fatal(err)
	}
	if err = service.Recover(ctx); err != nil {
		t.Fatal(err)
	}
	value, err := service.Maintenance(ctx, owner)
	if err != nil || !value.Enabled || value.Running || value.NextCheckAt == nil {
		t.Fatal("restart lost saved maintenance schedule")
	}
}

func TestMaintenanceStatusReadsPublicStateWithoutDecodingPrivateAttempts(t *testing.T) {
	service, store := fixture(t, `{"data":{"items":[],"total":0}}`)
	owner, _ := previewOwner(t, store)
	ctx := t.Context()
	preview, err := service.PreviewMaintenance(ctx, owner, accountworkbench.MaintenanceSettings{Enabled: true, IntervalMinutes: 5, CooldownMinutes: 10})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ConfigureMaintenance(ctx, owner, accountworkbench.RunConfirmation{ID: preview.ID, Revision: preview.Revision}); err != nil {
		t.Fatal(err)
	}
	documents, err := store.WorkbenchDocuments(ctx, "maintenance:", 1)
	if err != nil || len(documents) != 1 {
		t.Fatalf("maintenance document: %v, %d", err, len(documents))
	}
	revision, err := store.SaveWorkbenchDocument(ctx, documents[0].ID, documents[0].Revision, json.RawMessage(`{"public":{"enabled":true,"interval_minutes":5,"cooldown_minutes":10,"results":[{"account_id":"41","check":{"tokens":12345678901234567890}}]},"attempts":"private-invalid"}`))
	if err != nil {
		t.Fatal(err)
	}
	status, err := service.Maintenance(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	if !status.Enabled || status.Revision != revision || len(status.Results) != 1 || status.Results[0].Check["tokens"] != json.Number("12345678901234567890") {
		t.Fatalf("public status lost data: %+v", status)
	}
}

func TestMaintenanceStatusRejectsInvalidPublicState(t *testing.T) {
	service, store := fixture(t, `{"data":{"items":[],"total":0}}`)
	owner, _ := previewOwner(t, store)
	ctx := t.Context()
	preview, err := service.PreviewMaintenance(ctx, owner, accountworkbench.MaintenanceSettings{Enabled: true, IntervalMinutes: 5, CooldownMinutes: 10})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ConfigureMaintenance(ctx, owner, accountworkbench.RunConfirmation{ID: preview.ID, Revision: preview.Revision}); err != nil {
		t.Fatal(err)
	}
	documents, err := store.WorkbenchDocuments(ctx, "maintenance:", 1)
	if err != nil || len(documents) != 1 {
		t.Fatalf("maintenance document: %v, %d", err, len(documents))
	}
	if _, err := store.SaveWorkbenchDocument(ctx, documents[0].ID, documents[0].Revision, json.RawMessage(`{"public":{"results":"invalid"}}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Maintenance(ctx, owner); err == nil || err.Error() != "自动维护配置无法读取" {
		t.Fatalf("invalid public status accepted: %v", err)
	}
}

func TestMaintenanceSavedSettingsSurviveLogoutAndExecuteDueSchedule(t *testing.T) {
	service, store := fixture(t, `{"data":{"items":[],"total":0}}`)
	owner, token := previewOwner(t, store)
	runner, _ := runTasks(t, service)
	ctx := context.Background()
	settings := accountworkbench.MaintenanceSettings{Enabled: true, IntervalMinutes: 17, CooldownMinutes: 8, CheckAfterRepair: false, GroupIDs: []string{"7"}}
	preview, err := service.PreviewMaintenance(ctx, owner, settings)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.ConfigureMaintenance(ctx, owner, accountworkbench.RunConfirmation{ID: preview.ID, Revision: preview.Revision}); err != nil {
		t.Fatal(err)
	}
	if err = store.RevokeSession(ctx, token); err != nil {
		t.Fatal(err)
	}
	if err = service.Recover(ctx); err != nil {
		t.Fatal(err)
	}
	records, err := store.WorkbenchDocuments(ctx, "maintenance:", 1)
	if err != nil || len(records) != 1 {
		t.Fatal(err)
	}
	var document map[string]any
	if err = json.Unmarshal(records[0].Payload, &document); err != nil {
		t.Fatal(err)
	}
	public := document["public"].(map[string]any)
	public["next_check_at"] = time.Now().Add(-time.Minute).UTC().Format(time.RFC3339)
	raw, _ := json.Marshal(document)
	if _, err = store.SaveWorkbenchDocument(ctx, records[0].ID, records[0].Revision, raw); err != nil {
		t.Fatal(err)
	}
	if err = service.TickMaintenance(ctx); err != nil {
		t.Fatal(err)
	}
	awaitRun(t, runner)
	newOwner, _ := previewOwner(t, store)
	value, err := service.Maintenance(ctx, newOwner)
	if err != nil || !value.Enabled || value.Running || value.IntervalMinutes != 17 || value.CooldownMinutes != 8 || len(value.GroupIDs) != 1 || value.GroupIDs[0] != "7" || value.LastCheckAt == nil || value.NextCheckAt == nil {
		t.Fatalf("saved schedule lost: %+v %v", value, err)
	}
	if value.NextCheckAt.Sub(*value.LastCheckAt) != 17*time.Minute {
		t.Fatal("saved interval not used")
	}
	stopped, err := service.StopMaintenance(ctx, newOwner, value.Revision)
	if err != nil || stopped.Enabled {
		t.Fatal("stop failed", err)
	}
	if err = service.Recover(ctx); err != nil {
		t.Fatal(err)
	}
	value, err = service.Maintenance(ctx, newOwner)
	if err != nil || value.Enabled || value.NextCheckAt != nil {
		t.Fatal("manual stop was lost")
	}
}

func TestMaintenanceIntelligenceSwitchUsesSavedModelAndSkipsRateLimits(t *testing.T) {
	for _, tc := range []struct {
		name             string
		enabled, limited bool
		calls            int
	}{
		{"disabled", false, false, 0}, {"enabled", true, false, 1}, {"rate limited", true, true, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service, store := fixture(t, `{}`)
			owner, _ := previewOwner(t, store)
			runner, tasks := runTasks(t, service)
			checker := &maintenanceCandyChecker{t: t}
			service.UseExecution(tasks, runner, checker, t.TempDir())
			ctx := context.Background()
			if _, err := service.SaveImportModel(ctx, accountworkbench.ImportModelSettings{Model: "saved-custom-model"}); err != nil {
				t.Fatal(err)
			}
			account := map[string]any{"id": json.Number("41"), "name": "test", "platform": "openai", "type": "oauth", "status": "active", "schedulable": true, "credentials": map[string]any{"chatgpt_account_id": "workspace", "chatgpt_user_id": "user", "access_token": "test-access"}}
			if tc.limited {
				account["rate_limit_reset_at"] = "2099-01-01T00:00:00Z"
			}
			service.UseTransport(transportFunc(func(req *http.Request) (*http.Response, error) {
				if req.Method != http.MethodGet {
					t.Error("healthy or rate limited account was mutated")
				}
				if strings.HasSuffix(req.URL.Path, "/accounts") {
					return upstreamResponse(map[string]any{"data": map[string]any{"items": []any{account}, "total": 1}}), nil
				}
				return upstreamResponse(map[string]any{"data": account}), nil
			}))
			preview, err := service.PreviewMaintenance(ctx, owner, accountworkbench.MaintenanceSettings{CheckEnabled: tc.enabled, IntervalMinutes: 9, CooldownMinutes: 10})
			if err != nil {
				t.Fatal(err)
			}
			saved, err := service.ConfigureMaintenance(ctx, owner, accountworkbench.RunConfirmation{ID: preview.ID, Revision: preview.Revision})
			if err != nil {
				t.Fatal(err)
			}
			if saved.CheckEnabled != tc.enabled {
				t.Fatal("check switch not saved")
			}
			if _, err = service.CheckMaintenance(ctx, owner, saved.Revision); err != nil {
				t.Fatal(err)
			}
			awaitRun(t, runner)
			result, err := service.Maintenance(ctx, owner)
			if err != nil || len(result.Results) != 1 {
				t.Fatalf("result %v %v", result, err)
			}
			if checker.calls != tc.calls {
				t.Fatalf("calls=%d want=%d", checker.calls, tc.calls)
			}
			if tc.limited && result.Results[0].Reason != "rate_limited" {
				t.Fatal("rate limit reported as healthy")
			}
			if tc.calls > 0 && result.Results[0].Reason != "check_passed" {
				t.Fatal("candy result lost")
			}
		})
	}
}

type maintenanceCandyChecker struct {
	t     *testing.T
	calls int
}

func (c *maintenanceCandyChecker) CheckOAuthCandyWithProxy(_ context.Context, id, name string, credentials map[string]any, model string, timeout int, proxy string) (map[string]any, error) {
	c.calls++
	if id != "41" || model != "saved-custom-model" || credentials["access_token"] != "test-access" {
		c.t.Error("wrong account, model or credentials")
	}
	return map[string]any{"verdict": "MATCH"}, nil
}
func (c *maintenanceCandyChecker) CheckOAuthWithProxy(context.Context, string, string, map[string]any, string, int, string) (map[string]any, error) {
	c.t.Error("legacy Sol check executed")
	return nil, errors.New("unexpected legacy check")
}
