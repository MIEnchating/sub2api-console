package business_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

func TestAccountControlStateMatchesDetailProtectionAcrossModes(t *testing.T) {
	for _, mode := range []string{"完全模式", "监控模式"} {
		for _, scenario := range []struct {
			name, update string
			excluded     bool
		}{
			{"healthy", `UPDATE accounts SET schedulable=1,routing_state='healthy'`, false},
			{"fused", `UPDATE accounts SET schedulable=0,routing_state='fused'`, false},
			{"paused", `UPDATE accounts SET paused=1,schedulable=0`, false},
			{"cost blocked", `UPDATE accounts SET schedulable=0,routing_state='cost_blocked'`, false},
			{"concurrency limited", `UPDATE accounts SET schedulable=0,routing_state='concurrency_limited'`, false},
			{"disabled", `UPDATE accounts SET metadata_json='{"status":"disabled"}',schedulable=0`, false},
			{"manual", `INSERT INTO manual_priority_accounts(account_id,priority,previous_priority,previous_load_factor,previous_concurrency,previous_schedulable,created_at,updated_at) VALUES('41',1,10,'100',10,1,'now','now')`, false},
			{"excluded", `UPDATE accounts SET schedulable=1,routing_state='excluded'`, true},
		} {
			t.Run(mode+"/"+scenario.name, func(t *testing.T) {
				store, db := monitoringProjectionStore(t)
				if _, err := store.SetMode(t.Context(), mode); err != nil {
					t.Fatal(err)
				}
				if _, err := db.Exec(scenario.update); err != nil {
					t.Fatal(err)
				}
				if scenario.excluded {
					if _, err := store.SetAccountScopeControl(t.Context(), "41", "exclude", "test"); err != nil {
						t.Fatal(err)
					}
				}
				detail, err := store.Account(t.Context(), "41")
				if err != nil {
					t.Fatal(err)
				}
				state, err := store.AccountControlState(t.Context(), "41")
				if err != nil {
					t.Fatal(err)
				}
				if scenario.excluded && detail.Health != "excluded" {
					t.Fatalf("fixture did not protect excluded account: %s", detail.Health)
				}
				want := business.AccountControlState{ID: detail.ID, Name: detail.Name, Health: detail.Health, HealthStatus: detail.HealthStatus, RoutingState: detail.RoutingState, Paused: detail.Paused, Schedulable: detail.Schedulable, ManualPriority: detail.ManualPriority}
				if !reflect.DeepEqual(*state, want) {
					t.Fatalf("state=%#v differs from detail=%#v", *state, want)
				}
			})
		}
	}
}

func TestAccountControlStateRejectsInvalidOrMissingAccount(t *testing.T) {
	store, _ := monitoringProjectionStore(t)
	for _, id := range []string{"", "name", "0", "041"} {
		if _, err := store.AccountControlState(t.Context(), id); err == nil {
			t.Fatalf("accepted invalid id %q", id)
		}
	}
	if _, err := store.AccountControlState(t.Context(), "999"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("missing account: %v", err)
	}
}

func BenchmarkAccountControlState(b *testing.B) {
	path := filepath.Join(b.TempDir(), "control.db")
	store, err := business.Open(path)
	if err != nil {
		b.Fatal(err)
	}
	defer store.Close()
	if err := store.Bootstrap(context.Background()); err != nil {
		b.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		b.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO accounts(id,name,schedulable,routing_state,metadata_json,updated_at) VALUES('41','control',1,'healthy',?,'now')`, `{"status":"active","notes":"`+strings.Repeat("x", 64<<10)+`"}`); err != nil {
		b.Fatal(err)
	}
	for _, name := range []string{"detail", "control-state"} {
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if name == "detail" {
					_, err = store.Account(context.Background(), "41")
				} else {
					_, err = store.AccountControlState(context.Background(), "41")
				}
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
