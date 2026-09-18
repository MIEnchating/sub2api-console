package routing_test

import (
	"context"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/routing"
)

func TestProjectHealthOlderSnapshotCalculatedLastKeepsEarlierEvaluationTime(t *testing.T) {
	store, _ := healthEvidenceStore(t)
	service := routing.NewService(store)
	now := time.Now().UTC().Add(-time.Second)
	_, err := store.PersistTrafficSamples(t.Context(), []business.TrafficSample{{
		AccountID: "41", GroupName: "codex", Result: "通过", EvidenceKey: "original",
		ObservedAt: now.Add(-time.Second).Format(time.RFC3339Nano), Payload: map[string]any{},
	}})
	if err != nil {
		t.Fatal(err)
	}
	var earlier, later routing.HealthProjection
	err = store.WithReadSnapshot(t.Context(), func(first context.Context) error {
		originalResults, err := store.RecentAccountResults(first, "41", 10)
		if err != nil || len(originalResults) != 1 {
			t.Fatalf("initial colors are invalid: %+v (%v)", originalResults, err)
		}
		_, err = store.PersistTrafficSamples(t.Context(), []business.TrafficSample{{
			AccountID: "41", GroupName: "codex", Result: "失败", EvidenceKey: "new-failure",
			ObservedAt: now.Format(time.RFC3339Nano), Payload: map[string]any{"status_code": 503},
		}})
		if err != nil {
			return err
		}
		// Use the original context to open a second snapshot while the first is
		// still alive. Calculate its score before resuming the older snapshot.
		err = store.WithReadSnapshot(t.Context(), func(second context.Context) error {
			results, err := store.RecentAccountResults(second, "41", 10)
			if err != nil || len(results) != 2 || results[0].Result == nil || *results[0].Result != "失败" {
				t.Fatalf("new snapshot omitted the new failure color: %+v (%v)", results, err)
			}
			projected, err := service.ProjectHealth(second, routing.Scope{})
			later = projected["41"]
			return err
		})
		if err != nil {
			return err
		}
		projected, err := service.ProjectHealth(first, routing.Scope{})
		earlier = projected["41"]
		results, readErr := store.RecentAccountResults(first, "41", 10)
		if readErr != nil || len(results) != 1 || results[0].ID != originalResults[0].ID {
			t.Fatalf("older snapshot mixed in new colors: %+v (%v)", results, readErr)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if earlier.HealthScore == nil || *earlier.HealthScore != 100 || earlier.SampleCount != 1 || later.HealthScore == nil || *later.HealthScore >= 100 || later.SampleCount != 2 {
		t.Fatalf("snapshot scores do not match their evidence: earlier=%+v later=%+v", earlier, later)
	}
	firstTime, err := time.Parse(time.RFC3339Nano, earlier.HealthEvaluatedAt)
	if err != nil {
		t.Fatal(err)
	}
	secondTime, err := time.Parse(time.RFC3339Nano, later.HealthEvaluatedAt)
	if err != nil {
		t.Fatal(err)
	}
	if !firstTime.Before(secondTime) {
		t.Fatalf("older snapshot calculated last became newer: first=%s second=%s", firstTime, secondTime)
	}
}
