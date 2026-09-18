package business_test

import (
	"database/sql"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

func healthDecisionStore(t *testing.T) (*business.Store, *sql.DB) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "health-decisions.db")
	store, err := business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Bootstrap(t.Context()); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return store, db
}

func TestPreviousHealthDecisionsPreservesStateAndScopeAfterEpoch(t *testing.T) {
	store, db := healthDecisionStore(t)
	if _, err := db.Exec(`
		INSERT INTO accounts(id,name,updated_at) VALUES('41','first','now'),('42','second','now');
		INSERT INTO account_groups(account_id,group_name) VALUES('41','primary'),('41','secondary'),('42','other');
		INSERT INTO app_state(key,value_json,updated_at) VALUES('routing-decision-epoch','{}','2026-09-18T01:00:00Z');
		INSERT INTO routing_decisions(account_id,group_name,priority,schedulable,routing_state,updated_at,payload_json) VALUES
		('43','expired',1,1,'healthy','2026-09-18T00:00:00Z','{}'),
		('41','primary',2,0,'fused','2026-09-18T01:00:00Z','{"state_since":"2026-09-18T00:30:00Z"}'),
		('42','other',4,1,'healthy','2026-09-18T02:00:00Z','{}')`); err != nil {
		t.Fatal(err)
	}
	account, missing, group := "41", "missing", "secondary"
	for _, scenario := range []struct {
		name           string
		account, group *string
		keys           []string
	}{
		{name: "all excludes old epoch", keys: []string{"41/primary", "42/other"}},
		{name: "account retains recovery state", account: &account, keys: []string{"41/primary"}},
		{name: "secondary group retains primary recovery state", group: &group, keys: []string{"41/primary"}},
		{name: "missing account is empty", account: &missing, keys: []string{}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			rows, err := store.PreviousHealthDecisions(t.Context(), scenario.account, scenario.group)
			if err != nil {
				t.Fatal(err)
			}
			keys := make([]string, 0, len(rows))
			for _, row := range rows {
				keys = append(keys, row.AccountID+"/"+row.GroupName)
			}
			if !reflect.DeepEqual(keys, scenario.keys) {
				t.Fatalf("keys=%v want=%v", keys, scenario.keys)
			}
			full, err := store.PreviousRoutingDecisions(t.Context(), scenario.account, scenario.group)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(rows, full) {
				t.Fatalf("health state differs from routing state: health=%+v routing=%+v", rows, full)
			}
		})
	}
}

func TestPreviousHealthDecisionsRejectsDamagedState(t *testing.T) {
	store, db := healthDecisionStore(t)
	if _, err := db.Exec(`INSERT INTO routing_decisions(account_id,group_name,updated_at,payload_json)
		VALUES('41','primary','2026-09-18T01:00:00Z','null')`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.PreviousHealthDecisions(t.Context(), nil, nil); err == nil {
		t.Fatal("damaged health state must not silently produce a score")
	}
}
