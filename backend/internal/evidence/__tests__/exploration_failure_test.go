package evidence_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/evidence"
)

func TestStrictExplorationUnavailableWithFreshTrafficContinuesUsingTraffic(t *testing.T) {
	fixture := newProbeBatchFixture(t, []string{"1"}, []string{"1"}, "1")
	fixture.policy["probe"].(map[string]any)["performance_exploration_enabled"] = true
	accountID := "1"
	result, err := fixture.service.Collect(context.Background(), fixture.policy, fixture.admin, evidence.Options{
		AccountID: &accountID, FetchTraffic: true, ProbesAllowed: true, StrictFallback: true, Now: fixture.now,
	})
	if err != nil {
		t.Fatalf("optional exploration stopped inspection despite fresh traffic: %v", err)
	}
	if result.CollectionFailed || result.TrafficPersisted != 1 || result.ProbesPersisted != 0 || result.EffectiveSource != "traffic" {
		t.Fatalf("fresh traffic was not retained after unavailable exploration: %+v", result)
	}
	if len(result.SourceErrors) != 1 || !strings.Contains(result.SourceErrors[0], "不支持直连探活") {
		t.Fatalf("unavailable exploration reason was lost: %+v", result)
	}
	samples, err := fixture.store.RoutingSamples(context.Background(), &accountID, nil, "active-probe", 10)
	if err != nil || len(samples) != 0 {
		t.Fatalf("unavailable exploration created health evidence: %+v %v", samples, err)
	}
}

func TestStrictExplorationUnavailableWithoutTrafficStillFailsRequiredFallback(t *testing.T) {
	fixture := newProbeBatchFixture(t, []string{"1"}, nil, "1")
	fixture.policy["probe"].(map[string]any)["performance_exploration_enabled"] = true
	accountID := "1"
	result, err := fixture.service.Collect(context.Background(), fixture.policy, fixture.admin, evidence.Options{
		AccountID: &accountID, ProbesAllowed: true, StrictFallback: true, Now: fixture.now,
	})
	if err == nil || !strings.Contains(err.Error(), "不支持直连探活") || !result.CollectionFailed {
		t.Fatalf("missing required health fallback was accepted: result=%+v err=%v", result, err)
	}
}

func TestStrictExplorationUnavailableWithOlderTrafficStillFailsRequiredFallback(t *testing.T) {
	fixture := newProbeBatchFixture(t, []string{"1"}, []string{}, "1")
	fixture.policy["probe"].(map[string]any)["performance_exploration_enabled"] = true
	accountID := "1"
	_, err := fixture.store.PersistTrafficSamples(context.Background(), []business.TrafficSample{{
		AccountID: accountID, GroupName: "codex", Result: "通过", SampleCount: 1, Attempts: 1,
		ObservedAt: fixture.now.Add(-4 * time.Minute).Format(time.RFC3339Nano), EvidenceKey: "older-traffic",
	}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := fixture.service.Collect(context.Background(), fixture.policy, fixture.admin, evidence.Options{
		AccountID: &accountID, ProbesAllowed: true, StrictFallback: true, Now: fixture.now,
	})
	if err == nil || !strings.Contains(err.Error(), "不支持直连探活") {
		t.Fatalf("traffic outside probe freshness window bypassed required fallback: result=%+v err=%v", result, err)
	}
}

func TestStrictExplorationUnavailableWhenFreshTrafficSkipDisabledStillFailsRequiredFallback(t *testing.T) {
	fixture := newProbeBatchFixture(t, []string{"1"}, []string{"1"}, "1")
	fixture.policy["probe"].(map[string]any)["performance_exploration_enabled"] = true
	fixture.policy["probe"].(map[string]any)["skip_when_traffic_fresh"] = false
	accountID := "1"
	result, err := fixture.service.Collect(context.Background(), fixture.policy, fixture.admin, evidence.Options{
		AccountID: &accountID, FetchTraffic: true, ProbesAllowed: true, StrictFallback: true, Now: fixture.now,
	})
	if err == nil || !strings.Contains(err.Error(), "不支持直连探活") {
		t.Fatalf("explicit health probe requirement was bypassed: result=%+v err=%v", result, err)
	}
}
