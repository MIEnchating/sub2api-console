package business_test

import (
	"context"
	"errors"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"reflect"
	"testing"
)

func TestGroupReadbackMatchesListWithExactMembershipAndCrossGroupEvidence(t *testing.T) {
	store, db := groupParticipationStore(t)
	if _, err := db.Exec(`INSERT INTO local_groups(name,remote_id,updated_at) VALUES('Codex','9','now'),('empty','10','now');
 INSERT INTO accounts(id,name,schedulable,metadata_json,updated_at) VALUES
 ('42','closed',0,'{}','now'),('43','unknown',NULL,'{}','now'),('44','other',1,'{}','now');
 INSERT INTO account_groups(account_id,group_name,group_id) VALUES
 ('41','7','8'),('42','codex',NULL),('43','codex','7'),('44','Codex','9');
 INSERT INTO account_health_evaluations(account_id,group_name,health_score,sample_count,evaluated_at) VALUES
 ('41','7',80.25,1,'2026-01-01T00:00:00Z'),('42','codex',90.5,1,'2026-01-01T00:00:00Z');`); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"7", "8", "9", "10"} {
		t.Run(id, func(t *testing.T) {
			got, err := store.SetGroupExcluded(t.Context(), id, false, "test")
			if err != nil {
				t.Fatal(err)
			}
			all, err := store.Groups(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range all {
				if want.ID != nil && *want.ID == id {
					if !reflect.DeepEqual(got, want) {
						t.Fatalf("readback=%+v list=%+v", got, want)
					}
					return
				}
			}
			t.Fatalf("missing group %s", id)
		})
	}
}

func TestGroupReadbackIgnoresUnrelatedMalformedAccount(t *testing.T) {
	store, db := groupParticipationStore(t)
	if _, err := db.Exec(`INSERT INTO accounts(id,name,priority,updated_at) VALUES('42','unrelated','invalid-priority','now');
 INSERT INTO account_groups(account_id,group_name,group_id) VALUES('42','7','8');`); err != nil {
		t.Fatal(err)
	}
	got, err := store.SetGroupExcluded(t.Context(), "7", true, "test")
	if err != nil {
		t.Fatalf("unrelated account broke committed group readback: %v", err)
	}
	if got.ID == nil || *got.ID != "7" || got.Status != "excluded" || got.AccountCount != 1 {
		t.Fatalf("readback=%+v", got)
	}
}

func TestGroupReadbackRejectsMalformedMember(t *testing.T) {
	store, db := groupParticipationStore(t)
	if _, err := db.Exec(`UPDATE accounts SET priority='invalid-priority' WHERE id='41'`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GroupAllocation(t.Context(), "7"); err == nil {
		t.Fatal("malformed member silently accepted")
	}
}

func TestGroupReadbackRejectsMissingStableIDAndCancelledContext(t *testing.T) {
	store, _ := groupParticipationStore(t)
	if _, err := store.GroupAllocation(t.Context(), "41"); !errors.Is(err, business.ErrGroupNotFound) {
		t.Fatalf("missing group error=%v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := store.GroupAllocation(ctx, "7"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel error=%v", err)
	}
}
