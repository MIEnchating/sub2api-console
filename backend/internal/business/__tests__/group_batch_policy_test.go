package business_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

func TestBatchGroupPolicyRejectsInvalidScopeWithoutWrites(t *testing.T) {
	for _, ids := range [][]string{nil, {"7", "7"}, {"codex"}, {"0"}, {""}, make([]string, 501)} {
		store, db := groupParticipationStore(t)
		if _, err := store.BatchGroupPolicy(t.Context(), ids, business.GroupBatchExclude, "test"); err == nil {
			t.Fatalf("accepted IDs %v", ids)
		}
		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM runtime_events WHERE event_type='group.scope.updated'`).Scan(&count); err != nil || count != 0 {
			t.Fatalf("events=%d err=%v", count, err)
		}
	}
}

func TestBatchGroupPolicyAuditFailureRollsBackPolicyAndLocalGroups(t *testing.T) {
	store, db := groupParticipationStore(t)
	if _, err := store.UpdatePolicy(t.Context(), map[string]any{"group_strategies": map[string]any{"7": "speed_first", "8": "price_first"}}, "test"); err != nil {
		t.Fatal(err)
	}
	before, err := store.ControlPolicy(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TRIGGER reject_second_batch_event BEFORE INSERT ON runtime_events WHEN json_extract(NEW.payload_json,'$.group_id')='8' BEGIN SELECT RAISE(ABORT,'isolated audit failure'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.BatchGroupPolicy(t.Context(), []string{"7", "8"}, business.GroupBatchReset, "test"); err == nil {
		t.Fatal("audit failure accepted")
	}
	after, err := store.ControlPolicy(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatal("policy partially committed")
	}
	var strategy string
	if err := db.QueryRow(`SELECT strategy FROM local_groups WHERE remote_id='7'`).Scan(&strategy); err != nil || strategy != "speed_first" {
		t.Fatalf("strategy=%s err=%v", strategy, err)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM runtime_events WHERE event_type='group.policy.cleared'`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("events=%d err=%v", count, err)
	}
}

func TestBatchGroupPolicyResetPreservesExclusionAndIncludePreservesOverrides(t *testing.T) {
	store, _ := groupParticipationStore(t)
	if _, err := store.UpdatePolicy(t.Context(), map[string]any{"group_strategies": map[string]any{"7": "speed_first", "8": "price_first"}, "excluded_group_ids": []any{"7", "8"}}, "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.BatchGroupPolicy(t.Context(), []string{"7"}, business.GroupBatchReset, "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.BatchGroupPolicy(t.Context(), []string{"8"}, business.GroupBatchInclude, "test"); err != nil {
		t.Fatal(err)
	}
	rows, err := store.Groups(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if *row.ID == "7" && (row.Override != nil || row.Status != "excluded") {
			t.Fatalf("reset=%+v", row)
		}
		if *row.ID == "8" && (row.Strategy != "price_first" || row.Status == "excluded") {
			t.Fatalf("include=%+v", row)
		}
	}
}

func TestBatchGroupPolicyAppliesOneActionAndPreservesPerGroupEvents(t *testing.T) {
	store, db := groupParticipationStore(t)
	if _, err := db.Exec(`INSERT INTO local_groups(name,remote_id,updated_at) VALUES('other','9','now')`); err != nil {
		t.Fatal(err)
	}
	updated, err := store.BatchGroupPolicy(t.Context(), []string{"7", "9"}, business.GroupBatchExclude, "operator")
	if err != nil || updated != 2 {
		t.Fatalf("updated=%d err=%v", updated, err)
	}
	control, err := store.ControlPolicy(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	scope, ok := control["scope"].(map[string]any)
	if !ok {
		t.Fatalf("scope=%#v", control["scope"])
	}
	if got := scope["excluded_group_ids"]; !reflect.DeepEqual(got, []any{"7", "9"}) {
		t.Fatal("batch exclusion did not persist")
	}
	var events int
	if err := db.QueryRow(`SELECT COUNT(*) FROM runtime_events WHERE event_type='group.scope.updated' AND json_extract(payload_json,'$.actor')='operator'`).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if events != 2 {
		t.Fatalf("events=%d want 2", events)
	}
}

func TestBatchGroupPolicyRollsBackWhenAnyGroupIsMissing(t *testing.T) {
	store, db := groupParticipationStore(t)
	controlBefore, err := store.ControlPolicy(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var eventsBefore int
	if err := db.QueryRow(`SELECT COUNT(*) FROM runtime_events`).Scan(&eventsBefore); err != nil {
		t.Fatal(err)
	}
	if _, err := store.BatchGroupPolicy(t.Context(), []string{"7", "999"}, business.GroupBatchExclude, "operator"); !errors.Is(err, business.ErrGroupNotFound) {
		t.Fatalf("error=%v", err)
	}
	controlAfter, err := store.ControlPolicy(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(controlAfter, controlBefore) {
		t.Fatalf("control policy changed after rollback: before=%#v after=%#v", controlBefore, controlAfter)
	}
	var eventsAfter int
	if err := db.QueryRow(`SELECT COUNT(*) FROM runtime_events`).Scan(&eventsAfter); err != nil {
		t.Fatal(err)
	}
	if eventsAfter != eventsBefore {
		t.Fatalf("events changed after rollback: before=%d after=%d", eventsBefore, eventsAfter)
	}
}
