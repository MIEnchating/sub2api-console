package accountworkbench_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func TestMaintenanceExchangeCompletedBeforeBindingChangePersistsOriginalPrivateResultWithoutUpload(t *testing.T) {
	for _, boundary := range []string{"maintenance", "target", "profile", "cancel"} {
		t.Run(boundary, func(t *testing.T) {
			f, config, owner, _ := maintenanceProfileFixture(t, nil)
			fingerprint := queueTargetHash(t, f.importFixture)
			var writes atomic.Int32
			f.service.UseTransport(oauthTransportFunc(func(request *http.Request) (*http.Response, error) {
				if request.Method != http.MethodGet {
					writes.Add(1)
				}
				return http.DefaultTransport.RoundTrip(request)
			}))
			var taskID string
			changed := make(chan error, 1)
			f.service.UseOAuthTransport(oauthTransportFunc(func(*http.Request) (*http.Response, error) {
				var changeErr error
				switch boundary {
				case "maintenance":
					current := config
					current.Model = "changed-during-exchange"
					changeErr = f.private.SaveWorkbenchMaintenance(context.Background(), f.server.URL, current)
				case "target":
					changeErr = f.private.ConfigureTarget(context.Background(), f.server.URL, "changed-target-private", 3)
				case "profile":
					var profiles []configstore.WorkbenchLoginProfile
					profiles, changeErr = f.private.WorkbenchLoginProfiles(context.Background(), fingerprint)
					if changeErr == nil && len(profiles) == 1 {
						var profile configstore.WorkbenchLoginProfile
						profile, changeErr = f.private.WorkbenchLoginProfile(context.Background(), fingerprint, profiles[0].ID)
						if changeErr == nil {
							profile.Login = json.RawMessage(`{"email":"owner@example.com","password":"new-private-password"}`)
							_, changeErr = f.private.SaveWorkbenchLoginProfile(context.Background(), profile)
						}
					}
				case "cancel":
					if !f.runBoundary.CancelTask(taskID) {
						changeErr = fmt.Errorf("maintenance task not cancelled")
					}
				}
				changed <- changeErr
				claims, _ := json.Marshal(map[string]any{"email": "owner@example.com", "https://api.openai.com/auth": map[string]any{"chatgpt_user_id": "user-101", "chatgpt_account_id": "workspace-101"}})
				return oauthResponse(fmt.Sprintf(`{"access_token":"eyJhbGciOiJub25lIn0.%s.private-exchange-result","refresh_token":"rt_result_before_change","token_type":"Bearer"}`, base64.RawURLEncoding.EncodeToString(claims))), nil
			}))
			task, err := f.service.CheckMaintenance(context.Background(), config.Revision, true)
			if err != nil {
				t.Fatal(err)
			}
			taskID = task.ID
			child := f.awaitTask(t, "account-workbench-oauth", "waiting")
			if err := f.service.FinishOAuth(owner, child.ID); err != nil {
				t.Fatal(err)
			}
			f.awaitDone(t, task.ID)
			if err := <-changed; err != nil {
				t.Fatal(err)
			}
			records, err := f.private.WorkbenchMaintenanceExecutions(context.Background(), fingerprint)
			if err != nil || len(records) != 1 || len(records[0].Items) != 1 || records[0].ID != child.ID || records[0].Items[0].OriginalAccountID != "101" || records[0].Maintenance.Revision != config.Revision || writes.Load() != 0 {
				t.Fatalf("verified result was lost or uploaded after %s changed: records=%d writes=%d err=%v", boundary, len(records), writes.Load(), err)
			}
		})
	}
}
