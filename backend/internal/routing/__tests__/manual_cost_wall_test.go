package routing_test

import (
	"github.com/MIEnchating/sub2api-console/backend/internal/routing"
	"testing"
	"time"
)

func TestManualCostWallBlocksUntilExplicitlyExemptAndRestoresOnlyItsOwnPause(t *testing.T) {
	store, db := costWallStore(t, "1.25")
	if _, err := store.AssignManualPriority(t.Context(), "41", 1, "10", 10, true, "test"); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name           string
		exempt, paused bool
		state          string
		want           int
		enabled        bool
	}{
		{name: "manual account still blocked", want: 1},
		{name: "explicit exemption", exempt: true},
		{name: "cost drop restores cost pause", paused: true, state: "cost_blocked", exempt: true, want: 1, enabled: true},
		{name: "ordinary manual pause never restored", paused: true, exempt: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := store.SetAccountIgnoreCostWall(t.Context(), "41", tc.exempt, "test"); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(`UPDATE accounts SET schedulable=?,routing_state=? WHERE id='41'`, !tc.paused, tc.state); err != nil {
				t.Fatal(err)
			}
			policy, err := store.ControlPolicy(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			accounts, err := store.RoutingAccounts(t.Context(), nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			changes, err := routing.PlanManualCostWall(policy, accounts, time.Now())
			if err != nil || len(changes) != tc.want {
				t.Fatalf("changes=%+v error=%v", changes, err)
			}
			if len(changes) > 0 && changes[0].Schedulable != tc.enabled {
				t.Fatalf("wrong scheduling: %+v", changes[0])
			}
		})
	}
}

func TestManualCostWallUsesAllCurrentMembershipsAndPreservesOtherStops(t *testing.T) {
	for _, tc := range []struct {
		name, sql string
		want      int
		enabled   bool
	}{
		{name: "equal cost stops", sql: `UPDATE accounts SET multiplier='1' WHERE id='41'`, want: 1},
		{name: "lower cost restores owned stop", sql: `UPDATE accounts SET multiplier='0.5',schedulable=0,routing_state='cost_blocked' WHERE id='41'`, want: 1, enabled: true},
		{name: "an affordable membership keeps scheduling", sql: `INSERT INTO local_groups(name,remote_id,rate_multiplier,updated_at) VALUES('premium','8','2','now'); INSERT INTO account_groups(account_id,group_name,group_id) VALUES('41','premium','8')`},
		{name: "missing price cannot authorize a stop", sql: `UPDATE local_groups SET rate_multiplier=NULL WHERE remote_id='7'`},
		{name: "manual pause is preserved", sql: `UPDATE accounts SET paused=1,schedulable=0,routing_state='cost_blocked',multiplier='0.5' WHERE id='41'`},
		{name: "fuse is preserved", sql: `UPDATE accounts SET schedulable=0,routing_state='fused',multiplier='0.5' WHERE id='41'`},
		{name: "platform disabled is preserved", sql: `UPDATE accounts SET schedulable=0,routing_state='cost_blocked',multiplier='0.5',metadata_json='{"status":"disabled"}' WHERE id='41'`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store, db := costWallStore(t, "1.25")
			if _, err := store.AssignManualPriority(t.Context(), "41", 1, "10", 10, true, "test"); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(tc.sql); err != nil {
				t.Fatal(err)
			}
			policy, err := store.ControlPolicy(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			accounts, err := store.RoutingAccounts(t.Context(), nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			changes, err := routing.PlanManualCostWall(policy, accounts, time.Now())
			if err != nil || len(changes) != tc.want {
				t.Fatalf("changes=%+v error=%v", changes, err)
			}
			if len(changes) > 0 && changes[0].Schedulable != tc.enabled {
				t.Fatalf("wrong scheduling: %+v", changes[0])
			}
		})
	}
}
