package accountworkbench_test

import (
	"encoding/json"
	"fmt"
	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"net/http"
	"testing"
)

func TestWorkbenchAccountActionsConfirmRemoteBeforeRemovingRemembered(t *testing.T) {
	for _, scenario := range []string{"schedule", "delete-only", "delete-both", "delete-failed", "stale-account", "stale-record", "wrong-identity", "readback-mismatch"} {
		t.Run(scenario, func(t *testing.T) {
			f := importedRememberedFixture(t, false)
			runner, tasks := runTasks(t, f.service)
			writes, deleted := 0, false
			f.service.UseTransport(transportFunc(func(r *http.Request) (*http.Response, error) {
				if r.URL.Host != "isolated.invalid" {
					return nil, fmt.Errorf("unexpected host")
				}
				if r.Method == http.MethodDelete {
					writes++
					if scenario == "delete-failed" {
						return nil, fmt.Errorf("unavailable")
					}
					deleted = true
				}
				if deleted {
					response := upstreamResponse(map[string]any{"message": "absent"})
					response.StatusCode = 404
					return response, nil
				}
				if r.Method == http.MethodPost {
					writes++
					var body map[string]any
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						return nil, err
					}
					if len(body) != 1 || body["schedulable"] != false {
						return nil, fmt.Errorf("unexpected mutation")
					}
					if scenario != "readback-mismatch" {
						f.account["schedulable"] = false
					}
				}
				return upstreamResponse(map[string]any{"data": f.account}), nil
			}))
			preview, err := f.service.AccountActionPreview(t.Context(), f.owner, "41")
			if err != nil {
				t.Fatal(err)
			}
			input := accountworkbench.AccountActionInput{Action: "delete", TargetVersion: preview.TargetVersion, AccountVersion: preview.AccountVersion, DeleteRemembered: true, RememberedRevision: preview.Remembered.Revision}
			switch scenario {
			case "schedule", "readback-mismatch":
				disabled := false
				input.Action = "scheduling"
				input.Schedulable = &disabled
				input.DeleteRemembered = false
				input.RememberedRevision = 0
			case "delete-only":
				input.DeleteRemembered = false
				input.RememberedRevision = 0
			case "stale-account":
				f.account["name"] = "changed"
			case "stale-record":
				input.RememberedRevision++
			case "wrong-identity":
				f.account["credentials"].(map[string]any)["chatgpt_account_id"] = "other-workspace"
				preview, err = f.service.AccountActionPreview(t.Context(), f.owner, "41")
				if err != nil {
					t.Fatal(err)
				}
				input.AccountVersion = preview.AccountVersion
			}
			task, err := f.service.StartAccountAction(t.Context(), f.owner, "41", input, "operator")
			failed := scenario == "delete-failed" || scenario == "stale-account" || scenario == "stale-record" || scenario == "wrong-identity" || scenario == "readback-mismatch"
			if err == nil {
				awaitRun(t, runner)
				result, readErr := tasks.Get(t.Context(), task.ID)
				if readErr != nil {
					t.Fatal(readErr)
				}
				if (result.Status == "succeeded") == failed {
					t.Fatalf("unexpected result: %+v", result)
				}
			} else if !failed {
				t.Fatal(err)
			}
			records, err := f.service.RememberedAccounts(t.Context(), f.owner)
			if err != nil {
				t.Fatal(err)
			}
			wantRecords := 1
			if scenario == "delete-both" {
				wantRecords = 0
			}
			if len(records) != wantRecords {
				t.Fatalf("remembered records=%d want=%d", len(records), wantRecords)
			}
			wantWrites := 1
			if scenario == "stale-account" || scenario == "stale-record" || scenario == "wrong-identity" {
				wantWrites = 0
			}
			if writes != wantWrites {
				t.Fatalf("writes=%d want=%d", writes, wantWrites)
			}
		})
	}
}
