package business

import (
	"context"
	"testing"
)

func TestAccountRecoveryReadbackPreservesEngineGatesAndClearsManualOverrides(t *testing.T) {
	store := openPolicyStore(t)
	ctx := context.Background()
	if _, err := store.db.ExecContext(ctx, `INSERT INTO accounts(id,name,schedulable,routing_state,metadata_json,updated_at) VALUES('41','recovering',0,'fused','{"status":"active"}','now')`); err != nil {
		t.Fatal(err)
	}
	payload := `{"recovery":{"evaluated_at":"2026-09-07T12:00:00Z","ready":false,"conditions":[{"code":"success_streak","met":false,"detail":"连续成功 1/2 次"}]}}`
	if _, err := store.db.ExecContext(ctx, `INSERT INTO routing_decisions(account_id,group_name,routing_state,reason,updated_at,payload_json) VALUES('41','codex','fused','等待恢复','9999-01-01',?)`, payload); err != nil {
		t.Fatal(err)
	}
	account, err := store.Account(ctx, "41")
	if err != nil {
		t.Fatal(err)
	}
	if account.Recovery == nil || account.Recovery.Ready || len(account.Recovery.Conditions) != 1 || account.Recovery.Conditions[0].Detail != "连续成功 1/2 次" {
		t.Fatalf("recovery progress lost: %+v", account.Recovery)
	}
	if err := store.CommitAccountControlReadback(ctx, "41", "fuse", "operator", false, testControlOperation("manual-fuse")); err != nil {
		t.Fatal(err)
	}
	account, err = store.Account(ctx, "41")
	if err != nil {
		t.Fatal(err)
	}
	if account.Recovery != nil {
		t.Fatal("manual fuse retained automatic recovery progress")
	}
}

func TestIncompleteRecoveryPayloadDoesNotExposeInvalidProgress(t *testing.T) {
	store := openPolicyStore(t)
	ctx := context.Background()
	if _, err := store.db.ExecContext(ctx, `INSERT INTO accounts(id,name,metadata_json,updated_at) VALUES('41','recovering','{}','now')`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO routing_decisions(account_id,group_name,routing_state,updated_at,payload_json) VALUES('41','codex','fused','9999-01-01','{"recovery":{}}')`); err != nil {
		t.Fatal(err)
	}
	account, err := store.Account(ctx, "41")
	if err != nil {
		t.Fatal(err)
	}
	if account.Recovery != nil {
		t.Fatal("incomplete recovery payload must be omitted")
	}
}
