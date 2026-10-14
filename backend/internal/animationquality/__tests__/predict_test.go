package animationquality_test

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	q "github.com/MIEnchating/sub2api-console/backend/internal/animationquality"
)

// Synthetic weights only exercise the contract; this is not a trained model.
func model() q.Model {
	return q.Model{SchemaVersion: 1, ModelVersion: "test-only", Benchmark: "pelican-v1", PromptSHA256: q.Hash(q.PelicanPrompt), PreprocessVersion: "sklearn-char-v1", SmoothIDF: true, Analyzer: "char", NgramMin: 3, NgramMax: 5, Norm: "l2", Vocabulary: map[string]int{"中文注": 0, "a b": 1, "a\tb": 2}, IDF: []float64{2, 3, 4}, Weights: []float64{-1, 2, -3}, Intercept: 0.1, PositiveClass: "suspicious", Threshold: 0.5, UnknownMargin: 0.08, TrainingDataVersion: "fixture", TrainingLibraryVersion: "fixture", ValidationReport: "test-only", Accepted: true}
}
func decode(t *testing.T, m q.Model) *q.Detector {
	t.Helper()
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	d, err := q.Decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	return d
}
func TestUnicodeWhitespaceAndRepeatedNgramsUseSparseL2(t *testing.T) {
	d := decode(t, model())
	// Counts [2,3,1], TF-IDF [4,9,4]. Independently calculated decision
	// (-4+18-12)/sqrt(113)+0.1 = 0.28814417367671946.
	got := d.Predict(q.PelicanPrompt, "<html>中文注中文注 a  b a\r\nb a\u001c\u3000b a\tb</html>")
	if got.Score == nil || math.Abs(*got.Score-0.5715417353382543) > 1e-12 || got.Status != "unknown" || got.Reason != "ambiguous_score" {
		t.Fatalf("wrong Unicode/L2 prediction: %+v", got)
	}
}
func TestUnsupportedInputsNeverUseInterceptAsPrediction(t *testing.T) {
	d := decode(t, model())
	for _, tc := range []struct{ name, prompt, source, reason string }{
		{"unseen", q.PelicanPrompt, "<html>xyz</html>", "no_known_features"},
		{"new prompt", "new prompt", "<html>中文注</html>", "unsupported_benchmark"},
		{"old prompt missing", "", "<html>中文注</html>", "prompt_unavailable"},
		{"SVG", q.PelicanPrompt, "<svg>中文注</svg>", "unsupported_format"},
		{"invalid UTF8", q.PelicanPrompt, "<html>\xff</html>", "source_unavailable"},
		{"too large", q.PelicanPrompt, strings.Repeat("x", q.MaxSourceBytes+1), "source_unavailable"},
		{"blank", q.PelicanPrompt, " \n\t", "source_unavailable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := d.Predict(tc.prompt, tc.source)
			if p.Status != "unknown" || p.Score != nil || p.Reason != tc.reason {
				t.Fatalf("%+v", p)
			}
		})
	}
	var absent *q.Detector
	if p := absent.Predict(q.PelicanPrompt, "<html>中文注</html>"); p.Reason != "model_unavailable" || p.Score != nil {
		t.Fatalf("%+v", p)
	}
}
func TestStrictThresholdAndStableSigmoid(t *testing.T) {
	for _, tc := range []struct {
		name                         string
		intercept, threshold, margin float64
		want                         string
	}{
		{"equal lower", 0, 0.6, 0.1, "unknown"}, {"equal upper", 0, 0.4, 0.1, "unknown"}, {"equal threshold", 0, 0.5, 0, "unknown"}, {"positive extreme", 1e6, 0.5, 0.08, "suspicious"}, {"negative extreme", -1e6, 0.5, 0.08, "normal"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := model()
			m.Weights = []float64{0, 0, 0}
			m.Intercept = tc.intercept
			m.Threshold = tc.threshold
			m.UnknownMargin = tc.margin
			p := decode(t, m).Predict(q.PelicanPrompt, "<html>中文注</html>")
			if p.Status != tc.want || p.Score == nil || math.IsNaN(*p.Score) {
				t.Fatalf("%+v", p)
			}
		})
	}
}
func TestModelValidationRejectsBadContracts(t *testing.T) {
	for name, mutate := range map[string]func(*q.Model){
		"index gap": func(m *q.Model) { m.Vocabulary["a b"] = 3 }, "duplicate index": func(m *q.Model) { m.Vocabulary["a b"] = 0 }, "weights": func(m *q.Model) { m.Weights = nil }, "idf": func(m *q.Model) { m.IDF[0] = 0 }, "class": func(m *q.Model) { m.PositiveClass = "normal" }, "not accepted": func(m *q.Model) { m.Accepted = false }, "report": func(m *q.Model) { m.ValidationReport = "" }, "prompt": func(m *q.Model) { m.PromptSHA256 = "wrong" }, "preprocess": func(m *q.Model) { m.Lowercase = true }, "threshold": func(m *q.Model) { m.UnknownMargin = 0.6 }, "overflow": func(m *q.Model) { m.Weights[0] = 1e9 },
	} {
		t.Run(name, func(t *testing.T) {
			m := model()
			mutate(&m)
			raw, _ := json.Marshal(m)
			if _, err := q.Decode(raw); err == nil {
				t.Fatal("accepted invalid model")
			}
		})
	}
	raw, _ := json.Marshal(model())
	if _, err := q.Decode(append(raw, []byte(` {}`)...)); err == nil {
		t.Fatal("accepted trailing JSON")
	}
}
func TestLoadedModelIsImmutableDuringConcurrentInference(t *testing.T) {
	path := filepath.Join(t.TempDir(), "model.json")
	raw, _ := json.Marshal(model())
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	d, err := q.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(path); err != nil {
		t.Fatal(err)
	}
	want := d.Predict(q.PelicanPrompt, "<html>中文注</html>")
	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			for range 20 {
				p := d.Predict(q.PelicanPrompt, "<html>中文注</html>")
				if p.Status != want.Status || *p.Score != *want.Score || p.ModelSHA256 != q.Hash(string(raw)) {
					t.Error("prediction changed")
				}
			}
		})
	}
	wg.Wait()
	if _, err = q.Load(path); err == nil {
		t.Fatal("missing configured file accepted")
	}
	if d, err = q.Load(""); d != nil || err != nil {
		t.Fatal("optional model failed")
	}
}
