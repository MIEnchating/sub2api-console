package accountworkbench_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
	"github.com/MIEnchating/sub2api-console/backend/internal/upstreamsync"
)

func TestImportChecksBeforeWritingAndDoesNotGateOnBehavioralVerdict(t *testing.T) {
	for _, tc := range []struct {
		name, answer, verdict, failure, model string
		weights                               []float64
		status                                int
		transportError                        bool
		partialError                          bool
	}{
		{name: "Sol match", answer: "7", verdict: "SOL_CONSISTENT", weights: []float64{0, -10, -10}, model: "gpt-5.6-sol"},
		{name: "Luna like still imports", answer: "7", verdict: "LUNA_LIKE", weights: []float64{-10, 0, -10}, model: "gpt-5.6-luna"},
		{name: "insufficient evidence still imports", answer: "9", verdict: "INCONCLUSIVE", weights: []float64{0, -10, -10}, model: "gpt-5.6-sol"},
		{name: "network failure prevents all writes", failure: "无法连接目标接口", transportError: true, weights: []float64{0, -10, -10}, model: "gpt-5.6-sol"},
		{name: "partial request failure prevents import", answer: "7", failure: "无法连接目标接口", partialError: true, weights: []float64{0, -10, -10}, model: "gpt-5.6-sol"},
		{name: "HTTP failure prevents all writes", failure: "HTTP 429", status: 429, weights: []float64{0, -10, -10}, model: "gpt-5.6-sol"},
		{name: "invalid model fails without fallback", failure: "所选模型", weights: []float64{0, -10, -10}, model: "unconfigured-model"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service, private := fixture(t, `{}`)
			owner, _ := previewOwner(t, private)
			runner, tasks := runTasks(t, service)
			catalog, err := business.Open(filepath.Join(t.TempDir(), "business.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer catalog.Close()
			checker, err := modelcheck.New(tasks, private, catalog, upstreamsync.NewReader(&http.Client{}))
			if err != nil {
				t.Fatal(err)
			}
			view := checker.Configuration()
			config := view.Active.Payload
			probe := modelcheck.ProbeDefinition{ID: "seven", Kind: "numeric", Question: "Return seven.", Clusters: []modelcheck.NumericClusterDefinition{{ID: "seven", Center: 7}}, Tolerance: &modelcheck.NumericTolerance{Value: 0, Mode: "absolute"}, Weights: map[string][]float64{"seven": tc.weights}}
			config.SolProfile.Quick = []modelcheck.ProbeDefinition{probe}
			probe.ID = "reserve"
			config.SolProfile.Reserve = []modelcheck.ProbeDefinition{probe}
			draft, err := checker.SaveDraft(context.Background(), modelcheck.SaveDraftRequest{ExpectedFingerprint: view.Active.Fingerprint, Payload: config}, "test")
			if err != nil {
				t.Fatal(err)
			}
			if _, err = checker.PublishDraft(context.Background(), modelcheck.PublishRequest{ExpectedFingerprint: draft.Draft.Fingerprint}, "test"); err != nil {
				t.Fatal(err)
			}
			var checked atomic.Bool
			var calls atomic.Int32
			checker.UseOAuthTransport(transportFunc(func(req *http.Request) (*http.Response, error) {
				var body struct {
					Model string `json:"model"`
				}
				if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if body.Model != tc.model {
					t.Errorf("selected model %s replaced with %s", tc.model, body.Model)
				}
				checked.Store(true)
				if tc.transportError || (tc.partialError && calls.Add(1) == 1) {
					return nil, errors.New("private network details")
				}
				if tc.status != 0 {
					res := upstreamResponse(map[string]any{"error": "private upstream details"})
					res.StatusCode = tc.status
					return res, nil
				}
				raw, _ := json.Marshal(map[string]any{"type": "response.completed", "response": map[string]any{"status": "completed", "model": tc.model, "output": []any{map[string]any{"type": "message", "content": []any{map[string]any{"type": "output_text", "text": "[\"" + tc.answer + "\"]"}}}}}})
				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader("data: " + string(raw) + "\n\n"))}, nil
			}))
			service.UseExecution(tasks, runner, checker, t.TempDir())
			writes := 0
			var current map[string]any
			service.UseTransport(transportFunc(func(req *http.Request) (*http.Response, error) {
				if req.Method == "GET" {
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
				if !checked.Load() {
					t.Error("account written before detection")
				}
				var body map[string]any
				decoder := json.NewDecoder(req.Body)
				decoder.UseNumber()
				if err := decoder.Decode(&body); err != nil {
					t.Fatal(err)
				}
				if current == nil {
					current = body
					current["id"] = json.Number("41")
				} else {
					for key, value := range body {
						current[key] = value
					}
				}
				return upstreamResponse(map[string]any{"data": current}), nil
			}))
			ctx := context.Background()
			preview, err := service.Preview(ctx, owner, accountworkbench.PreviewInput{Action: "import", Content: signedRunInput(t, service), Check: true, Promote: true, Model: tc.model})
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
			public, _ := json.Marshal(run)
			if strings.Contains(string(public), "private network details") || strings.Contains(string(public), "private upstream details") || strings.Contains(string(public), "rt_run-private") {
				t.Fatal("private response or credentials exposed")
			}
			if tc.failure != "" {
				if writes != 0 || row.AccountID != "" || row.Status != "failed" || !strings.Contains(row.Message, tc.failure) || row.Check["verdict"] != "ERROR" {
					t.Fatalf("failure not blocked/explained: writes=%d status=%s message=%s verdict=%v", writes, row.Status, row.Message, row.Check["verdict"])
				}
				if _, err := service.EnableReview(ctx, owner, accountworkbench.RunConfirmation{ID: run.ID, Revision: run.Revision}, []string{row.ID}); err == nil {
					t.Fatal("failed check allowed manual enable")
				}
				return
			}
			if run.Status != "completed" || row.Check["verdict"] != tc.verdict || current["schedulable"] != true {
				t.Fatalf("completed detection prevented import: status=%s verdict=%v message=%s", run.Status, row.Check["verdict"], row.Message)
			}
		})
	}
}
