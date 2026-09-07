package business

import (
	"context"
	"encoding/json"
	"testing"
)

func TestAccountReadModelExposesPendingEvidenceWithoutChangingRoutingState(t *testing.T) {
	for _, tc := range []struct {
		name    string
		payload string
		reason  string
		pending bool
	}{
		{"pending", `{"evidence_pending":true}`, "短暂异常待确认，保持当前调度位置", true},
		{"existing decision", `{}`, "短暂异常待确认，保持当前调度位置", true},
		{"confirmed", `{"evidence_pending":false}`, "已计算", false},
		{"explicit false", `{"evidence_pending":false}`, "短暂异常待确认，保持当前调度位置", false},
		{"no evidence", `{}`, "已计算", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := openPolicyStore(t)
			ctx := context.Background()
			if _, err := store.db.ExecContext(ctx, `INSERT INTO accounts(id,name,schedulable,routing_state,metadata_json,updated_at) VALUES('41','channel-41',1,'healthy','{"status":"active"}','now')`); err != nil {
				t.Fatal(err)
			}
			if _, err := store.db.ExecContext(ctx, `INSERT INTO routing_decisions(account_id,group_name,routing_state,reason,updated_at,payload_json) VALUES('41','codex','healthy',?,'9999-01-01',?)`, tc.reason, tc.payload); err != nil {
				t.Fatal(err)
			}
			account, err := store.Account(ctx, "41")
			if err != nil {
				t.Fatal(err)
			}
			encoded, err := json.Marshal(account.AccountStatus)
			if err != nil {
				t.Fatal(err)
			}
			var response map[string]any
			if err := json.Unmarshal(encoded, &response); err != nil {
				t.Fatal(err)
			}
			if response["evidence_pending"] != tc.pending {
				t.Fatalf("pending evidence = %v, want %v", response["evidence_pending"], tc.pending)
			}
			if account.Health != "healthy" || account.Schedulable == nil || !*account.Schedulable {
				t.Fatalf("observation changed routing: %+v", account.AccountStatus)
			}
		})
	}
}

func TestDegradedAccountReadModelRetainsPendingEvidence(t *testing.T) {
	store := openPolicyStore(t)
	ctx := context.Background()
	if _, err := store.db.ExecContext(ctx, `INSERT INTO accounts(id,name,schedulable,routing_state,metadata_json,updated_at) VALUES('41','channel-41',1,'degraded','{"status":"active"}','now')`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO routing_decisions(account_id,group_name,routing_state,reason,updated_at,payload_json) VALUES('41','codex','degraded','短暂异常待确认，暂时降低调度权重','9999-01-01','{"evidence_pending":true}')`); err != nil {
		t.Fatal(err)
	}
	account, err := store.Account(ctx, "41")
	if err != nil {
		t.Fatal(err)
	}
	if account.Health != "degraded" || !account.EvidencePending || account.Schedulable == nil || !*account.Schedulable {
		t.Fatalf("pending degradation lost in projection: %+v", account.AccountStatus)
	}
}
