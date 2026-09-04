package business

import (
	"context"
	"testing"
)

func TestAccountMultiplierChangeAlertsRequireANumericChange(t *testing.T) {
	store := openPolicyStore(t)
	ctx := context.Background()
	if _, err := store.db.ExecContext(ctx, `INSERT INTO accounts(id,name,multiplier,metadata_json,updated_at)
		VALUES('41','rate-account','0.15','{}','now')`); err != nil {
		t.Fatal(err)
	}
	recordMultiplierAudit := func(operationID, before, after string) {
		t.Helper()
		if err := store.RecordAccountOperation(ctx, AccountOperation{
			OperationID: operationID, OperationType: "account.sync", State: "succeeded", Phase: "readback",
			Actor: "scheduler", RemoteConfirmed: true, ReadbackConfirmed: true, ObjectID: "41", Writeback: true,
			Before: map[string]any{"rate_multiplier": before}, After: map[string]any{"rate_multiplier": after},
		}); err != nil {
			t.Fatal(err)
		}
	}

	recordMultiplierAudit("same-rate", "0.15", "0.1500")
	var count int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM alert_incidents`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("equivalent multiplier values created %d alerts", count)
	}

	recordMultiplierAudit("rate-up", "0.15", "0.2")
	recordMultiplierAudit("rate-down", "0.2", "0.1")
	rows, err := store.db.QueryContext(ctx, `SELECT event_type,cause_code FROM alert_incidents ORDER BY event_type`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	want := []struct{ eventType, causeCode string }{
		{"account.multiplier_decreased", "MULTIPLIER_DECREASED:0.2 -> 0.1"},
		{"account.multiplier_increased", "MULTIPLIER_INCREASED:0.15 -> 0.2"},
	}
	for _, expected := range want {
		if !rows.Next() {
			t.Fatalf("missing multiplier alert %#v", expected)
		}
		var eventType, causeCode string
		if err := rows.Scan(&eventType, &causeCode); err != nil {
			t.Fatal(err)
		}
		if eventType != expected.eventType || causeCode != expected.causeCode {
			t.Fatalf("multiplier alert=(%q,%q), want=(%q,%q)", eventType, causeCode, expected.eventType, expected.causeCode)
		}
	}
	if rows.Next() {
		t.Fatal("unexpected extra multiplier alert")
	}
}

func TestAccountMultiplierChangeAlertDirectionSwitchesAreIndependent(t *testing.T) {
	store := openPolicyStore(t)
	ctx := context.Background()
	payload := alertPolicyDocument(DefaultAlertPolicy())
	payload["multiplier_increase_enabled"] = false
	payload["multiplier_decrease_enabled"] = true
	if _, err := store.UpdateAlertPolicy(ctx, payload); err != nil {
		t.Fatal(err)
	}
	for _, operation := range []AccountOperation{
		{
			OperationID: "disabled-up", OperationType: "account.sync", State: "succeeded", Phase: "readback",
			RemoteConfirmed: true, ReadbackConfirmed: true, ObjectID: "41", Writeback: true,
			Before: map[string]any{"rate_multiplier": "0.1"}, After: map[string]any{"rate_multiplier": "0.2"},
		},
		{
			OperationID: "enabled-down", OperationType: "account.sync", State: "succeeded", Phase: "readback",
			RemoteConfirmed: true, ReadbackConfirmed: true, ObjectID: "41", Writeback: true,
			Before: map[string]any{"rate_multiplier": "0.2"}, After: map[string]any{"rate_multiplier": "0.1"},
		},
	} {
		if err := store.RecordAccountOperation(ctx, operation); err != nil {
			t.Fatal(err)
		}
	}
	var eventType string
	if err := store.db.QueryRowContext(ctx, `SELECT event_type FROM alert_incidents`).Scan(&eventType); err != nil {
		t.Fatal(err)
	}
	if eventType != "account.multiplier_decreased" {
		t.Fatalf("independent direction switches produced %q", eventType)
	}
	var increased int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM alert_incidents WHERE event_type='account.multiplier_increased'`).Scan(&increased); err != nil {
		t.Fatal(err)
	}
	if increased != 0 {
		t.Fatal("disabled multiplier increase notification was created")
	}
}

func TestAccountMultiplierChangeAlertsRequireConfirmedSuccessfulReadback(t *testing.T) {
	store := openPolicyStore(t)
	ctx := context.Background()
	operations := []AccountOperation{
		{
			OperationID: "failed-sync", OperationType: "account.sync", State: "failed", Phase: "readback",
			RemoteConfirmed: true, ReadbackConfirmed: true, ObjectID: "41",
			Before: map[string]any{"rate_multiplier": "0.1"}, After: map[string]any{"rate_multiplier": "0.2"},
		},
		{
			OperationID: "unconfirmed-write", OperationType: "account.sync", State: "succeeded", Phase: "remote-write",
			RemoteConfirmed: false, ReadbackConfirmed: false, ObjectID: "41",
			Before: map[string]any{"rate_multiplier": "0.1"}, After: map[string]any{"rate_multiplier": "0.2"},
		},
		{
			OperationID: "invalid-rate", OperationType: "account.sync", State: "succeeded", Phase: "readback",
			RemoteConfirmed: true, ReadbackConfirmed: true, ObjectID: "41",
			Before: map[string]any{"rate_multiplier": "not-a-rate"}, After: map[string]any{"rate_multiplier": "0.2"},
		},
	}
	for _, operation := range operations {
		if err := store.RecordAccountOperation(ctx, operation); err != nil {
			t.Fatal(err)
		}
	}
	var count int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM alert_incidents`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("unconfirmed or invalid multiplier changes created %d alerts", count)
	}
}

func TestAccountGroupReadbackCopiesAccountCostInsteadOfGroupSalePrice(t *testing.T) {
	store := openReadModelFixture(t)
	ctx := context.Background()
	if _, err := store.db.ExecContext(ctx, `UPDATE local_groups SET rate_multiplier='3' WHERE remote_id='2'`); err != nil {
		t.Fatal(err)
	}
	operation := AccountOperation{
		OperationID: "groups-41", OperationType: "account.groups.sync", State: "succeeded", Phase: "readback",
		Actor: "operator", RemoteConfirmed: true, ReadbackConfirmed: true, ObjectID: "41", Writeback: true,
	}
	if err := store.CommitAccountGroupsReadback(ctx, "41", []LocalOnboardingGroup{{ID: "2", Name: "pro"}}, nil, operation); err != nil {
		t.Fatal(err)
	}
	var groupRate string
	if err := store.db.QueryRowContext(ctx, `SELECT group_rate FROM account_groups WHERE account_id='41' AND group_id='2'`).Scan(&groupRate); err != nil {
		t.Fatal(err)
	}
	if groupRate != "0.1" {
		t.Fatalf("group membership rate=%q, want account cost 0.1", groupRate)
	}
}

func TestAccountHostEditPreservesStableBindingIdentity(t *testing.T) {
	store := openReadModelFixture(t)
	ctx := context.Background()
	if err := store.ensureStableUpstreamRelations(ctx); err != nil {
		t.Fatal(err)
	}
	var beforeID string
	if err := store.db.QueryRowContext(ctx, `SELECT bi.upstream_id FROM binding_identities bi
		JOIN bindings b ON b.id=bi.binding_id WHERE b.local_account_id='41' LIMIT 1`).Scan(&beforeID); err != nil {
		t.Fatal(err)
	}
	host := "new-address.example.test"
	operation := AccountOperation{
		OperationID: "host-41", OperationType: "account.fields.sync", State: "succeeded", Phase: "readback",
		Actor: "operator", RemoteConfirmed: true, ReadbackConfirmed: true, ObjectID: "41", Writeback: true,
	}
	if err := store.CommitAccountFieldsReadback(ctx, "41", nil, nil, nil, nil, nil, &host, nil, false, nil, operation); err != nil {
		t.Fatal(err)
	}
	var recordedHost, afterID string
	if err := store.db.QueryRowContext(ctx, `SELECT upstream_host FROM accounts WHERE id='41'`).Scan(&recordedHost); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRowContext(ctx, `SELECT bi.upstream_id FROM binding_identities bi
		JOIN bindings b ON b.id=bi.binding_id WHERE b.local_account_id='41' LIMIT 1`).Scan(&afterID); err != nil {
		t.Fatal(err)
	}
	if recordedHost != host || afterID != beforeID {
		t.Fatalf("recorded host=%q binding identity before=%q after=%q", recordedHost, beforeID, afterID)
	}
}
