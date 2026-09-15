package business_test

import (
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

func TestGroupAllocationReportsEffectiveCapacityWhenTheDecisionKeepsOrChangesConcurrency(t *testing.T) {
	for _, fixture := range []struct {
		name        string
		current     any
		schedulable int
		desired     string
		wantChannel string
		wantTotal   string
	}{
		{name: "unchanged finite configuration remains allocated", current: 32, schedulable: 1, desired: "null", wantChannel: "32", wantTotal: "40"},
		{name: "explicit target overrides current configuration", current: 32, schedulable: 1, desired: "40", wantChannel: "40", wantTotal: "48"},
		{name: "capacity pause contributes zero despite retained configuration", current: 32, schedulable: 0, desired: "16", wantChannel: "0", wantTotal: "8"},
		{name: "missing active configuration keeps total unknown", current: nil, schedulable: 1, desired: "null", wantChannel: "null", wantTotal: "null"},
		{name: "unlimited active configuration keeps total unknown", current: 0, schedulable: 1, desired: "null", wantChannel: "null", wantTotal: "null"},
		{name: "unknown scheduling state keeps total unknown", current: 32, schedulable: 2, desired: "40", wantChannel: "null", wantTotal: "null"},
		{name: "invalid target does not silently inherit current configuration", current: 32, schedulable: 1, desired: "1.5", wantChannel: "null", wantTotal: "null"},
		{name: "large target retains exact integer precision", current: 32, schedulable: 1, desired: "9007199254740993", wantChannel: "9007199254740993", wantTotal: "9007199254741001"},
		{name: "overflowing total remains unknown", current: 32, schedulable: 1, desired: "9223372036854775807", wantChannel: "9223372036854775807", wantTotal: "null"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			store, db := allocationCapacityStore(t)
			if _, err := db.Exec(`INSERT INTO accounts(id,name,concurrency,schedulable,updated_at) VALUES
				('41','primary',?,1,'now'),('42','secondary',8,1,'now')`, fixture.current); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(`INSERT INTO account_groups(account_id,group_name,group_id) VALUES('41','codex','1'),('42','codex','1')`); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(`INSERT INTO routing_decisions(account_id,group_name,schedulable,routing_state,updated_at,payload_json)
				VALUES('41','codex',?,'healthy','2099-01-01T00:00:00Z',?),('42','codex',1,'healthy','2099-01-01T00:00:00Z','{}')`,
				fixture.schedulable, `{"desired_concurrency":`+fixture.desired+`}`); err != nil {
				t.Fatal(err)
			}
			allocation, err := store.GroupAllocation(t.Context(), "1")
			if err != nil {
				t.Fatal(err)
			}
			if len(allocation.Channels) != 2 || allocation.Channels[0].AccountID != "41" {
				t.Fatalf("unexpected group channels: %+v", allocation.Channels)
			}
			channel, err := json.Marshal(allocation.Channels[0].AssignedConcurrency)
			if err != nil {
				t.Fatal(err)
			}
			total, err := json.Marshal(allocation.AssignedConcurrency)
			if err != nil {
				t.Fatal(err)
			}
			if string(channel) != fixture.wantChannel || string(total) != fixture.wantTotal {
				t.Fatalf("capacity channel=%s total=%s, want channel=%s total=%s", channel, total, fixture.wantChannel, fixture.wantTotal)
			}
		})
	}
}

func allocationCapacityStore(t *testing.T) (*business.Store, *sql.DB) {
	path := filepath.Join(t.TempDir(), "allocation.sqlite3")
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
	if _, err := db.Exec(`INSERT INTO local_groups(name,remote_id,account_count,updated_at) VALUES('codex','1',2,'now')`); err != nil {
		t.Fatal(err)
	}
	return store, db
}
