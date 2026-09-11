package routing_test

import (
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/routing"
)

func TestSuccessfulZeroTokenTrafficLowersHealthAndDoesNotRecover(t *testing.T) {
	health, err := routing.HealthScore([]routing.Sample{
		{Result: "通过", Source: "traffic", Payload: map[string]any{"input_tokens": 0, "output_tokens": 0, "first_token_ms": 1800}},
		{Result: "通过", Source: "traffic"},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if health.LatestEvent != "empty_response" || health.HealthScore != 70 || health.FailureStreak != 1 || health.RecoveryPassStreak != 0 || health.Fatal {
		t.Fatalf("zero-token success must lower health without fatal veto: %+v", health)
	}
	if health.P95MS != nil {
		t.Fatal("empty response polluted successful latency statistics")
	}
}

func TestEmptyResponseRequiresExplicitZeroUsageAndPreservesOtherFailures(t *testing.T) {
	for _, tc := range []struct {
		name, source, result string
		payload              map[string]any
		event                routing.Event
	}{
		{"missing usage", "traffic", "通过", nil, routing.EventHealthy},
		{"missing output", "traffic", "通过", map[string]any{"input_tokens": 0}, routing.EventHealthy},
		{"null input", "traffic", "通过", map[string]any{"input_tokens": nil, "output_tokens": 0}, routing.EventHealthy},
		{"invalid input", "traffic", "通过", map[string]any{"input_tokens": -1, "output_tokens": 0}, routing.EventHealthy},
		{"input only", "traffic", "通过", map[string]any{"input_tokens": 10, "output_tokens": 0}, routing.EventHealthy},
		{"normal output", "traffic", "通过", map[string]any{"input_tokens": 0, "output_tokens": 5}, routing.EventHealthy},
		{"cache hit", "traffic", "通过", map[string]any{"input_tokens": 0, "output_tokens": 0, "cache_read_tokens": 10}, routing.EventHealthy},
		{"cache creation", "traffic", "通过", map[string]any{"input_tokens": 0, "output_tokens": 0, "cache_creation_tokens": 10}, routing.EventHealthy},
		{"image output", "traffic", "通过", map[string]any{"input_tokens": 0, "output_tokens": 0, "image_count": 1}, routing.EventHealthy},
		{"probe", "active-probe", "通过", map[string]any{"input_tokens": 0, "output_tokens": 0}, routing.EventHealthy},
		{"gateway failure", "traffic", "失败", map[string]any{"input_tokens": 0, "output_tokens": 0, "status_code": 502}, routing.EventGateway},
		{"string counters", "logs", "通过", map[string]any{"input_tokens": "0", "output_tokens": "0"}, "empty_response"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := routing.ClassifySample(routing.Sample{Result: tc.result, Source: tc.source, Payload: tc.payload}, nil)
			if err != nil || got.Event != tc.event {
				t.Fatalf("classified=%+v err=%v", got, err)
			}
		})
	}
}

func TestEmptyResponseUsesConfiguredScore(t *testing.T) {
	got, err := routing.ClassifySample(routing.Sample{Result: "通过", Source: "traffic", Payload: map[string]any{"input_tokens": 0, "output_tokens": 0}}, map[string]any{"scoring": map[string]any{"event_scores": map[string]any{"empty_response": 35}}})
	if err != nil || got.Score != 35 || !got.Failure {
		t.Fatalf("classified=%+v err=%v", got, err)
	}
}
