package business_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/sqliteutil"
)

func TestExpiredInspectionLeaseCannotBeRenewedByItsOriginalOwner(t *testing.T) {
	store, err := business.Open(filepath.Join(t.TempDir(), "expired-lease.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	now := time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC)
	acquired, err := store.AcquireInspectionLease(ctx, "original-owner", 1, "isolated.example", now, now, time.Minute)
	if err != nil || !acquired {
		t.Fatalf("acquire lease: acquired=%v, %v", acquired, err)
	}
	renewed, err := store.RenewInspectionLease(ctx, "original-owner", now.Add(time.Minute), time.Minute)
	if err != nil || renewed {
		t.Fatalf("expired lease renewed: renewed=%v, %v", renewed, err)
	}
	active, err := store.ActiveInspectionCheckedAt(ctx, now.Add(time.Minute))
	if err != nil || active != nil {
		t.Fatalf("expired owner became active: checkedAt=%v, %v", active, err)
	}
}

func TestCorruptInspectionTaskStateIsRejectedWithoutOverwritingCooldowns(t *testing.T) {
	for _, raw := range []string{"{broken", "null"} {
		t.Run(raw, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "inspection-state.db")
			store, err := business.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = store.Close() })
			db, err := sql.Open("sqlite", sqliteutil.DSN(path, ""))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Close() })
			ctx := context.Background()
			if _, err := db.ExecContext(ctx, `INSERT INTO app_state(key,value_json,updated_at)
				VALUES('auto-inspection-heartbeat',?,'original')`, raw); err != nil {
				t.Fatal(err)
			}
			now := time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC)
			if due, err := store.InspectionTaskDue(ctx, "price-management", 120, now); err == nil || due {
				t.Errorf("corrupt cooldown state treated as due: due=%v, %v", due, err)
			}
			if err := store.MarkInspectionTask(ctx, "price-management", now); err == nil {
				t.Error("corrupt cooldown state accepted a new mark")
			}
			if err := store.ResetInspectionTask(ctx, "price-management", now); err == nil {
				t.Error("corrupt cooldown state accepted a reset")
			}
			var stored string
			if err := db.QueryRowContext(ctx, `SELECT value_json FROM app_state WHERE key='auto-inspection-heartbeat'`).Scan(&stored); err != nil {
				t.Fatal(err)
			}
			if stored != raw {
				t.Fatalf("corrupt cooldown state was overwritten: got=%q, want=%q", stored, raw)
			}
		})
	}
}
