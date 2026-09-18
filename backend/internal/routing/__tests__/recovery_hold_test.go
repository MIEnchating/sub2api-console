package routing_test

import (
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/routing"
)

func TestHighTrafficRecoveryKeepsHoldEvidenceBeyondSampleCountLimit(t *testing.T) {
	store, db := healthEvidenceStore(t)
	if _, err := db.Exec(`UPDATE accounts SET routing_state='degraded' WHERE id='41'`); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Add(-2 * time.Second)
	samples := make([]business.TrafficSample, 800)
	for index := range samples {
		samples[index] = business.TrafficSample{AccountID: "41", GroupName: "codex", EvidenceKey: strconv.Itoa(index), Result: "通过", ObservedAt: now.Add(-time.Duration(index) * 100 * time.Millisecond).Format(time.RFC3339Nano), Payload: map[string]any{"status_code": 200}}
	}
	if _, err := store.PersistTrafficSamples(t.Context(), samples); err != nil {
		t.Fatal(err)
	}
	result, err := routing.NewService(store, routing.WithClock(func() time.Time { return now.Add(2 * time.Second) })).Calculate(t.Context(), routing.Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	decision := result.AccountDecisions["41"]
	if decision.RoutingState != "healthy" || !decision.Schedulable || decision.SampleCount != 60 {
		t.Fatalf("successful traffic failed hold without changing scoring window: %+v", decision)
	}
}

func TestRecoveryHoldRejectsInterruptionsOutsideTheBoundedHealthCache(t *testing.T) {
	for _, test := range []struct {
		name    string
		result  string
		payload map[string]any
	}{
		{"gateway failure", "失败", map[string]any{"status_code": 502}},
		{"neutral client error", "失败", map[string]any{"status_code": 403}},
		{"empty response", "通过", map[string]any{"input_tokens": 0, "output_tokens": 0}},
	} {
		t.Run(test.name, func(t *testing.T) {
			store, db := healthEvidenceStore(t)
			if _, err := db.Exec(`UPDATE accounts SET routing_state='degraded' WHERE id='41'`); err != nil {
				t.Fatal(err)
			}
			now := time.Now().UTC().Add(-2 * time.Second)
			samples := make([]business.TrafficSample, 800)
			for index := range samples {
				samples[index] = business.TrafficSample{AccountID: "41", GroupName: "codex", EvidenceKey: strconv.Itoa(index), Result: "通过", ObservedAt: now.Add(-time.Duration(index) * 100 * time.Millisecond).Format(time.RFC3339Nano), Payload: map[string]any{"status_code": 200}}
			}
			samples[400].Result, samples[400].Payload = test.result, test.payload
			if _, err := store.PersistTrafficSamples(t.Context(), samples); err != nil {
				t.Fatal(err)
			}
			// Database writes under -race must not age the interruption out of the hold window.
			result, err := routing.NewService(store, routing.WithClock(func() time.Time { return now.Add(2 * time.Second) })).Calculate(t.Context(), routing.Scope{}, true)
			if err != nil {
				t.Fatal(err)
			}
			decision := result.AccountDecisions["41"]
			if decision.RoutingState != "degraded" || decision.HealthScore != 100 || decision.Recovery == nil || decision.Recovery.Ready {
				t.Fatalf("interrupted hold was treated as continuous health: %+v", decision)
			}
		})
	}
}

func TestRecoveryHoldTimeEvidenceStillRespectsFuseStartAndFreshness(t *testing.T) {
	for _, test := range []struct {
		name  string
		fused bool
	}{
		{"pre-fuse successes", true},
		{"expired traffic", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			store, db := healthEvidenceStore(t)
			now := time.Now().UTC().Add(-2 * time.Second)
			if test.fused {
				payload, err := json.Marshal(map[string]any{"state_since": now.Add(-30 * time.Second).Format(time.RFC3339Nano), "fused_until": now.Add(-time.Second).Format(time.RFC3339Nano)})
				if err != nil {
					t.Fatal(err)
				}
				_, err = db.Exec(`UPDATE accounts SET routing_state='fused',schedulable=0 WHERE id='41';
					INSERT INTO routing_decisions(account_id,group_name,schedulable,routing_state,updated_at,payload_json)
					VALUES('41','codex',0,'fused',?,?)`, now.Format(time.RFC3339Nano), string(payload))
				if err != nil {
					t.Fatal(err)
				}
			} else {
				if _, err := db.Exec(`UPDATE accounts SET routing_state='degraded' WHERE id='41'`); err != nil {
					t.Fatal(err)
				}
				if _, err := store.UpdatePolicy(t.Context(), map[string]any{"traffic_lookback_minutes": 1}, "test"); err != nil {
					t.Fatal(err)
				}
			}
			samples := make([]business.TrafficSample, 800)
			for index := range samples {
				samples[index] = business.TrafficSample{AccountID: "41", GroupName: "codex", EvidenceKey: strconv.Itoa(index), Result: "通过", ObservedAt: now.Add(-time.Duration(index) * 100 * time.Millisecond).Format(time.RFC3339Nano), Payload: map[string]any{"status_code": 200}}
			}
			if _, err := store.PersistTrafficSamples(t.Context(), samples); err != nil {
				t.Fatal(err)
			}
			// Put the freshness cutoff between samples: retained evidence spans
			// less than the required 60 seconds regardless of database write time.
			result, err := routing.NewService(store, routing.WithClock(func() time.Time { return now.Add(2050 * time.Millisecond) })).Calculate(t.Context(), routing.Scope{}, true)
			if err != nil {
				t.Fatal(err)
			}
			decision := result.AccountDecisions["41"]
			if decision.RoutingState == "healthy" || decision.Recovery == nil || decision.Recovery.Ready {
				t.Fatalf("out-of-scope hold evidence allowed recovery: %+v", decision)
			}
		})
	}
}
