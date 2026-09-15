package routing_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/routing"
)

func controlScopeStore(t *testing.T) (*business.Store, *sql.DB) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "control-scope.sqlite3")
	store, err := business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Bootstrap(context.Background()); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return store, db
}

func TestRoutingGroupScopeUsesStableIDDespiteNumericGroupName(t *testing.T) {
	for _, excluded := range []bool{true, false} {
		name := "selected group"
		if excluded {
			name = "excluded group"
		}
		t.Run(name, func(t *testing.T) {
			store, db := controlScopeStore(t)
			_, err := db.Exec(`INSERT INTO accounts(id,name,multiplier,schedulable,metadata_json,updated_at)
				VALUES('41','selected','1',1,'{}','now'),('42','numeric-name','1',1,'{}','now');
				INSERT INTO account_groups(account_id,group_name,group_id) VALUES('41','codex','7'),('42','7','8')`)
			if err != nil {
				t.Fatal(err)
			}
			policy := map[string]any{"advanced_policy": map[string]any{"scope": map[string]any{"managed_group_mode": "selected", "managed_group_ids": []any{"7"}}}}
			if excluded {
				policy = map[string]any{"excluded_group_ids": []any{"7"}}
			}
			if _, err := store.UpdatePolicy(context.Background(), policy, "test"); err != nil {
				t.Fatal(err)
			}
			result, err := routing.NewService(store).Calculate(context.Background(), routing.Scope{}, true)
			if err != nil {
				t.Fatal(err)
			}
			if result.AccountTargets["41"].ReleaseControl != excluded || result.AccountTargets["42"].ReleaseControl == excluded {
				t.Fatalf("scope matched a group name instead of its stable ID: %+v", result.AccountTargets)
			}
		})
	}
}

func TestRoutingControlComparesLoadFactorByDecimalValue(t *testing.T) {
	for _, test := range []struct {
		name    string
		current any
		abandon bool
	}{
		{name: "equivalent trailing zeros", current: "1.00"},
		{name: "equivalent exponent", current: "1e0"},
		{name: "actual external change", current: "1.1", abandon: true},
		{name: "missing current load", current: nil, abandon: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			store, db := controlScopeStore(t)
			_, err := db.Exec(`INSERT INTO accounts(id,name,multiplier,schedulable,load_factor,metadata_json,updated_at)
				VALUES('41','managed','1',1,?,'{}','now')`, test.current)
			if err != nil {
				t.Fatal(err)
			}
			_, err = db.Exec(`INSERT INTO account_groups(account_id,group_name,group_id) VALUES('41','codex','7');
				INSERT INTO routing_baselines(account_id,captured_at,managed_load_factor) VALUES('41','now','1')`)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := store.UpdatePolicy(context.Background(), map[string]any{"advanced_policy": map[string]any{"scope": map[string]any{"manage_all_accounts": false}}}, "test"); err != nil {
				t.Fatal(err)
			}
			result, err := routing.NewService(store).Calculate(context.Background(), routing.Scope{}, true)
			if err != nil {
				t.Fatal(err)
			}
			if target := result.AccountTargets["41"]; target.AbandonControl != test.abandon {
				t.Fatalf("decimal comparison produced an incorrect control target: %+v", target)
			}
		})
	}
}
