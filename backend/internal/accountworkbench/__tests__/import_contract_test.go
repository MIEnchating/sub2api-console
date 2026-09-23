package accountworkbench_test

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
)

// Model the actual management API: PUT does not accept schedulable, and
// account creation reports runtime status active independently of scheduling.
func TestImportUsesSchedulingEndpointBeforeUpdatingExistingAccount(t *testing.T) {
	for _, tc := range []struct {
		name                                 string
		promote, ignorePause                 bool
		pauseResponseLost, changedAfterPause bool
	}{
		{name: "existing account applies template group before enabling", promote: true},
		{name: "legacy promote option cannot prevent enabling when detection is disabled"},
		{name: "ignored pause prevents credential and configuration writes", promote: true, ignorePause: true},
		{name: "lost pause response resumes after readback without repeating pause", promote: true, pauseResponseLost: true},
		{name: "configuration changed after uncertain pause prevents continuation", promote: true, pauseResponseLost: true, changedAfterPause: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service, store := fixture(t, `{"data":`+sourceAccount+`}`)
			ctx := context.Background()
			owner, _ := previewOwner(t, store)
			runner, _ := runTasks(t, service)
			input := signedRunInput(t, service)
			source, err := service.TemplateSource(ctx, "41")
			if err != nil {
				t.Fatal(err)
			}
			library, err := service.SaveTemplate(ctx, accountworkbench.TemplateInput{Name: "导入模板", SourceID: "41", SourceVersion: source.SourceVersion})
			if err != nil {
				t.Fatal(err)
			}
			current := map[string]any{"id": json.Number("41"), "platform": "openai", "type": "oauth", "name": "原账号", "status": "active", "schedulable": true, "group_ids": []any{}, "credentials": map[string]any{"chatgpt_account_id": "run-workspace", "chatgpt_user_id": "run-user"}}
			var writes []string
			pauseLost := false
			service.UseTransport(transportFunc(func(r *http.Request) (*http.Response, error) {
				if r.URL.Host != "isolated.invalid" {
					return nil, fmt.Errorf("unexpected host")
				}
				path := strings.TrimPrefix(r.URL.Path, "/api/v1")
				if r.Method == http.MethodGet {
					if path == "/admin/accounts" {
						return upstreamResponse(map[string]any{"data": map[string]any{"items": []any{current}, "total": 1}}), nil
					}
					return upstreamResponse(map[string]any{"data": current}), nil
				}
				var body map[string]any
				decoder := json.NewDecoder(r.Body)
				decoder.UseNumber()
				if err := decoder.Decode(&body); err != nil {
					return nil, err
				}
				writes = append(writes, r.Method+" "+path)
				switch path {
				case "/admin/accounts/41/schedulable":
					if body["schedulable"] == true && !slices.Equal(current["group_ids"].([]any), []any{json.Number("7")}) {
						return nil, fmt.Errorf("enabled before template group applied")
					}
					if !tc.ignorePause {
						current["schedulable"] = body["schedulable"]
					}
					if tc.pauseResponseLost && body["schedulable"] == false && !pauseLost {
						pauseLost = true
						return nil, fmt.Errorf("pause response connection lost")
					}
				case "/admin/accounts/41":
					if current["schedulable"] != false {
						return nil, fmt.Errorf("configuration updated before verified isolation")
					}
					delete(body, "schedulable")
					maps.Copy(current, body)
					current["notes"] = nil
				case "/admin/accounts/41/clear-error":
				default:
					return nil, fmt.Errorf("unexpected write")
				}
				return upstreamResponse(map[string]any{"data": current}), nil
			}))
			preview, err := service.Preview(ctx, owner, accountworkbench.PreviewInput{Action: "import", Content: input, TemplateID: library.PreferredID, Promote: tc.promote})
			if err != nil {
				t.Fatal(err)
			}
			queued, err := service.Start(ctx, owner, accountworkbench.RunConfirmation{ID: preview.ID, Revision: preview.Revision})
			if err != nil {
				t.Fatal(err)
			}
			awaitRun(t, runner)
			result, err := service.Run(ctx, owner, queued.ID)
			if err != nil {
				t.Fatal(err)
			}
			if tc.pauseResponseLost {
				if result.Status == "completed" || len(writes) != 1 {
					t.Fatalf("unconfirmed pause continued writing: %v", writes)
				}
				if tc.changedAfterPause {
					current["name"] = "操作者已修改"
				}
				_, err = service.Retry(ctx, owner, accountworkbench.RunConfirmation{ID: result.ID, Revision: result.Revision})
				if err != nil {
					t.Fatal(err)
				}
				awaitRun(t, runner)
				result, err = service.Run(ctx, owner, queued.ID)
				if err != nil {
					t.Fatal(err)
				}
				if tc.changedAfterPause {
					if result.Status == "completed" || len(writes) != 1 {
						t.Fatal("changed account was overwritten")
					}
					return
				}
				pauses := 0
				for _, write := range writes {
					if write == "POST /admin/accounts/41/schedulable" {
						pauses++
					}
				}
				if pauses != 2 {
					t.Fatalf("pause replayed instead of one pause and one enable: %v", writes)
				}
			}
			if tc.ignorePause {
				if result.Status == "completed" || !strings.Contains(result.Items[0].Message, "停止调度") {
					t.Fatalf("missing actionable pause failure: %s", result.Items[0].Message)
				}
				if !slices.Equal(writes, []string{"POST /admin/accounts/41/schedulable"}) {
					t.Fatalf("unsafe writes: %v", writes)
				}
				return
			}
			if result.Status != "completed" || result.Items[0].ImportAction != "updated" || current["schedulable"] != true {
				t.Fatalf("import failed: status=%s message=%s", result.Status, result.Items[0].Message)
			}
			if len(writes) == 0 || writes[0] != "POST /admin/accounts/41/schedulable" {
				t.Fatalf("pause was not first: %v", writes)
			}
			if !slices.Equal(current["group_ids"].([]any), []any{json.Number("7")}) {
				t.Fatal("template group was lost")
			}
		})
	}
}
