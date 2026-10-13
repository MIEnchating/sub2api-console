package onboarding_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
	"github.com/MIEnchating/sub2api-console/backend/internal/onboarding"
)

func TestProbeCandyPreservesModelClassifiesAnswerAndCleansKey(t *testing.T) {
	for _, scenario := range []struct{ name, answer, status string }{
		{"correct", "21", "passed"},
		{"wrong", "22", "failed"},
		{"credential echo", testKey, "failed"},
		{"partial stream", "21", "failed"},
		{"upstream failure", "", "failed"},
		{"cancelled", "", "failed"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			var calls atomic.Int32
			gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				var body struct {
					Model, Input string
					Stream       bool
					Reasoning    struct{ Effort string }
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
					return
				}
				if body.Model != "selected-model" || !body.Stream || body.Reasoning.Effort != "medium" || !strings.Contains(body.Input, "圆形苹果") {
					t.Errorf("unexpected candy request: %+v", body)
				}
				if scenario.name == "cancelled" {
					cancel()
					return
				}
				if scenario.name == "upstream failure" {
					w.WriteHeader(http.StatusServiceUnavailable)
					return
				}
				w.Header().Set("Content-Type", "text/event-stream")
				if scenario.name == "partial stream" {
					fmt.Fprint(w, "event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"21\"}\n\n")
					return
				}
				fmt.Fprintf(w, "event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"model\":\"actual-model\",\"output_text\":%q}}\n\n", scenario.answer)
			}))
			t.Cleanup(gateway.Close)
			service, keys, tasks, runner := modelOptionsFixture(t, gateway.URL, false)
			deps := &animationDependencies{}
			checks, err := modelcheck.New(deps, deps, deps, deps)
			if err != nil {
				t.Fatal(err)
			}
			service.UseAnimationService(checks)
			if _, err := service.EnqueueProbe(t.Context(), "probe", "options.test", "6", "selected-model", "candy"); err != nil {
				t.Fatal(err)
			}
			runner.run(ctx)
			if calls.Load() != 1 {
				t.Fatalf("candy must not replay: requests=%d", calls.Load())
			}
			if scenario.name == "cancelled" && tasks.final.Status != "cancelled" {
				t.Fatalf("cancel not preserved: %+v", tasks.final)
			}
			result, ok := tasks.final.Result["probe_result"].(onboarding.ProbeResult)
			if !ok || result.Status != scenario.status || result.RequestModel != "selected-model" {
				t.Fatalf("unexpected result: %+v", tasks.final)
			}
			if scenario.name == "correct" && (result.ResponseText != "21" || result.ActualModel != "actual-model") {
				t.Fatalf("missing answer/model: %+v", result)
			}
			if len(keys.created) != 1 || len(keys.deleted) != 1 {
				t.Fatalf("temporary key not cleaned: %+v", keys)
			}
			raw, _ := json.Marshal(tasks.final)
			if strings.Contains(string(raw), testKey) {
				t.Fatal("credential leaked")
			}
		})
	}
}
