package routing_test

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/routing"
)

func TestProjectHealthNewEvidenceUpdatesScoresWithoutPersistingOrCompounding(t *testing.T) {
	store, db := healthEvidenceStore(t)
	service := routing.NewService(store)
	now := time.Now().UTC()
	_, err := store.PersistTrafficSamples(t.Context(), []business.TrafficSample{{
		AccountID: "41", GroupName: "codex", Result: "通过", EvidenceKey: "success",
		ObservedAt: now.Add(-time.Minute).Format(time.RFC3339Nano), Payload: map[string]any{},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Calculate(t.Context(), routing.Scope{}, true); err != nil {
		t.Fatal(err)
	}
	observedAt := now.Format(time.RFC3339Nano)
	_, err = store.PersistTrafficSamples(t.Context(), []business.TrafficSample{{
		AccountID: "41", GroupName: "codex", Result: "失败", EvidenceKey: "failure",
		ObservedAt: observedAt, Payload: map[string]any{"status_code": 502},
	}})
	if err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"account_health_evaluations", "routing_decisions", "accounts", "cleanup_states", "runtime_events", "app_state"} {
		for _, operation := range []string{"INSERT", "UPDATE", "DELETE"} {
			_, err := db.Exec(fmt.Sprintf(`CREATE TRIGGER reject_projection_%s_%s BEFORE %s ON %s BEGIN SELECT RAISE(ABORT,'health projection attempted a write'); END`, table, operation, operation, table))
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	beforeRead := time.Now().UTC()
	first, err := service.ProjectHealth(t.Context(), routing.Scope{})
	if err != nil {
		t.Fatal(err)
	}
	health, present := first["41"]
	if !present || health.HealthScore == nil || *health.HealthScore >= 100 || health.SampleCount != 2 {
		t.Fatalf("fresh failure did not replace the old score: %+v", health)
	}
	if health.HealthEvidenceAt == nil || *health.HealthEvidenceAt != observedAt {
		t.Fatalf("projection has incorrect evidence time: %+v", health)
	}
	evaluatedAt, err := time.Parse(time.RFC3339Nano, health.HealthEvaluatedAt)
	if err != nil || evaluatedAt.Before(beforeRead) || evaluatedAt.After(time.Now().UTC()) {
		t.Fatalf("invalid projection evaluation time: %q (%v)", health.HealthEvaluatedAt, err)
	}
	second, err := service.ProjectHealth(t.Context(), routing.Scope{})
	if err != nil {
		t.Fatal(err)
	}
	repeated := second["41"]
	repeated.HealthEvaluatedAt = health.HealthEvaluatedAt
	if !reflect.DeepEqual(health, repeated) {
		t.Fatalf("repeated read changed the same health evidence: first=%+v second=%+v", health, repeated)
	}
	var persistedScore float64
	if err := db.QueryRow(`SELECT health_score FROM account_health_evaluations WHERE account_id='41'`).Scan(&persistedScore); err != nil || persistedScore != 100 {
		t.Fatalf("reading modified the persisted score: %g (%v)", persistedScore, err)
	}
}

func TestProjectHealthEvidenceSelectionMatchesScheduling(t *testing.T) {
	for _, test := range []struct {
		name          string
		trafficResult string
		trafficAge    time.Duration
		probeResult   string
		probeReason   string
		probeAge      time.Duration
		fused         bool
		wantScore     float64
	}{
		{name: "new traffic supersedes older failed probe", trafficResult: "通过", trafficAge: time.Minute, probeResult: "失败", probeAge: 2 * time.Minute, wantScore: 100},
		{name: "expired traffic falls back to fresh probe", trafficResult: "失败", trafficAge: 3 * time.Hour, probeResult: "通过", probeAge: time.Minute, wantScore: 100},
		{name: "new fatal probe overrides traffic success", trafficResult: "通过", trafficAge: 2 * time.Minute, probeResult: "失败", probeReason: "invalid api key", probeAge: time.Minute, wantScore: 0},
		{name: "fused account only uses post fuse recovery", trafficResult: "失败", trafficAge: 10 * time.Minute, probeResult: "通过", probeAge: time.Minute, fused: true, wantScore: 100},
	} {
		t.Run(test.name, func(t *testing.T) {
			store, db := healthEvidenceStore(t)
			now := time.Now().UTC()
			_, err := store.PersistTrafficSamples(t.Context(), []business.TrafficSample{{
				AccountID: "41", GroupName: "codex", Result: test.trafficResult, EvidenceKey: "request",
				ObservedAt: now.Add(-test.trafficAge).Format(time.RFC3339Nano), Payload: map[string]any{},
			}})
			if err != nil {
				t.Fatal(err)
			}
			_, err = store.PersistProbeSamples(t.Context(), []business.ProbeSample{{
				AccountID: "41", GroupName: "codex", Result: test.probeResult, FailureReason: &test.probeReason,
				ObservedAt: now.Add(-test.probeAge).Format(time.RFC3339Nano),
			}})
			if err != nil {
				t.Fatal(err)
			}
			if test.fused {
				fusedAt := now.Add(-5 * time.Minute).Format(time.RFC3339Nano)
				_, err = db.Exec(`UPDATE accounts SET schedulable=0,routing_state='fused' WHERE id='41';
					INSERT INTO routing_decisions(account_id,group_name,routing_state,updated_at,payload_json)
					VALUES('41','codex','fused',?,json_object('state_since',?))`, fusedAt, fusedAt)
				if err != nil {
					t.Fatal(err)
				}
			}
			service := routing.NewService(store)
			projected, err := service.ProjectHealth(t.Context(), routing.Scope{})
			if err != nil {
				t.Fatal(err)
			}
			health := projected["41"]
			if health.HealthScore == nil || *health.HealthScore != test.wantScore {
				t.Fatalf("unexpected evidence score: %+v", health)
			}
			scheduled, err := service.Calculate(t.Context(), routing.Scope{}, true)
			if err != nil {
				t.Fatal(err)
			}
			decision := scheduled.AccountDecisions["41"]
			if *health.HealthScore != decision.HealthScore || *health.ShortScore != decision.ShortScore || *health.LongScore != decision.LongScore || health.SampleCount != int64(decision.SampleCount) || *health.ShortSampleCount != int64(decision.ShortSampleCount) || !reflect.DeepEqual(health.TTFBP95MS, decision.TTFBP95MS) {
				t.Fatalf("projection differs from scheduling score: projection=%+v decision=%+v", health, decision)
			}
		})
	}
}

func TestProjectHealthNoFreshSamplesReturnsEmptyScores(t *testing.T) {
	for _, expired := range []bool{false, true} {
		t.Run(fmt.Sprintf("expired_%t", expired), func(t *testing.T) {
			store, _ := healthEvidenceStore(t)
			if expired {
				_, err := store.PersistTrafficSamples(t.Context(), []business.TrafficSample{{
					AccountID: "41", GroupName: "codex", Result: "通过", EvidenceKey: "expired",
					ObservedAt: time.Now().UTC().Add(-48 * time.Hour).Format(time.RFC3339Nano), Payload: map[string]any{},
				}})
				if err != nil {
					t.Fatal(err)
				}
			}
			projected, err := routing.NewService(store).ProjectHealth(t.Context(), routing.Scope{})
			if err != nil {
				t.Fatal(err)
			}
			health, present := projected["41"]
			if !present || health.HealthScore != nil || health.ShortScore != nil || health.LongScore != nil || health.SampleCount != 0 || health.LongSampleCount != 0 || health.ShortSampleCount == nil || *health.ShortSampleCount != 0 || health.HealthEvidenceAt != nil {
				t.Fatalf("missing samples became a score or disappeared: %+v", projected)
			}
		})
	}
}
