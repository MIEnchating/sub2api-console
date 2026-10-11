package onboarding_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/onboarding"
)

func TestProbeTaskDoesNotExposeKeyEchoedOutsideCredentialFields(t *testing.T) {
	for _, scenario := range []struct {
		name, action, mode, body, status string
		httpStatus                       int
	}{
		{"JSON output", "probe", "default", `{"model":"gpt-test","output_text":"pong ` + testKey + `","usage":{"output_tokens":2}}`, "succeeded", 200},
		{"escaped JSON output", "probe", "default", `{"model":"gpt-test","output_text":"pong ` + strings.ReplaceAll(testKey, "-", `\u002d`) + `"}`, "succeeded", 200},
		{"output truncation", "probe", "default", `{"model":"gpt-test","output_text":"pong ` + strings.Repeat("x", 1983) + testKey + `"}`, "succeeded", 200},
		{"credential field name", "probe", "default", `{"model":"gpt-test","output_text":"pong","` + testKey + `_token":"private"}`, "succeeded", 200},
		{"stream output", "probe", "stream", "data: {\"type\":\"response.output_text.delta\",\"delta\":\"pong " + testKey + "\",\"model\":\"gpt-test\"}\n\ndata: [DONE]\n\n", "succeeded", 200},
		{"business failure", "probe", "default", `{"error":{"message":"quota rejected ` + testKey + `"}}`, "failed", 200},
		{"HTTP failure", "probe", "default", "request rejected " + testKey, "failed", 403},
		{"escaped HTTP error", "probe", "default", `{"error":{"message":"request rejected ` + strings.ReplaceAll(testKey, "-", `\u002d`) + `"}}`, "failed", 403},
		{"error truncation", "probe", "default", `{"error":{"message":"rejected ` + strings.Repeat("x", 282) + testKey + `"}}`, "failed", 200},
		{"model list failure", "models", "", `{"error":{"message":"model list rejected ` + testKey + `"}}`, "failed", 403},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer "+testKey {
					t.Error("probe did not send its bound key")
				}
				w.WriteHeader(scenario.httpStatus)
				_, _ = w.Write([]byte(scenario.body))
			}))
			t.Cleanup(gateway.Close)
			tasks, runner := &modelOptionsTasks{}, &modelOptionsRunner{}
			service := onboarding.New(probeRepository{}, probePrivate{baseURL: gateway.URL}, &keyClient{}, tasks)
			service.UseTaskRunner(runner)
			if _, err := service.EnqueueProbe(t.Context(), scenario.action, "upstream.test", "6", "gpt-test", scenario.mode); err != nil {
				t.Fatal(err)
			}

			runner.run(t.Context())

			if tasks.final.Status != scenario.status {
				t.Fatalf("status=%s want=%s", tasks.final.Status, scenario.status)
			}
			raw, err := json.Marshal(tasks.final)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(raw), "isolated-onb") {
				t.Fatal("public task contains the bound key echoed by the upstream")
			}
			if scenario.status == "succeeded" {
				result := tasks.final.Result["probe_result"].(onboarding.ProbeResult)
				if !strings.Contains(result.ResponseText, "pong") || !json.Valid([]byte(result.ResponseJSON)) {
					t.Fatal("redaction lost ordinary output or valid response JSON")
				}
			} else if !strings.Contains(tasks.final.Message, "rejected") {
				t.Fatal("redaction lost the actionable failure reason")
			}
		})
	}
}
