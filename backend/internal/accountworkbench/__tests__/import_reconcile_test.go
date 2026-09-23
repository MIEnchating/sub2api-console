package accountworkbench_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
)

func TestRetryReconcilesInterruptedImportWithoutReplayingWrites(t *testing.T) {
	for _, tc := range []struct {
		name                           string
		existing, deleted, wrongGroups bool
	}{
		{name: "completed creation with template groups is recognized"},
		{name: "completed update is recognized", existing: true},
		{name: "deleted stable ID gives explicit next step", existing: true, deleted: true},
		{name: "creation with wrong groups remains interrupted", wrongGroups: true},
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
			var current map[string]any
			if tc.existing {
				current = map[string]any{"id": json.Number("41"), "platform": "openai", "type": "oauth", "schedulable": false, "credentials": map[string]any{"chatgpt_account_id": "run-workspace", "chatgpt_user_id": "run-user"}}
			}
			unreachable, retrying, writes := false, false, 0
			service.UseTransport(transportFunc(func(r *http.Request) (*http.Response, error) {
				if r.URL.Host != "isolated.invalid" {
					return nil, fmt.Errorf("unexpected host")
				}
				if r.Method == http.MethodGet {
					if unreachable {
						return nil, fmt.Errorf("readback connection unavailable")
					}
					if retrying && tc.deleted && r.URL.Path == "/api/v1/admin/accounts/41" {
						response := upstreamResponse(map[string]any{"reason": "ACCOUNT_NOT_FOUND"})
						response.StatusCode = 404
						return response, nil
					}
					if r.URL.Path == "/api/v1/admin/accounts" {
						items := []any{}
						if current != nil {
							items = append(items, current)
						}
						return upstreamResponse(map[string]any{"data": map[string]any{"items": items, "total": len(items)}}), nil
					}
					return upstreamResponse(map[string]any{"data": current}), nil
				}
				writes++
				var body map[string]any
				decoder := json.NewDecoder(r.Body)
				decoder.UseNumber()
				if err := decoder.Decode(&body); err != nil {
					return nil, err
				}
				current = body
				current["id"] = json.Number("41")
				current["status"] = "active"
				unreachable = true
				return upstreamResponse(map[string]any{"data": current}), nil
			}))
			preview, err := service.Preview(ctx, owner, accountworkbench.PreviewInput{Action: "import", Content: input, TemplateID: library.PreferredID, Promote: true})
			if err != nil {
				t.Fatal(err)
			}
			queued, err := service.Start(ctx, owner, accountworkbench.RunConfirmation{ID: preview.ID, Revision: preview.Revision})
			if err != nil {
				t.Fatal(err)
			}
			awaitRun(t, runner)
			result, err := service.Run(ctx, owner, queued.ID)
			if err != nil || result.Status == "completed" {
				t.Fatalf("readback failure not retained: %v", err)
			}
			unreachable, retrying = false, true
			current["status"], current["schedulable"] = "active", true
			current["group_ids"] = []any{json.Number("7")}
			if tc.wrongGroups {
				current["group_ids"] = []any{}
			}
			before := writes
			_, err = service.Retry(ctx, owner, accountworkbench.RunConfirmation{ID: result.ID, Revision: result.Revision})
			if err != nil {
				t.Fatal(err)
			}
			awaitRun(t, runner)
			result, err = service.Run(ctx, owner, queued.ID)
			if err != nil {
				t.Fatal(err)
			}
			if writes != before {
				t.Fatal("uncertain write was replayed")
			}
			if tc.deleted {
				if result.Status == "completed" || !strings.Contains(result.Items[0].Message, "已不存在") || !strings.Contains(result.Items[0].Message, "重新导入") {
					t.Fatalf("missing deleted-account guidance: %s", result.Items[0].Message)
				}
			} else if tc.wrongGroups {
				if result.Status == "completed" {
					t.Fatal("incorrect template groups accepted")
				}
			} else if result.Status != "completed" {
				t.Fatalf("applied import not reconciled: %s", result.Items[0].Message)
			}
		})
	}
}
