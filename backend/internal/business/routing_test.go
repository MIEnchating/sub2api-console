package business

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestRoutingSamplesLimitsCombinedSourcesPerAccount(t *testing.T) {
	store := openPolicyStore(t)
	ctx := context.Background()
	base := time.Date(2026, 8, 28, 0, 0, 0, 0, time.UTC)

	insert := func(accountID string, index int, source string) {
		t.Helper()
		observedAt := base.Add(time.Duration(index) * time.Second).Format(time.RFC3339Nano)
		evidenceKey := fmt.Sprintf("%s:%s:%d", accountID, source, index)
		if _, err := store.db.ExecContext(ctx, `INSERT INTO health_samples(
			account_id,group_name,result,observed_at,source,evidence_key,payload_json
		) VALUES(?, 'codex', '通过', ?, ?, ?, '{}')`, accountID, observedAt, source, evidenceKey); err != nil {
			t.Fatal(err)
		}
	}

	for index := range 60 {
		insert("41", index, "traffic")
	}
	for index := range 3 {
		insert("41", 60+index, "active-probe")
	}
	for index := range 8 {
		insert("42", index, "traffic")
	}

	rows, err := store.RoutingSamples(ctx, nil, nil, "traffic", 60)
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for _, row := range rows {
		counts[row.AccountID]++
	}
	if counts["41"] != 60 || counts["42"] != 8 || len(rows) != 68 {
		t.Fatalf("combined scoring window must be capped per account: counts=%v total=%d", counts, len(rows))
	}
	if rows[0].AccountID != "41" || rows[0].Source != "active-probe" {
		t.Fatalf("latest mixed-source sample must be first: %#v", rows[0])
	}
}

func TestRoutingSamplesFiltersPureProbeBeforeApplyingAccountWindow(t *testing.T) {
	store := openPolicyStore(t)
	ctx := context.Background()
	base := time.Date(2026, 8, 28, 0, 0, 0, 0, time.UTC)
	insert := func(index int, source string) {
		t.Helper()
		observedAt := base.Add(time.Duration(index) * time.Second).Format(time.RFC3339Nano)
		if _, err := store.db.ExecContext(ctx, `INSERT INTO health_samples(
			account_id,group_name,result,observed_at,source,evidence_key,payload_json
		) VALUES('41','codex','通过',?,?,?,'{}')`, observedAt, source, fmt.Sprintf("%s:%d", source, index)); err != nil {
			t.Fatal(err)
		}
	}
	for index := range 8 {
		insert(index, "active-probe")
	}
	for index := range 63 {
		insert(100+index, "traffic")
	}

	rows, err := store.RoutingSamples(ctx, nil, nil, "active_probe", 60)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 8 {
		t.Fatalf("pure-probe mode must retain the actual 8 probe samples before limiting: %d", len(rows))
	}
	for _, row := range rows {
		if row.Source != "active-probe" {
			t.Fatalf("pure-probe query leaked source %q", row.Source)
		}
	}
}

func TestRoutingSamplesForSecondaryGroupUsesAccountEvidence(t *testing.T) {
	store := openPolicyStore(t)
	ctx := context.Background()
	if _, err := store.db.ExecContext(ctx, `INSERT INTO accounts(id,name,metadata_json,updated_at)
		VALUES('41','multi-group','{}','now')`); err != nil {
		t.Fatal(err)
	}
	for _, groupName := range []string{"codex", "pro"} {
		if _, err := store.db.ExecContext(ctx, `INSERT INTO account_groups(account_id,group_name)
			VALUES('41',?)`, groupName); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO health_samples(
		account_id,group_name,result,observed_at,source,evidence_key,payload_json
	) VALUES('41','codex','通过','2026-08-28T00:00:00Z','traffic','request-1','{}')`); err != nil {
		t.Fatal(err)
	}
	groupName := "pro"
	rows, err := store.RoutingSamples(ctx, nil, &groupName, "traffic", 60)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].AccountID != "41" || rows[0].GroupName != "codex" {
		t.Fatalf("secondary group did not receive account-level evidence: %#v", rows)
	}
}

func TestPreviousRoutingDecisionUsesAnySuccessfulWriteForSharedCooldown(t *testing.T) {
	store := openPolicyStore(t)
	ctx := context.Background()
	now := time.Date(2026, 8, 28, 1, 2, 3, 0, time.UTC).Format(time.RFC3339Nano)
	if _, err := store.db.ExecContext(ctx, `INSERT INTO routing_decisions(
		account_id,group_name,routing_state,updated_at,payload_json
	) VALUES('41','codex','healthy',?,'{}')`, now); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO operation_audit(
		source_id,operation_id,operation_type,state,phase,remote_confirmed,readback_confirmed,
		object_type,object_id,group_names_json,field_name,writeback,created_at
	) VALUES(-1,'op-1','routing.writeback','succeeded','readback',1,1,
		'account','41','["codex"]','priority',1,?)`, now); err != nil {
		t.Fatal(err)
	}

	rows, err := store.PreviousRoutingDecisions(ctx, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].LastApplyAt.IsZero() || rows[0].LastApplyAt.Format(time.RFC3339Nano) != now {
		t.Fatalf("priority-only writeback did not start shared cooldown: %#v", rows)
	}
}

func TestPreviousRoutingDecisionQueryUsesDedicatedAuditIndex(t *testing.T) {
	store := openPolicyStore(t)
	query, arguments := previousRoutingDecisionsQuery(nil, nil)
	rows, err := store.db.Query("EXPLAIN QUERY PLAN "+query, arguments...)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	indexUses := 0
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(detail, "ix_operation_audit_routing_lookup") {
			indexUses++
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if indexUses != 3 {
		t.Fatalf("routing history used dedicated audit index %d times, want 3", indexUses)
	}
}

func TestRoutingWritebackPendingUsesOrderedAuditIndex(t *testing.T) {
	store := openPolicyStore(t)
	rows, err := store.db.Query("EXPLAIN QUERY PLAN "+routingWritebackPendingSQL, routingCalculationKey)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	found := false
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(detail, "ix_operation_audit_type_object_recent") {
			found = true
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("routing writeback pending query did not use the ordered operation audit index")
	}
}
