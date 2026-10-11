package business_test

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

func membershipSnapshotFixture(t testing.TB) (*business.Store, *sql.DB, []map[string]any) {
	t.Helper()
	store, db := pricingProjectionFixture(t)
	groups := []map[string]any{{"id": "7", "name": "first"}, {"id": "8", "name": "second"}}
	if _, err := store.SyncManagementSnapshot(t.Context(), []map[string]any{
		{"id": "41", "name": "member", "rate_multiplier": "0.1", "group_ids": []any{"7", "8"}},
	}, groups, "test"); err != nil {
		t.Fatal(err)
	}
	return store, db, groups
}

func TestRepeatedManagementMembershipsAvoidDeleteInsertAndStillRefreshCosts(t *testing.T) {
	store, db, groups := membershipSnapshotFixture(t)
	if _, err := db.Exec(`CREATE TRIGGER reject_member_delete BEFORE DELETE ON account_groups BEGIN SELECT RAISE(ABORT,'unchanged membership deleted'); END;
		CREATE TRIGGER reject_member_insert BEFORE INSERT ON account_groups BEGIN SELECT RAISE(ABORT,'unchanged membership inserted'); END;
		INSERT INTO routing_decisions(account_id,group_name,updated_at) VALUES('41','first','before');
		INSERT INTO account_health_evaluations(account_id,group_name,evaluated_at) VALUES('41','first','before')`); err != nil {
		t.Fatal(err)
	}
	result, err := store.SyncManagementSnapshot(t.Context(), []map[string]any{
		{"id": "41", "rate_multiplier": "0.25", "group_ids": []any{"8", "7", "8"}},
	}, groups, "test")
	if err != nil {
		t.Fatal(err)
	}
	if result.Accounts != 1 || result.GroupLinks != 2 || result.Groups != 2 {
		t.Fatalf("unchanged membership lost summary: %+v", result)
	}
	var links, projections int
	if err := db.QueryRow(`SELECT COUNT(*) FROM account_groups WHERE account_id='41' AND group_rate='0.25'`).Scan(&links); err != nil || links != 2 {
		t.Fatalf("cost refresh: links=%d error=%v", links, err)
	}
	if err := db.QueryRow(`SELECT (SELECT COUNT(*) FROM routing_decisions WHERE account_id='41')+(SELECT COUNT(*) FROM account_health_evaluations WHERE account_id='41')`).Scan(&projections); err != nil || projections != 2 {
		t.Fatalf("unchanged membership invalidated projections: count=%d error=%v", projections, err)
	}
}

func TestChangedManagementMembershipsReplaceLinksAndClearOldProjections(t *testing.T) {
	for _, tc := range []struct {
		name    string
		desired []any
	}{{"one-member", []any{"8"}}, {"ungrouped", []any{}}} {
		t.Run(tc.name, func(t *testing.T) {
			desired := tc.desired
			store, db, groups := membershipSnapshotFixture(t)
			if _, err := db.Exec(`INSERT INTO routing_decisions(account_id,group_name,updated_at) VALUES('41','first','before');
				INSERT INTO account_health_evaluations(account_id,group_name,evaluated_at) VALUES('41','first','before')`); err != nil {
				t.Fatal(err)
			}
			result, err := store.SyncManagementSnapshot(t.Context(), []map[string]any{{"id": "41", "group_ids": desired}}, groups, "test")
			if err != nil {
				t.Fatal(err)
			}
			var links, stale int
			if err := db.QueryRow(`SELECT COUNT(*) FROM account_groups WHERE account_id='41'`).Scan(&links); err != nil || links != len(desired) || result.GroupLinks != len(desired) {
				t.Fatalf("links=%d summary=%+v error=%v", links, result, err)
			}
			if err := db.QueryRow(`SELECT (SELECT COUNT(*) FROM routing_decisions WHERE account_id='41')+(SELECT COUNT(*) FROM account_health_evaluations WHERE account_id='41')`).Scan(&stale); err != nil || stale != 0 {
				t.Fatalf("stale projections=%d error=%v", stale, err)
			}
		})
	}
}

func TestManagementMembershipCanonicalizationStillReplacesWhitespace(t *testing.T) {
	for _, tc := range []struct {
		name, groupName string
		groupID         any
	}{
		{"whitespace-name", " first ", "7"}, {"whitespace-id", "first", " 7 "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store, db, groups := membershipSnapshotFixture(t)
			if _, err := db.Exec(`UPDATE account_groups SET group_name=?,group_id=? WHERE account_id='41' AND group_name='first'`, tc.groupName, tc.groupID); err != nil {
				t.Fatal(err)
			}
			if _, err := store.SyncManagementSnapshot(t.Context(), []map[string]any{{"id": "41", "group_ids": []any{"7", "8"}}}, groups, "test"); err != nil {
				t.Fatal(err)
			}
			var canonical int
			if err := db.QueryRow(`SELECT COUNT(*) FROM account_groups WHERE account_id='41' AND group_name='first' AND group_id='7'`).Scan(&canonical); err != nil || canonical != 1 {
				t.Fatalf("noncanonical member retained: count=%d error=%v", canonical, err)
			}
		})
	}
}

func TestManagementLegacyMembershipReplacesEmptyIDWithNullAndRestoresMissingGroup(t *testing.T) {
	store, db := pricingProjectionFixture(t)
	if _, err := db.Exec(`INSERT INTO accounts(id,name,multiplier,updated_at) VALUES('41','legacy-member','0.1','before');
		INSERT INTO account_groups(account_id,group_name,group_id) VALUES('41','legacy','')`); err != nil {
		t.Fatal(err)
	}
	result, err := store.SyncManagementSnapshot(t.Context(), []map[string]any{{"id": "41", "groups": []any{"legacy"}}}, nil, "test")
	if err != nil {
		t.Fatal(err)
	}
	var canonical, groups int
	if err := db.QueryRow(`SELECT COUNT(*) FROM account_groups WHERE account_id='41' AND group_name='legacy' AND group_id IS NULL`).Scan(&canonical); err != nil || canonical != 1 {
		t.Fatalf("empty ID not canonicalized: %d %v", canonical, err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM local_groups WHERE name='legacy' AND remote_id IS NULL AND account_count=1`).Scan(&groups); err != nil || groups != 1 || result.GroupLinks != 1 {
		t.Fatalf("legacy group not restored: %d %+v %v", groups, result, err)
	}
	if _, err := db.Exec(`DELETE FROM local_groups WHERE name='legacy';
		CREATE TRIGGER reject_member_delete BEFORE DELETE ON account_groups BEGIN SELECT RAISE(ABORT,'unchanged legacy member deleted'); END;
		CREATE TRIGGER reject_member_insert BEFORE INSERT ON account_groups BEGIN SELECT RAISE(ABORT,'unchanged legacy member inserted'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SyncManagementSnapshot(t.Context(), []map[string]any{{"id": "41", "groups": []any{"legacy"}}}, nil, "test"); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM local_groups WHERE name='legacy' AND account_count=1`).Scan(&groups); err != nil || groups != 1 {
		t.Fatalf("unchanged membership skipped group restoration: %d %v", groups, err)
	}
}

func TestUnchangedManagementMembershipCostAndAuditFailureRollBackTogether(t *testing.T) {
	store, db, groups := membershipSnapshotFixture(t)
	if _, err := db.Exec(`CREATE TRIGGER reject_snapshot BEFORE INSERT ON runtime_events WHEN NEW.event_type='management.snapshot.synced' BEGIN SELECT RAISE(ABORT,'snapshot audit unavailable'); END`); err != nil {
		t.Fatal(err)
	}
	_, err := store.SyncManagementSnapshot(t.Context(), []map[string]any{{"id": "41", "rate_multiplier": "0.25", "group_ids": []any{"7", "8"}}}, groups, "test")
	if err == nil || !strings.Contains(err.Error(), "snapshot audit unavailable") {
		t.Fatalf("audit failure=%v", err)
	}
	var multiplier string
	var links int
	if err := db.QueryRow(`SELECT multiplier FROM accounts WHERE id='41'`).Scan(&multiplier); err != nil || multiplier != "0.1" {
		t.Fatalf("cost escaped rollback: %s %v", multiplier, err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM account_groups WHERE account_id='41' AND group_rate='0.1'`).Scan(&links); err != nil || links != 2 {
		t.Fatalf("membership cost escaped rollback: %d %v", links, err)
	}
}
