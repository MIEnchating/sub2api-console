package accountworkbench_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
	"github.com/MIEnchating/sub2api-console/backend/internal/upstreamsync"
)

func TestImportAppliesTemplateBeforeDetectionAndOnlyEnablesPassingAccounts(t *testing.T) {
	for _, tc := range []struct {
		name, answer, verdict, failure, model string
		status                                int
		transportError                        bool
		existing                              bool
		proxy                                 bool
	}{
		{name: "candy passes", answer: "21", verdict: "MATCH", model: "gpt-6-astra"},
		{name: "platform proxy shared", proxy: true, answer: "21", verdict: "MATCH", model: "custom-model"},
		{name: "existing account preserves groups", existing: true, answer: "21", verdict: "MATCH", model: "gpt-6-astra"},
		{name: "wrong answer stays paused", answer: "20", verdict: "MISMATCH", model: "gpt-6-astra"},
		{name: "explanation stays paused", answer: "答案是21", verdict: "MISMATCH", model: "gpt-6-astra"},
		{name: "network failure stays paused", failure: "无法连接目标接口", transportError: true, model: "gpt-6-astra"},
		{name: "HTTP failure stays paused", failure: "HTTP 429", status: 429, model: "gpt-6-astra"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service, private := fixture(t, `{"data":`+sourceAccount+`}`)
			owner, _ := previewOwner(t, private)
			runner, tasks := runTasks(t, service)
			ctx := context.Background()
			source, err := service.TemplateSource(ctx, "41")
			if err != nil {
				t.Fatal(err)
			}
			library, err := service.SaveTemplate(ctx, accountworkbench.TemplateInput{Name: "导入分组模板", SourceID: "41", SourceVersion: source.SourceVersion})
			if err != nil {
				t.Fatal(err)
			}
			catalog, err := business.Open(filepath.Join(t.TempDir(), "business.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer catalog.Close()
			checker, err := modelcheck.New(tasks, private, catalog, upstreamsync.NewReader(&http.Client{}))
			if err != nil {
				t.Fatal(err)
			}
			var current map[string]any
			var calls atomic.Int32
			checker.UseOAuthTransport(transportFunc(func(req *http.Request) (*http.Response, error) {
				var body struct {
					Model     string `json:"model"`
					Reasoning struct {
						Effort string `json:"effort"`
					} `json:"reasoning"`
					Input []struct {
						Content string `json:"content"`
					} `json:"input"`
				}
				if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				calls.Add(1)
				if body.Model != tc.model {
					t.Errorf("selected model %s replaced with %s", tc.model, body.Model)
				}
				if body.Reasoning.Effort != "medium" || len(body.Input) != 1 || !strings.Contains(body.Input[0].Content, "圆形苹果") {
					t.Error("expected shared candy question with medium reasoning")
				}
				if current == nil || current["schedulable"] != false || !slices.Equal(current["group_ids"].([]any), []any{json.Number("7")}) {
					t.Error("detection ran before isolated import and template groups were confirmed")
				}
				if tc.transportError {
					return nil, errors.New("private network details")
				}
				if tc.status != 0 {
					res := upstreamResponse(map[string]any{"error": "private upstream details"})
					res.StatusCode = tc.status
					return res, nil
				}
				raw, _ := json.Marshal(map[string]any{"type": "response.completed", "response": map[string]any{"status": "completed", "model": tc.model, "output": []any{map[string]any{"type": "message", "content": []any{map[string]any{"type": "output_text", "text": tc.answer}}}}}})
				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader("data: " + string(raw) + "\n\n"))}, nil
			}))
			service.UseExecution(tasks, runner, checker, t.TempDir())
			writes := 0
			if tc.existing {
				current = map[string]any{"id": json.Number("41"), "platform": "openai", "type": "oauth", "status": "active", "schedulable": true, "group_ids": []any{json.Number("9")}, "credentials": map[string]any{"chatgpt_account_id": "run-workspace", "chatgpt_user_id": "run-user"}}
			}
			service.UseTransport(transportFunc(func(req *http.Request) (*http.Response, error) {
				if req.URL.Host != "isolated.invalid" {
					return nil, errors.New("unexpected management host")
				}
				if req.Method == "GET" {
					if tc.proxy && strings.HasSuffix(req.URL.Path, "/proxies/3") {
						return upstreamResponse(map[string]any{"data": platformProxyFixture()}), nil
					}
					if strings.HasSuffix(req.URL.Path, "/accounts") {
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
				decoder := json.NewDecoder(req.Body)
				decoder.UseNumber()
				if err := decoder.Decode(&body); err != nil {
					t.Fatal(err)
				}
				if current == nil {
					current = body
					current["id"] = json.Number("41")
					current["status"] = "active"
					current["notes"] = nil
				} else {
					if req.Method == http.MethodPut {
						if current["schedulable"] != false {
							t.Error("template applied before isolation")
						}
						delete(body, "schedulable") // The real update endpoint does not accept this field.
					}
					if body["schedulable"] == true && !slices.Equal(current["group_ids"].([]any), []any{json.Number("7")}) {
						t.Error("scheduling enabled before template group was applied")
					}
					for key, value := range body {
						current[key] = value
					}
				}
				return upstreamResponse(map[string]any{"data": current}), nil
			}))
			proxyID := ""
			if tc.proxy {
				proxyID = "3"
			}
			preview, err := service.Preview(ctx, owner, accountworkbench.PreviewInput{Action: "import", Content: signedRunInput(t, service), TemplateID: library.PreferredID, Check: true, Promote: true, Model: tc.model, PlatformProxyID: proxyID})
			if err != nil {
				t.Fatal(err)
			}
			run, err := service.Start(ctx, owner, accountworkbench.RunConfirmation{ID: preview.ID, Revision: preview.Revision})
			if err != nil {
				t.Fatal(err)
			}
			awaitRun(t, runner)
			run, err = service.Run(ctx, owner, run.ID)
			if err != nil {
				t.Fatal(err)
			}
			row := run.Items[0]
			if calls.Load() != 1 {
				t.Fatalf("expected one candy request, got %d", calls.Load())
			}
			if tc.proxy && (current["proxy_id"] != json.Number("3") || row.Check["transport"] != "oauth-proxy") {
				t.Fatal("detection and account did not share selected platform proxy")
			}
			public, _ := json.Marshal(run)
			if strings.Contains(string(public), "private network details") || strings.Contains(string(public), "private upstream details") || strings.Contains(string(public), "rt_run-private") {
				t.Fatal("private response or credentials exposed")
			}
			if row.AccountID != "41" || !slices.Equal(current["group_ids"].([]any), []any{json.Number("7")}) {
				t.Fatal("import did not preserve account ID and template groups")
			}
			if tc.failure != "" || tc.verdict != "MATCH" {
				if run.Status != "needs_attention" || row.Status != "review" || current["schedulable"] != false {
					t.Fatalf("failed detection enabled account: status=%s message=%s", row.Status, row.Message)
				}
				if tc.failure != "" && (!strings.Contains(row.Message, tc.failure) || row.Check["verdict"] != "ERROR") {
					t.Fatalf("detection failure reason lost: %s", row.Message)
				}
				if tc.answer == "答案是21" {
					beforeWrites := writes
					if _, err := service.Retry(ctx, owner, accountworkbench.RunConfirmation{ID: run.ID, Revision: run.Revision}); err != nil {
						t.Fatal(err)
					}
					awaitRun(t, runner)
					run, err = service.Run(ctx, owner, run.ID)
					if err != nil {
						t.Fatal(err)
					}
					if writes != beforeWrites || run.Items[0].Status != "review" {
						t.Fatal("retry rewrote imported account instead of repeating detection")
					}
					row = run.Items[0]
					current["priority"] = json.Number("99")
					_, err = service.EnableReview(ctx, owner, accountworkbench.RunConfirmation{ID: run.ID, Revision: run.Revision}, []string{row.ID})
					if err != nil {
						t.Fatal(err)
					}
					awaitRun(t, runner)
					run, err = service.Run(ctx, owner, run.ID)
					if err != nil {
						t.Fatal(err)
					}
					if writes != beforeWrites || current["schedulable"] != false || run.Items[0].Status != "failed" {
						t.Fatal("manual enable overwrote changed account")
					}
					return
				}
				before, _ := json.Marshal(row.Check)
				if _, err := service.EnableReview(ctx, owner, accountworkbench.RunConfirmation{ID: run.ID, Revision: run.Revision}, []string{row.ID}); err != nil {
					t.Fatal(err)
				}
				awaitRun(t, runner)
				manual, err := service.Run(ctx, owner, run.ID)
				if err != nil {
					t.Fatal(err)
				}
				after, _ := json.Marshal(manual.Items[0].Check)
				if manual.Status != "completed" || !manual.Items[0].ManualEnabled || current["schedulable"] != true || string(before) != string(after) {
					t.Fatal("manual enable did not preserve the failed detection result")
				}
				return
			}
			if run.Status != "completed" || row.Check["verdict"] != tc.verdict || current["schedulable"] != true {
				t.Fatalf("passing detection did not enable account: status=%s message=%s", run.Status, row.Message)
			}
		})
	}
}
