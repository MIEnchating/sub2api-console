package evidence_test

import (
	"context"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/evidence"
)

func TestAutomaticProbeBatchSizeUsesEightWavesOfConfiguredConcurrency(t *testing.T) {
	for _, test := range []struct {
		name   string
		policy map[string]any
		want   int
	}{
		{name: "default concurrency", policy: map[string]any{}, want: 32},
		{name: "single worker", policy: map[string]any{"probe": map[string]any{"concurrency": 1}}, want: 8},
		{name: "sixteen workers", policy: map[string]any{"probe": map[string]any{"concurrency": 16}}, want: 128},
		{name: "maximum concurrency", policy: map[string]any{"probe": map[string]any{"concurrency": 32}}, want: 256},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := evidence.AutomaticProbeBatchSize(test.policy)
			if err != nil || got != test.want {
				t.Fatalf("batch size=%d want=%d err=%v", got, test.want, err)
			}
		})
	}
}

func TestAutomaticProbeBatchSizeRejectsInvalidConcurrency(t *testing.T) {
	for _, concurrency := range []any{0, 33, "sixteen"} {
		_, err := evidence.AutomaticProbeBatchSize(map[string]any{"probe": map[string]any{"concurrency": concurrency}})
		if err == nil {
			t.Fatalf("invalid concurrency accepted: %v", concurrency)
		}
	}
}

func TestNegativeProbeBatchSizeFailsBeforeAccessingAnyTargets(t *testing.T) {
	_, err := evidence.New(nil, nil).Collect(context.Background(), nil, nil, evidence.Options{ProbeBatchSize: -1})
	if err == nil {
		t.Fatal("negative batch size silently removed the automatic limit")
	}
}
