package business

import (
	"context"
	"testing"
)

func TestAccountControlPersistsPauseFuseAndRecoveryState(t *testing.T) {
	store := openPolicyStore(t)
	ctx := context.Background()
	if _, err := store.db.ExecContext(ctx, `INSERT INTO accounts(id,name,metadata_json,updated_at) VALUES('41','channel-41','{}','now')`); err != nil {
		t.Fatal(err)
	}

	if err := store.CommitAccountControlReadback(ctx, "41", "pause", "operator", false, testControlOperation("pause-1")); err != nil {
		t.Fatal(err)
	}
	document, err := store.readPolicyDocument(ctx, store.db, "control-plane")
	if err != nil {
		t.Fatal(err)
	}
	scope := document["scope"].(map[string]any)
	if values := controlAccountIDs(scope["paused_account_ids"]); len(values) != 1 {
		t.Fatalf("pause list=%#v", values)
	}
	var paused int
	if err := store.db.QueryRowContext(ctx, `SELECT paused FROM accounts WHERE id='41'`).Scan(&paused); err != nil || paused != 1 {
		t.Fatalf("paused=%d err=%v", paused, err)
	}

	if err := store.CommitAccountControlReadback(ctx, "41", "resume", "operator", true, testControlOperation("resume-1")); err != nil {
		t.Fatal(err)
	}
	if err := store.CommitAccountControlReadback(ctx, "41", "fuse", "operator", false, testControlOperation("fuse-1")); err != nil {
		t.Fatal(err)
	}
	document, _ = store.readPolicyDocument(ctx, store.db, "control-plane")
	scope = document["scope"].(map[string]any)
	if values := controlAccountIDs(scope["manual_fused_account_ids"]); len(values) != 1 {
		t.Fatalf("manual fuse list=%#v", values)
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO routing_decisions(account_id,group_name,routing_state,updated_at,payload_json) VALUES('41','codex','fused','now','{}')`); err != nil {
		t.Fatal(err)
	}
	if err := store.CommitAccountControlReadback(ctx, "41", "recover", "operator", true, testControlOperation("recover-1")); err != nil {
		t.Fatal(err)
	}
	var decisions int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM routing_decisions WHERE account_id='41'`).Scan(&decisions); err != nil || decisions != 0 {
		t.Fatalf("decisions=%d err=%v", decisions, err)
	}
}

func TestAccountControlDoesNotResetOtherAccountsRoutingHistory(t *testing.T) {
	store := openPolicyStore(t)
	ctx := context.Background()
	epochAt := "2026-08-28T00:00:00Z"
	decisionAt := "2026-08-28T01:00:00Z"
	if _, err := store.db.ExecContext(ctx, `INSERT INTO accounts(id,name,metadata_json,updated_at) VALUES
		('41','channel-41','{}','now'),('42','channel-42','{}','now');
		INSERT INTO app_state(key,value_json,updated_at) VALUES('routing-decision-epoch','{}',?)
		ON CONFLICT(key) DO UPDATE SET updated_at=excluded.updated_at;
		INSERT INTO routing_decisions(account_id,group_name,routing_state,updated_at,payload_json)
		VALUES('42','codex','fused',?,'{"fused_until":"2026-08-28T01:03:00Z"}')`, epochAt, decisionAt); err != nil {
		t.Fatal(err)
	}

	if err := store.CommitAccountControlReadback(ctx, "41", "pause", "operator", false, testControlOperation("pause-1")); err != nil {
		t.Fatal(err)
	}
	rows, err := store.PreviousRoutingDecisions(ctx, nil, nil)
	if err != nil || len(rows) != 1 || rows[0].AccountID != "42" || rows[0].State != "fused" {
		t.Fatalf("unrelated routing history was discarded: rows=%#v err=%v", rows, err)
	}
	var epoch string
	if err := store.db.QueryRowContext(ctx, `SELECT updated_at FROM app_state WHERE key='routing-decision-epoch'`).Scan(&epoch); err != nil {
		t.Fatal(err)
	}
	if epoch != epochAt {
		t.Fatalf("account control changed global decision epoch: got=%s want=%s", epoch, epochAt)
	}
}

func TestAccountScopeControlRejectsRemoteSchedulingActions(t *testing.T) {
	store := openPolicyStore(t)
	if _, err := store.SetAccountScopeControl(context.Background(), "41", "pause", "operator"); err == nil {
		t.Fatal("local scope control accepted a remote scheduling action")
	}
}

func testControlOperation(id string) AccountOperation {
	field := "schedulable"
	return AccountOperation{
		OperationID: id, OperationType: "account.control", State: "succeeded", Phase: "readback",
		Actor: "operator", RemoteConfirmed: true, ReadbackConfirmed: true,
		ObjectID: "41", GroupNames: []string{}, FieldName: &field,
		Before: map[string]any{}, After: map[string]any{}, Writeback: true,
	}
}

func TestAccountTestModelPersistsAndCanBeCleared(t *testing.T) {
	store := openPolicyStore(t)
	ctx := context.Background()
	if _, err := store.db.ExecContext(ctx, `INSERT INTO accounts(id,name,metadata_json,updated_at) VALUES('41','channel-41','{}','now')`); err != nil {
		t.Fatal(err)
	}
	model := " gpt-5.1-codex "
	if err := store.SetAccountTestModel(ctx, "41", &model, "operator"); err != nil {
		t.Fatal(err)
	}
	detail, err := store.Account(ctx, "41")
	if err != nil || detail.TestModel == nil || *detail.TestModel != "gpt-5.1-codex" {
		t.Fatalf("model=%v err=%v", detail.TestModel, err)
	}
	if err := store.SetAccountTestModel(ctx, "41", nil, "operator"); err != nil {
		t.Fatal(err)
	}
	detail, err = store.Account(ctx, "41")
	if err != nil || detail.TestModel != nil {
		t.Fatalf("cleared model=%v err=%v", detail.TestModel, err)
	}
}
