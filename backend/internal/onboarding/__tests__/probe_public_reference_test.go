package onboarding_test

import (
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

func TestUnifiedModelProbeIncludesPublicReferenceAndCleansTemporaryKey(t *testing.T) {
	var calls atomic.Int32
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if strings.HasSuffix(r.URL.Path, "/responses") {
			fmt.Fprint(w, `{"model":"qwen3.7-flash","output_text":"0"}`)
			return
		}
		var body struct {
			MaxTokens int `json:"max_tokens"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.MaxTokens != 1 {
			t.Error("missing automatic public tokenizer probe")
		}
		fmt.Fprint(w, `{"model":"qwen3.7-flash","choices":[{"finish_reason":"length","message":{"content":"a"}}],"usage":{"prompt_tokens":9,"completion_tokens":1}}`)
	}))
	defer gateway.Close()
	service, keys, tasks, runner := modelOptionsFixture(t, gateway.URL, false)
	deps := &animationDependencies{}
	checks, err := modelcheck.New(deps, deps, deps, deps)
	if err != nil {
		t.Fatal(err)
	}
	service.UseAnimationService(checks)
	if _, err := service.EnqueueProbe(t.Context(), "probe", "options.test", "6", "qwen3.7-flash", "verification"); err != nil {
		t.Fatal(err)
	}
	runner.run(t.Context())
	result, ok := tasks.final.Result["probe_result"].(onboarding.ProbeResult)
	if !ok || tasks.final.Status != "succeeded" || result.Status != "completed" || calls.Load() != 29 {
		t.Fatalf("unified probe failed: %s %d %s", tasks.final.Status, calls.Load(), result.Message)
	}
	if len(keys.created) != 1 || len(keys.deleted) != 1 {
		t.Fatal("temporary key lifecycle changed")
	}
	var report modelcheck.VerificationReport
	raw, _ := json.Marshal(result.Verification)
	_ = json.Unmarshal(raw, &report)
	if report.PublicReference == nil || report.PublicReference.Tokenizer == nil || report.Total != 29 || report.CapabilityTotal != 7 {
		t.Fatal("unified reference evidence lost")
	}
}
