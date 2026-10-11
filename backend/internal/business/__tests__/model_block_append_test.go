package business_test

import (
	"fmt"
	"reflect"
	"slices"
	"testing"
)

func TestModelBlockAppendAuditFailureRollsBackPolicy(t *testing.T) {
	store, db := groupParticipationStore(t)
	if err := store.SaveAccountModelExclusionList(t.Context(), []string{"legacy-*"}, "seed"); err != nil {
		t.Fatal(err)
	}
	before, err := store.ControlPolicy(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TRIGGER reject_model_block BEFORE INSERT ON runtime_events WHEN NEW.event_type='account_model_policy.updated' BEGIN SELECT RAISE(ABORT,'isolated audit failure'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AddAccountModelBlock(t.Context(), "model-a", "operator"); err == nil {
		t.Fatal("audit failure accepted")
	}
	after, err := store.ControlPolicy(t.Context())
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatalf("policy changed after rejected audit: %v", err)
	}
}

func TestModelBlockAppendAtLimitRejectsNewRuleButAllowsExistingMatch(t *testing.T) {
	store, _ := groupParticipationStore(t)
	patterns := make([]string, 200)
	for i := range patterns {
		patterns[i] = fmt.Sprintf("model-%03d", i)
	}
	if err := store.SaveAccountModelExclusionList(t.Context(), patterns, "seed"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AddAccountModelBlock(t.Context(), "new-model", "operator"); err == nil {
		t.Fatal("over-limit append accepted")
	}
	result, err := store.AddAccountModelBlock(t.Context(), " MODEL-001 ", "operator")
	if err != nil || !slices.Equal(result.BlockedPatterns, patterns) {
		t.Fatalf("existing model: %+v %v", result, err)
	}
	saved, err := store.AccountModelSyncSettings(t.Context())
	if err != nil || !slices.Equal(saved.BlockedPatterns, patterns) {
		t.Fatalf("saved: %+v %v", saved, err)
	}
}

func TestModelBlockAppendRejectsMissingPolicyWithoutAudit(t *testing.T) {
	store, db := groupParticipationStore(t)
	if _, err := db.Exec(`DELETE FROM policy_nodes WHERE policy_key='control-plane'`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AddAccountModelBlock(t.Context(), "model-a", "operator"); err == nil {
		t.Fatal("missing policy accepted")
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM runtime_events WHERE event_type='account_model_policy.updated'`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("audit count=%d err=%v", count, err)
	}
}
