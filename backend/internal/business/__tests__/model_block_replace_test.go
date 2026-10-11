package business_test

import (
	"errors"
	"reflect"
	"sync"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

func TestModelBlockReplacementConcurrentEditorsOnlyOneCommits(t *testing.T) {
	store, db := groupParticipationStore(t)
	if err := store.SaveAccountModelExclusionList(t.Context(), []string{"original"}, "seed"); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	var workers sync.WaitGroup
	for _, model := range []string{"first", "second"} {
		workers.Go(func() {
			<-start
			_, err := store.ReplaceAccountModelBlocks(t.Context(), []string{model}, []string{"original"}, "editor")
			results <- err
		})
	}
	close(start)
	workers.Wait()
	close(results)
	success, conflict := 0, 0
	for err := range results {
		if err == nil {
			success++
		} else if errors.Is(err, business.ErrModelSyncSettingsChanged) {
			conflict++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("success=%d conflict=%d", success, conflict)
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM runtime_events WHERE event_type='account_model_policy.updated'`).Scan(&count); err != nil || count != 2 {
		t.Fatalf("audit=%d err=%v", count, err)
	}
}

func TestModelBlockReplacementAuditFailureRollsBackPolicy(t *testing.T) {
	store, db := groupParticipationStore(t)
	if err := store.SaveAccountModelExclusionList(t.Context(), []string{"original"}, "seed"); err != nil {
		t.Fatal(err)
	}
	before, err := store.ControlPolicy(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TRIGGER reject_model_replace BEFORE INSERT ON runtime_events WHEN NEW.event_type='account_model_policy.updated' BEGIN SELECT RAISE(ABORT,'isolated audit failure'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReplaceAccountModelBlocks(t.Context(), []string{"draft"}, []string{"original"}, "editor"); err == nil {
		t.Fatal("audit failure accepted")
	}
	after, err := store.ControlPolicy(t.Context())
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatalf("policy changed: %v", err)
	}
}
