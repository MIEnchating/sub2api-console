package routing_test

import (
	"fmt"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/routing"
)

func TestCapacityFailuresUseGatewayHealthWithoutCredentialOrQuotaPenalty(t *testing.T) {
	for _, reason := range []string{
		"Our servers are currently overloaded. Please try again later.",
		"The service is busy. Please retry later.",
		"Selected model is at capacity. Please try a different model.",
	} {
		for _, status := range []int{0, 200, 503} {
			t.Run(fmt.Sprintf("%s/status_%d", reason, status), func(t *testing.T) {
				sample := routing.Sample{Result: "失败", Source: "traffic", FailureReason: reason}
				if status != 0 {
					sample.StatusCode = &status
				}
				classified, err := routing.ClassifySample(sample, nil)
				if err != nil {
					t.Fatal(err)
				}
				if classified.Event != routing.EventGateway || !classified.Failure || !classified.Gateway || classified.Fatal || classified.RateLimited || classified.Neutral {
					t.Fatalf("temporary capacity failure misclassified: %+v", classified)
				}
			})
		}
	}
}

func TestCapacityTextDoesNotOverrideExplicitAuthenticationOrQuotaFailure(t *testing.T) {
	for _, scenario := range []struct {
		status int
		event  routing.Event
	}{
		{401, routing.EventCredentialBad},
		{429, routing.EventRateLimited},
	} {
		classified, err := routing.ClassifySample(routing.Sample{
			Result: "失败", Source: "traffic", StatusCode: &scenario.status,
			FailureReason: "The service is busy. Please retry later.",
		}, nil)
		if err != nil {
			t.Fatal(err)
		}
		if classified.Event != scenario.event {
			t.Fatalf("explicit error status lost: %+v", classified)
		}
	}
}

func TestRepeatedCapacityFailuresLowerHealthAndResetRecovery(t *testing.T) {
	health, err := routing.HealthScore([]routing.Sample{
		{Result: "失败", Source: "traffic", FailureReason: "The service is busy. Please retry later."},
		{Result: "失败", Source: "traffic", FailureReason: "Our servers are currently overloaded. Please try again later."},
		{Result: "通过", Source: "traffic"},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if health.LatestEvent != routing.EventGateway || health.GatewayFailures != 2 || health.FailureStreak != 2 || health.RecoveryPassStreak != 0 || health.HealthScore >= 60 || health.Fatal || health.RateLimited != 0 {
		t.Fatalf("capacity failures did not lower health consistently: %+v", health)
	}
}

func TestSuccessfulResultWithCapacityTextDoesNotBecomeHealthFailure(t *testing.T) {
	classified, err := routing.ClassifySample(routing.Sample{
		Result: "通过", Source: "traffic", FailureReason: "The service is busy. Please retry later.",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if classified.Event != routing.EventHealthy || classified.Failure || classified.Fatal || classified.RateLimited {
		t.Fatalf("non-error capacity text changed a successful result: %+v", classified)
	}
}
