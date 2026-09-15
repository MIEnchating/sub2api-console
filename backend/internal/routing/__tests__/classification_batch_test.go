package routing_test

import (
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/routing"
)

func TestSampleClassifierKeepsConfiguredScoresConsistentAcrossSamples(t *testing.T) {
	scores := map[string]any{"gateway_error": 30, "perfect": 95}
	policy := map[string]any{"scoring": map[string]any{"event_scores": scores}}
	classify, err := routing.NewSampleClassifier(policy)
	if err != nil {
		t.Fatal(err)
	}
	scores["gateway_error"], scores["perfect"] = 10, 80
	for _, sample := range []struct {
		input routing.Sample
		event routing.Event
		score float64
	}{
		{routing.Sample{Result: "失败", FailureReason: "HTTP 502 Bad Gateway", Source: "traffic"}, routing.EventGateway, 30},
		{routing.Sample{Result: "通过", Source: "traffic"}, routing.EventHealthy, 95},
	} {
		got := classify(sample.input)
		if got.Event != sample.event || got.Score != sample.score {
			t.Fatalf("classification changed within the batch: %+v", got)
		}
	}
}

func TestSampleClassifierRejectsInvalidPolicyBeforeClassifying(t *testing.T) {
	classify, err := routing.NewSampleClassifier(map[string]any{"scoring": map[string]any{"short_window": 0}})
	if err == nil || classify != nil {
		t.Fatal("invalid policy returned a usable classifier")
	}
}

func BenchmarkSampleClassificationBatch(b *testing.B) {
	policy := map[string]any{
		"scoring":  map[string]any{"event_scores": map[string]any{"gateway_error": 30}},
		"classify": map[string]any{"fatal_patterns": []any{"invalid api key", "account expired"}},
	}
	sample := routing.Sample{Result: "失败", FailureReason: "HTTP 502 Bad Gateway", Source: "traffic"}
	b.ReportAllocs()
	for b.Loop() {
		classify, err := routing.NewSampleClassifier(policy)
		if err != nil {
			b.Fatal(err)
		}
		for range 1000 {
			classify(sample)
		}
	}
}
