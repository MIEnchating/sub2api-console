package business_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

func TestGroupProbeModelsDoesNotDependOnHealthProjection(t *testing.T) {
	store, db := groupParticipationStore(t)
	if _, err := db.Exec(`UPDATE accounts SET metadata_json='{"known_models":["model-a"]}' WHERE id='41'; DROP TABLE account_health_evaluations`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Groups(t.Context()); err == nil {
		t.Fatal("fixture must make health projection unavailable")
	}
	result, err := store.GroupProbeModels(t.Context(), "7")
	if err != nil {
		t.Fatal(err)
	}
	if result.GroupID != "7" || result.GroupName != "codex" || !result.Complete || !reflect.DeepEqual(result.Models, []string{"model-a"}) {
		t.Fatalf("unexpected catalog: %+v", result)
	}
}

func TestGroupProbeModelsPreservesIdentityAndCancellationErrors(t *testing.T) {
	store, _ := groupParticipationStore(t)
	if _, err := store.GroupProbeModels(t.Context(), "99"); !errors.Is(err, business.ErrGroupNotFound) {
		t.Fatalf("missing ID: %v", err)
	}
	if _, err := store.GroupProbeModels(t.Context(), "codex"); err == nil {
		t.Fatal("name cannot replace stable ID")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := store.GroupProbeModels(ctx, "7"); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled read: %v", err)
	}
}
