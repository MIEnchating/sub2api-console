package business

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestManagementSnapshotUsesStableIDsAndPreservesLocalPolicyAndPartialFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "management.sqlite3")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	if err := store.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	result, err := store.SyncManagementSnapshot(ctx, []map[string]any{
		{
			"id": json.Number("11"), "name": "alpha", "schedulable": true,
			"priority": json.Number("20"), "load_factor": json.Number("3"), "rate_multiplier": json.Number("0.25"),
			"groups": []any{json.Number("7")}, "group_rate_by_group": map[string]any{"codex": json.Number("4")},
		},
		{"id": json.Number("12"), "name": "retained", "groups": []any{}},
	}, []map[string]any{
		{"id": json.Number("7"), "name": "codex", "platform": "openai", "rate_multiplier": json.Number("2"), "strategy": "price_first"},
	}, "tester")
	if err != nil {
		t.Fatal(err)
	}
	if result.Accounts != 2 || result.Groups != 1 || result.GroupLinks != 1 || result.RemoteWrite || !result.ReadOnly || result.EventID >= 0 {
		t.Fatalf("unexpected result: %#v", result)
	}
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`UPDATE local_groups SET strategy='reliability',strategy_source='group_override' WHERE remote_id='7'`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SyncManagementSnapshot(ctx, []map[string]any{
		{"id": json.Number("11"), "name": "alpha-renamed"},
	}, []map[string]any{
		{"id": json.Number("7"), "name": "codex", "strategy": "speed_first"},
	}, "tester"); err != nil {
		t.Fatal(err)
	}
	var name, loadFactor, multiplier string
	var priority int
	var schedulable bool
	if err := db.QueryRow(`SELECT name,schedulable,priority,load_factor,multiplier FROM accounts WHERE id='11'`).Scan(
		&name, &schedulable, &priority, &loadFactor, &multiplier,
	); err != nil {
		t.Fatal(err)
	}
	if name != "alpha-renamed" || !schedulable || priority != 20 || loadFactor != "3" || multiplier != "0.25" {
		t.Fatalf("partial snapshot erased fields: name=%q schedulable=%v priority=%d load=%q multiplier=%q", name, schedulable, priority, loadFactor, multiplier)
	}
	var retained int
	if err := db.QueryRow(`SELECT COUNT(*) FROM accounts WHERE id='12'`).Scan(&retained); err != nil || retained != 1 {
		t.Fatalf("missing partial account was deleted: count=%d err=%v", retained, err)
	}
	var strategy, source, platform, groupRate string
	if err := db.QueryRow(`SELECT strategy,strategy_source,platform,rate_multiplier FROM local_groups WHERE remote_id='7'`).Scan(
		&strategy, &source, &platform, &groupRate,
	); err != nil {
		t.Fatal(err)
	}
	if strategy != "reliability" || source != "group_override" {
		t.Fatalf("remote catalog overwrote local policy: strategy=%q source=%q", strategy, source)
	}
	if platform != "openai" || groupRate != "2" {
		t.Fatalf("partial group snapshot erased metadata: platform=%q rate=%q", platform, groupRate)
	}
	if _, err := store.SyncManagementSnapshot(ctx, []map[string]any{
		{"id": json.Number("11"), "rate_multiplier": json.Number("0.17")},
	}, []map[string]any{}, "tester"); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT group_rate FROM account_groups WHERE account_id='11' AND group_name='codex'`).Scan(&groupRate); err != nil {
		t.Fatal(err)
	}
	if groupRate != "0.17" {
		t.Fatalf("account multiplier update left stale membership rate: %q", groupRate)
	}
}

func TestManagementSnapshotKeepsCurrentAccountErrorAndClearsStaleError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "management-error.sqlite3")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	if err := store.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SyncManagementSnapshot(ctx, []map[string]any{{
		"id": json.Number("11"), "name": "alpha", "error_message": "API returned 503",
	}}, []map[string]any{}, "tester"); err != nil {
		t.Fatal(err)
	}
	var raw string
	if err := store.db.QueryRow(`SELECT metadata_json FROM accounts WHERE id='11'`).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(raw, `"error_message":"API returned 503"`) {
		t.Fatalf("current account error was not retained: %s", raw)
	}

	if _, err := store.SyncManagementSnapshot(ctx, []map[string]any{{
		"id": json.Number("11"), "name": "alpha",
	}}, []map[string]any{}, "tester"); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow(`SELECT metadata_json FROM accounts WHERE id='11'`).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(raw, "error_message") {
		t.Fatalf("stale account error was not cleared: %s", raw)
	}
}

func TestManagementSnapshotRollsBackOnInvalidStableIDOrField(t *testing.T) {
	path := filepath.Join(t.TempDir(), "management-invalid.sqlite3")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	if err := store.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	_, err = store.SyncManagementSnapshot(ctx, []map[string]any{
		{"id": json.Number("1"), "name": "valid"},
		{"id": "account-name", "name": "invalid"},
	}, []map[string]any{}, "tester")
	if err == nil || !strings.Contains(err.Error(), "稳定 ID") {
		t.Fatalf("err=%v", err)
	}
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var accounts int
	if err := db.QueryRow(`SELECT COUNT(*) FROM accounts`).Scan(&accounts); err != nil || accounts != 0 {
		t.Fatalf("invalid batch partially committed: accounts=%d err=%v", accounts, err)
	}
	_, err = store.SyncManagementSnapshot(ctx, []map[string]any{
		{"id": json.Number("1"), "name": "valid", "schedulable": "unknown"},
	}, []map[string]any{}, "tester")
	if err == nil || !strings.Contains(err.Error(), "schedulable") {
		t.Fatalf("err=%v", err)
	}
}
