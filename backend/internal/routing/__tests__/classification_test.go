package routing_test

import (
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/routing"
)

func TestAccessDenialWithoutCredentialEvidenceIsNotFatal(t *testing.T) {
	for _, test := range []struct {
		name   string
		status int
		reason string
		event  routing.Event
	}{
		{"wrapped upstream forbidden remains a gateway failure", 502, "Upstream access forbidden, please contact administrator", routing.EventGateway},
		{"wrapped permission denial remains a gateway failure", 502, "Permission denied for this model", routing.EventGateway},
		{"wrapped access denial remains a gateway failure", 502, "Access denied for this model", routing.EventGateway},
		{"forbidden without a status remains an upstream failure", 0, "Upstream access forbidden, please contact administrator", routing.EventUnknown},
		{"direct forbidden remains a neutral traffic error", 403, "Forbidden", routing.EventClientError},
	} {
		t.Run(test.name, func(t *testing.T) {
			// Persisted policies can still include broad access-denial patterns.
			policy := map[string]any{"classify": map[string]any{
				"fatal_patterns": []any{"forbidden", "permission denied", "access denied"},
			}}
			sample := routing.Sample{Result: "失败", Source: "traffic", FailureReason: test.reason}
			if test.status != 0 {
				sample.StatusCode = &test.status
			}
			classified, err := routing.ClassifySample(sample, policy)
			if err != nil {
				t.Fatal(err)
			}
			if classified.Fatal || classified.Event != test.event {
				t.Fatalf("access denial misclassified: %+v", classified)
			}
			if test.status == 502 && (!classified.Gateway || !classified.Failure || classified.Neutral) {
				t.Fatalf("wrapped denial must retain gateway failure evidence: %+v", classified)
			}
		})
	}
}

func TestExplicitCredentialEvidenceStillTriggersFatalClassification(t *testing.T) {
	for _, test := range []struct {
		name   string
		status int
		reason string
	}{
		{"401 remains fatal despite generic forbidden text", 401, "Forbidden"},
		{"wrapped authentication failure remains fatal", 502, "Upstream authentication failed, please contact administrator"},
		{"invalid key remains fatal alongside forbidden", 502, "Forbidden: invalid api key"},
		{"direct forbidden with a revoked key remains fatal", 403, "Forbidden: key revoked"},
		{"custom credential failure pattern remains fatal", 502, "tenant credential rejected"},
	} {
		t.Run(test.name, func(t *testing.T) {
			classified, err := routing.ClassifySample(routing.Sample{
				Result: "失败", Source: "traffic", StatusCode: &test.status, FailureReason: test.reason,
			}, map[string]any{"classify": map[string]any{"fatal_patterns": []any{"tenant credential rejected"}}})
			if err != nil {
				t.Fatal(err)
			}
			if !classified.Fatal || classified.Event != routing.EventCredentialBad {
				t.Fatalf("credential failure lost fatal classification: %+v", classified)
			}
		})
	}
}

func TestWrappedForbiddenWithDefaultPolicyLowersHealthWithoutFatalVeto(t *testing.T) {
	status := 502
	health, err := routing.HealthScore([]routing.Sample{
		{Result: "失败", Source: "traffic", StatusCode: &status, FailureReason: "Upstream access forbidden, please contact administrator"},
		{Result: "通过", Source: "traffic"},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if health.Fatal || health.LatestEvent != routing.EventGateway || health.HealthScore != 62.5 || health.GatewayFailures != 1 {
		t.Fatalf("wrapped forbidden must lower health as a gateway failure: %+v", health)
	}
}
