package routing_test

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/routing"
)

const discoveryPromptFingerprint = "v1:8f434346648f6b96df89dda901c5176b10a6d83961dd3c1ac88b59b2dc327aa4"

func TestComparableProbeSeriesDiscoversFastIdleCandidateWithoutChangingTrafficPercentiles(t *testing.T) {
	store, db := discoveryStore(t)
	now := time.Now().UTC().Add(-time.Second)
	insertDiscoveryProbes(t, db, "41", "4000", now, 5, nil)
	insertDiscoveryProbes(t, db, "42", "200", now, 5, nil)
	result, err := routing.NewService(store).Calculate(t.Context(), routing.Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	incumbent, challenger := result.AccountDecisions["41"], result.AccountDecisions["42"]
	if incumbent.Priority == nil || challenger.Priority == nil || *challenger.Priority >= *incumbent.Priority {
		t.Fatalf("comparable sustained probes failed to discover idle candidate: incumbent=%+v challenger=%+v", incumbent, challenger)
	}
	if challenger.TTFBP50MS != nil || challenger.TTFBP95MS != nil || incumbent.TTFBP95MS != nil {
		t.Fatal("probe discovery polluted real traffic percentiles")
	}
}

func TestUnreliableOrIncomparableProbeEvidenceCannotDisplaceIncumbent(t *testing.T) {
	for _, scenario := range []struct {
		name   string
		mutate func(int, map[string]any)
		count  int
		age    time.Duration
		policy map[string]any
	}{
		{name: "one lucky probe", count: 1},
		{name: "retry recovered", count: 5, mutate: func(_ int, p map[string]any) { p["retry_recovered"] = true }},
		{name: "different actual model", count: 5, mutate: func(_ int, p map[string]any) { p["actual_model"] = "other-model" }},
		{name: "missing actual model", count: 5, mutate: func(_ int, p map[string]any) { delete(p, "actual_model") }},
		{name: "different request configuration", count: 5, mutate: func(_ int, p map[string]any) { p["probe_request_fingerprint"] = "v1:other-request" }},
		{name: "no measured first token", count: 5, mutate: func(_ int, p map[string]any) { p["measured_first_token"] = false }},
		{name: "interrupted successful series", count: 6, mutate: func(i int, p map[string]any) {
			if i == 2 {
				p["performance_eligible"] = false
			}
		}},
		{name: "expired latest observation", count: 5, age: 16 * time.Minute},
		{name: "changed current prompt", count: 5, policy: map[string]any{"prompt": "new prompt"}},
		{name: "changed current model", count: 5, policy: map[string]any{"model": "new-model"}},
		{name: "exploration disabled", count: 5, policy: map[string]any{"performance_exploration_enabled": false}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			store, db := discoveryStore(t)
			if scenario.policy != nil {
				patch := map[string]any{"advanced_policy": map[string]any{"probe": scenario.policy}}
				if model, ok := scenario.policy["model"]; ok {
					patch = map[string]any{"probe_model": model}
				}
				if _, err := store.UpdatePolicy(t.Context(), patch, "test"); err != nil {
					t.Fatal(err)
				}
			}
			now := time.Now().UTC().Add(-time.Second)
			insertDiscoveryProbes(t, db, "41", "4000", now, 5, nil)
			insertDiscoveryProbes(t, db, "42", "200", now.Add(-scenario.age), scenario.count, scenario.mutate)
			result, err := routing.NewService(store).Calculate(t.Context(), routing.Scope{}, true)
			if err != nil {
				t.Fatal(err)
			}
			incumbent, challenger := result.AccountDecisions["41"], result.AccountDecisions["42"]
			if incumbent.Priority == nil || challenger.Priority == nil || *incumbent.Priority >= *challenger.Priority {
				t.Fatalf("untrusted probe series displaced incumbent: incumbent=%+v challenger=%+v", incumbent, challenger)
			}
		})
	}
}

func TestSufficientComparableRealTrafficTakesPrecedenceOverProbeSpeed(t *testing.T) {
	store, db := discoveryStore(t)
	now := time.Now().UTC().Add(-time.Second)
	insertDiscoveryProbes(t, db, "41", "4000", now.Add(-time.Minute), 5, nil)
	insertDiscoveryProbes(t, db, "42", "200", now.Add(-time.Minute), 5, nil)
	for _, account := range []struct {
		id      string
		latency int
	}{{"41", 100}, {"42", 1500}} {
		for i := range 5 {
			_, err := store.PersistTrafficSamples(t.Context(), []business.TrafficSample{{
				AccountID: account.id, GroupName: "codex", EvidenceKey: fmt.Sprintf("traffic-%s-%d", account.id, i),
				Result: "通过", ObservedAt: now.Add(-time.Duration(i) * time.Second).Format(time.RFC3339Nano),
				Payload: map[string]any{"status_code": 200, "model": "model-a", "first_token_ms": account.latency},
			}})
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	result, err := routing.NewService(store).Calculate(t.Context(), routing.Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	incumbent, challenger := result.AccountDecisions["41"], result.AccountDecisions["42"]
	if incumbent.Priority == nil || challenger.Priority == nil || *incumbent.Priority >= *challenger.Priority || incumbent.TTFBP95MS == nil || *incumbent.TTFBP95MS != 100 {
		t.Fatalf("traffic performance lost precedence: incumbent=%+v challenger=%+v", incumbent, challenger)
	}
}

func TestBusyIncumbentRetainsReferenceProbesForIdleCandidateComparison(t *testing.T) {
	store, db := discoveryStore(t)
	now := time.Now().UTC().Add(-time.Second)
	insertDiscoveryProbes(t, db, "41", "4000", now.Add(-time.Minute), 5, nil)
	insertDiscoveryProbes(t, db, "42", "200", now, 5, nil)
	samples := make([]business.TrafficSample, 220)
	for i := range samples {
		samples[i] = business.TrafficSample{AccountID: "41", GroupName: "codex", EvidenceKey: fmt.Sprintf("busy-%d", i), Result: "通过", ObservedAt: now.Add(-time.Duration(i) * time.Millisecond).Format(time.RFC3339Nano), Payload: map[string]any{"status_code": 200}}
	}
	if _, err := store.PersistTrafficSamples(t.Context(), samples); err != nil {
		t.Fatal(err)
	}
	result, err := routing.NewService(store).Calculate(t.Context(), routing.Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	incumbent, challenger := result.AccountDecisions["41"], result.AccountDecisions["42"]
	if incumbent.Priority == nil || challenger.Priority == nil || *challenger.Priority >= *incumbent.Priority {
		t.Fatal("busy account traffic evicted its reference probes and blocked discovery")
	}
}

func TestProbeConfidenceThresholdCanExceedScoringWindow(t *testing.T) {
	store, db := discoveryStore(t)
	_, err := store.UpdatePolicy(t.Context(), map[string]any{"advanced_policy": map[string]any{"weights": map[string]any{"performance_min_samples": 65}}}, "test")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Add(-time.Second)
	insertDiscoveryProbes(t, db, "41", "4000", now, 65, nil)
	insertDiscoveryProbes(t, db, "42", "200", now, 65, nil)
	result, err := routing.NewService(store).Calculate(t.Context(), routing.Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if result.AccountDecisions["42"].Rank == nil || *result.AccountDecisions["42"].Rank != 1 {
		t.Fatal("confidence threshold above scoring window became unreachable")
	}
}

func TestOneFreshProbeCannotReuseAnInterruptedHistoricalPerformanceSeries(t *testing.T) {
	store, db := discoveryStore(t)
	now := time.Now().UTC().Add(-time.Second)
	insertDiscoveryProbes(t, db, "41", "4000", now, 5, nil)
	insertDiscoveryProbes(t, db, "42", "200", now, 5, nil)
	for i := 1; i < 5; i++ {
		_, err := db.Exec(`UPDATE health_samples SET observed_at=? WHERE account_id='42' AND evidence_key=?`, now.Add(-23*time.Hour-time.Duration(i)*time.Second).Format(time.RFC3339Nano), fmt.Sprintf("probe-42-model-a-%d", i))
		if err != nil {
			t.Fatal(err)
		}
	}
	result, err := routing.NewService(store).Calculate(t.Context(), routing.Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if result.AccountDecisions["41"].Rank == nil || *result.AccountDecisions["41"].Rank != 1 {
		t.Fatal("a single current probe reused the interrupted historical series to promote")
	}
}

func TestMultipleProbeModelsEachReceiveTheirOwnConfidenceWindow(t *testing.T) {
	for _, scenario := range []struct{ models, minimum int }{{20, 5}, {2, 65}} {
		t.Run(fmt.Sprintf("%d_models_%d_samples", scenario.models, scenario.minimum), func(t *testing.T) {
			store, db := discoveryStore(t)
			models := make([]string, scenario.models)
			for i := range models {
				models[i] = fmt.Sprintf("model-%02d", i)
			}
			for _, id := range []string{"41", "42"} {
				if err := store.SetAccountTestModels(t.Context(), id, models, "test"); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := store.UpdatePolicy(t.Context(), map[string]any{"advanced_policy": map[string]any{"weights": map[string]any{"performance_min_samples": scenario.minimum}}}, "test"); err != nil {
				t.Fatal(err)
			}
			now := time.Now().UTC().Add(-time.Second)
			for _, model := range models {
				mutate := func(_ int, p map[string]any) { p["request_model"], p["actual_model"] = model, model }
				insertDiscoveryProbes(t, db, "41", "4000", now, scenario.minimum, mutate)
				insertDiscoveryProbes(t, db, "42", "200", now, scenario.minimum, mutate)
			}
			result, err := routing.NewService(store).Calculate(t.Context(), routing.Scope{}, true)
			if err != nil {
				t.Fatal(err)
			}
			if result.AccountDecisions["42"].Rank == nil || *result.AccountDecisions["42"].Rank != 1 {
				t.Fatal("global probe window prevented each model reaching its confidence threshold")
			}
		})
	}
}

func discoveryStore(t *testing.T) (*business.Store, *sql.DB) {
	t.Helper()
	store, db := healthEvidenceStore(t)
	if _, err := db.Exec(`UPDATE accounts SET priority=20 WHERE id='41';
		INSERT INTO accounts(id,name,multiplier,priority,schedulable,metadata_json,updated_at) VALUES('42','idle-candidate','1',21,1,'{}','now');
		INSERT INTO account_groups(account_id,group_name) VALUES('42','codex')`); err != nil {
		t.Fatal(err)
	}
	return store, db
}

func insertDiscoveryProbes(t *testing.T, db *sql.DB, id, latency string, now time.Time, count int, mutate func(int, map[string]any)) {
	t.Helper()
	for i := range count {
		payload := map[string]any{
			"status_code": 200, "request_model": "model-a", "actual_model": "model-a", "probe_protocol": "responses",
			"probe_prompt_fingerprint": discoveryPromptFingerprint, "probe_request_fingerprint": "v1:request-fixture",
			"measured_first_token": true, "performance_eligible": true,
			"latency_metric": "first_token", "latency_source": "upstream_direct.first_content", "latency_unit": "ms",
		}
		if mutate != nil {
			mutate(i, payload)
		}
		raw, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		at := now.Add(-time.Duration(i) * 5 * time.Minute).Format(time.RFC3339Nano)
		_, err = db.Exec(`INSERT INTO health_samples(account_id,group_name,result,latency_p95,sample_count,attempts,observed_at,source,evidence_key,payload_json)
			VALUES(?,'codex','通过',?,1,1,?,'active-probe',?,?)`, id, latency, at, fmt.Sprintf("probe-%s-%s-%d", id, payload["request_model"], i), string(raw))
		if err != nil {
			t.Fatal(err)
		}
	}
}
