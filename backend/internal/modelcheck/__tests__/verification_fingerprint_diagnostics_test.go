package modelcheck_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
)

func TestVerificationFingerprintExplainsMissingMetadataAndRetainsKnownChanges(t *testing.T) {
	for _, mode := range []string{"missing_model", "missing_model_changed_usage", "baseline_missing", "stream_created_model"} {
		t.Run(mode, func(t *testing.T) {
			counts := map[string]int{}
			f := setup(t, 1, "openai", func(w http.ResponseWriter, r *http.Request) {
				var body struct{ Input string }
				_ = json.NewDecoder(r.Body).Decode(&body)
				counts[body.Input]++
				baseline := strings.Contains(body.Input, "<sample>a</sample>")
				tokens := 99
				if baseline {
					tokens = 59
				}
				if strings.Contains(body.Input, "The quick") && mode == "missing_model_changed_usage" {
					tokens = 436
					if counts[body.Input] > 1 {
						tokens = 79
					}
				}
				response := map[string]any{"output_text": "OK", "usage": map[string]any{"input_tokens": tokens, "output_tokens": 1}}
				if mode == "stream_created_model" {
					fmt.Fprint(w, "data: {\"type\":\"response.created\",\"response\":{\"model\":\"new-model\"}}\n\n")
					raw, _ := json.Marshal(map[string]any{"type": "response.completed", "response": response})
					fmt.Fprintf(w, "data: %s\n\n", raw)
					return
				}
				if mode == "baseline_missing" && !baseline {
					response["model"] = "new-model"
				}
				_ = json.NewEncoder(w).Encode(response)
			})
			runner := &deferredRunner{}
			f.service.UseTaskRunner(runner)
			_, err := f.service.Enqueue(t.Context(), modelcheck.Request{AccountIDs: []string{"1"}, Models: []string{"new-model"}, TimeoutSeconds: 5, Fingerprint: true})
			if err != nil {
				t.Fatal(err)
			}
			runner.runs[0](t.Context())
			row := finished(t, f).Result["tests"].([]map[string]any)[0]
			raw, _ := json.Marshal(row["fingerprint"])
			var report struct {
				Status, Summary string
				Samples         []struct {
					ID, Status, Detail string
					Delta              *int64
				}
			}
			if err := json.Unmarshal(raw, &report); err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "missing_model":
				if report.Status != "unavailable" || !strings.Contains(report.Samples[0].Detail, "首次未返回型号") || !strings.Contains(report.Samples[0].Detail, "复测未返回型号") {
					t.Fatalf("missing exact cause: %s", raw)
				}
			case "missing_model_changed_usage":
				sample := report.Samples[2]
				if sample.Status != "unstable" || report.Status != "unstable" || !strings.Contains(sample.Detail, "436 → 79") || !strings.Contains(sample.Detail, "未返回型号") {
					t.Fatalf("known token changes must survive missing metadata: %s", raw)
				}
			case "baseline_missing":
				sample := report.Samples[1]
				if sample.Status != "unavailable" || sample.Delta != nil || !strings.Contains(sample.Detail, "基准文本") {
					t.Fatalf("stable sample needs usable baseline: %s", raw)
				}
			case "stream_created_model":
				if report.Status != "stable" || report.Samples[1].Delta == nil || *report.Samples[1].Delta != 40 {
					t.Fatalf("model in created event must be preserved: %s", raw)
				}
			}
		})
	}
}
