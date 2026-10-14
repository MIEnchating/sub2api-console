package onboarding_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
	"github.com/MIEnchating/sub2api-console/backend/internal/onboarding"
)

func TestGeminiProbeAnimationAndCandyPreserveSelectedModelAndCleanTemporaryKey(t *testing.T) {
	for _, mode := range []string{"animation", "candy"} {
		t.Run(mode, func(t *testing.T) {
			gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v1beta/models/gemini-selected:streamGenerateContent" || r.URL.Query().Get("alt") != "sse" || r.Header.Get("x-goog-api-key") != testKey {
					t.Errorf("unexpected native probe: %s", r.URL)
				}
				output := "21"
				if mode == "animation" {
					output = "<svg xmlns='http://www.w3.org/2000/svg'><circle r='10'/></svg>"
				}
				fmt.Fprintf(w, "data: {\"modelVersion\":\"gemini-actual\",\"candidates\":[{\"content\":{\"parts\":[{\"text\":%q}]},\"finishReason\":\"STOP\"}]}\n\n", output)
			}))
			t.Cleanup(gateway.Close)
			service, keys, tasks, runner, _, _ := modelOptionsFixtureStores(t, gateway.URL, false, "gemini")
			deps := &animationDependencies{}
			animations, err := modelcheck.New(deps, deps, deps, deps)
			if err != nil {
				t.Fatal(err)
			}
			service.UseAnimationService(animations)
			if _, err := service.EnqueueProbe(t.Context(), "probe", "options.test", "6", "gemini-selected", mode); err != nil {
				t.Fatal(err)
			}
			runner.run(t.Context())
			if tasks.final.Status != "succeeded" {
				t.Fatalf("probe failed: %+v", tasks.final)
			}
			if len(keys.created) != 1 || len(keys.deleted) != 1 || keys.created[0] != keys.deleted[0] {
				t.Fatalf("temporary key not cleaned: %+v", keys)
			}
			result, ok := tasks.final.Result["probe_result"].(onboarding.ProbeResult)
			if !ok || result.Animation == nil || result.Animation.Model != "gemini-selected" || result.Animation.ResponseModel != "gemini-actual" {
				t.Fatalf("model lost: %+v", result)
			}
			raw, _ := json.Marshal(tasks.final)
			if strings.Contains(string(raw), testKey) {
				t.Fatal("key persisted")
			}
		})
	}
}
