package evidence_test

import (
	"database/sql"
	"reflect"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/evidence"
)

func TestCostWallSuppressesAutomaticProbesBeforeSchedulingStateIsWritten(t *testing.T) {
	for _, rate := range []string{"1", "1.25"} {
		t.Run(rate, func(t *testing.T) {
			fixture := newProbeBatchFixture(t, []string{"41", "42"}, nil)
			db, err := sql.Open("sqlite", fixture.path)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Close() })
			if _, err := db.Exec(`INSERT INTO local_groups(name,remote_id,rate_multiplier,updated_at) VALUES('codex','7','1','now');
				UPDATE account_groups SET group_id='7';
				UPDATE accounts SET multiplier='0.5',schedulable=1`); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(`UPDATE accounts SET multiplier=? WHERE id='41'`, rate); err != nil {
				t.Fatal(err)
			}
			plan, err := fixture.service.Plan(t.Context(), fixture.policy, nil, nil, fixture.now)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(plan.ProbeAccountIDs, []string{"42"}) {
				t.Errorf("cost-wall account must not enter the probe plan: %+v", plan)
			}
			result, err := fixture.service.Collect(t.Context(), fixture.policy, fixture.admin, evidence.Options{ProbesAllowed: true, Now: fixture.now})
			if err != nil {
				t.Fatal(err)
			}
			if ids := fixture.probeIDs(); !reflect.DeepEqual(ids, []string{"42"}) {
				t.Errorf("cost-wall account must not consume probe requests: %v, result=%+v", ids, result)
			}
			var samples int
			if err := db.QueryRow(`SELECT COUNT(*) FROM health_samples WHERE account_id='41'`).Scan(&samples); err != nil {
				t.Fatal(err)
			}
			if samples != 0 {
				t.Errorf("suppressed probing must not create health evidence, got %d samples", samples)
			}
		})
	}
}

func TestCostWallOnlyConfirmedFallbackResumesAutomaticProbes(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		name := "unconfirmed fallback keeps probes stopped"
		if enabled {
			name = "confirmed fallback resumes probes"
		}
		t.Run(name, func(t *testing.T) {
			fixture := newProbeBatchFixture(t, []string{"41"}, nil)
			db, err := sql.Open("sqlite", fixture.path)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Close() })
			if _, err := db.Exec(`INSERT INTO local_groups(name,remote_id,rate_multiplier,updated_at) VALUES('codex','7','1','now');
				UPDATE account_groups SET group_id='7';
				UPDATE accounts SET multiplier='1.25',routing_state='survivor'`); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(`UPDATE accounts SET schedulable=?`, enabled); err != nil {
				t.Fatal(err)
			}
			plan, err := fixture.service.Plan(t.Context(), fixture.policy, nil, nil, fixture.now)
			if err != nil {
				t.Fatal(err)
			}
			if (len(plan.ProbeAccountIDs) == 1) != enabled {
				t.Fatalf("only a confirmed enabled fallback may be probed: enabled=%t plan=%+v", enabled, plan)
			}
		})
	}
}
