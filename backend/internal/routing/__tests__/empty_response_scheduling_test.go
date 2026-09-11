package routing_test

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/routing"
)

func TestEmptyResponseLowersWeightWithoutRepeatedPenaltyThenConfirms(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "routing.sqlite3")
	store, err := business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO accounts(id,name,multiplier,schedulable,metadata_json,updated_at) VALUES('41','empty','1',1,'{}','now'),('42','healthy','1',1,'{}','now'); INSERT INTO account_groups(account_id,group_name) VALUES('41','codex'),('42','codex')`)
	_ = db.Close()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	for _, id := range []string{"41", "42"} {
		payload := map[string]any{"input_tokens": 10, "output_tokens": 5}
		if id == "41" {
			payload = map[string]any{"input_tokens": 0, "output_tokens": 0}
		}
		_, err := store.PersistTrafficSamples(ctx, []business.TrafficSample{{AccountID: id, GroupName: "codex", Result: "通过", EvidenceKey: id, ObservedAt: now.Add(-time.Second).Format(time.RFC3339Nano), Payload: payload}})
		if err != nil {
			t.Fatal(err)
		}
	}
	service := routing.NewService(store)
	for round := 0; round < 3; round++ {
		result, err := service.Calculate(ctx, routing.Scope{}, true)
		if err != nil {
			t.Fatal(err)
		}
		empty, healthy := result.AccountDecisions["41"], result.AccountDecisions["42"]
		if empty.LatestEvent != "empty_response" || empty.HealthScore != 40 || empty.RoutingHealthScore != 70 || !empty.EvidencePending || empty.RoutingState != "degraded" || !empty.Schedulable || empty.Weight >= healthy.Weight || empty.Weight <= 0 {
			t.Fatalf("round=%d empty=%+v healthy=%+v", round, empty, healthy)
		}
	}
	_, err = store.PersistTrafficSamples(ctx, []business.TrafficSample{{AccountID: "41", GroupName: "codex", Result: "通过", EvidenceKey: "second-empty", ObservedAt: now.Format(time.RFC3339Nano), Payload: map[string]any{"input_tokens": 0, "output_tokens": 0}}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Calculate(ctx, routing.Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	confirmed := result.AccountDecisions["41"]
	if confirmed.EvidencePending || confirmed.RoutingHealthScore != 40 || confirmed.RoutingState != "degraded" {
		t.Fatalf("consecutive empty responses did not confirm degradation: %+v", confirmed)
	}
}

func TestEmptyResponseScorePolicyAcceptsValidValuesAndRejectsInvalidValues(t *testing.T) {
	for _, value := range []any{35, -1, 101, "40"} {
		t.Run(fmt.Sprint(value), func(t *testing.T) {
			store, err := business.Open(filepath.Join(t.TempDir(), "policy.sqlite3"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = store.Close() })
			if err := store.Bootstrap(context.Background()); err != nil {
				t.Fatal(err)
			}
			_, err = store.UpdatePolicy(context.Background(), map[string]any{"advanced_policy": map[string]any{"scoring": map[string]any{"event_scores": map[string]any{"empty_response": value}}}}, "test")
			if (err == nil) != (value == 35) {
				t.Fatalf("value=%v err=%v", value, err)
			}
		})
	}
}
