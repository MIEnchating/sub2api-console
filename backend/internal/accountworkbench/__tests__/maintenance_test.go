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

func TestMaintenanceRestartPausesPreviouslyEnabledScheduleUntilNewConfirmation(t *testing.T) {
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
	if err != nil || value.Enabled || value.Running || value.NextCheckAt != nil {
		t.Fatal("restart silently resumed delegated maintenance")
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
