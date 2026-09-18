package business_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/sqliteutil"
)

func readSnapshotStore(t *testing.T, name string) (*business.Store, *sql.DB) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "snapshot.sqlite3")
	store, err := business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	db, err := sql.Open("sqlite", sqliteutil.DSN(path, "_pragma=busy_timeout%281000%29"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.ExecContext(t.Context(), `INSERT INTO app_state(key,value_json,updated_at)
		VALUES('model-check-configuration',?,'original');
		INSERT INTO health_samples(account_id,group_name,result,observed_at,source,evidence_key)
		VALUES('41','codex','passed','2026-09-17T07:00:00.000000000Z','traffic','original')`, name)
	if err != nil {
		t.Fatal(err)
	}
	return store, db
}

func TestReadSnapshotKeepsEvidenceAndConfigurationStableWhileExternalWritesCommit(t *testing.T) {
	store, db := readSnapshotStore(t, `{"version":1}`)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	err := store.WithReadSnapshot(ctx, func(snapshot context.Context) error {
		capturedAt, found := store.ReadSnapshotTime(snapshot)
		if !found || capturedAt.IsZero() {
			t.Fatal("snapshot has no acquisition time")
		}
		before, err := store.RecentAccountResults(snapshot, "41", 10)
		if err != nil || len(before) != 1 {
			t.Fatalf("initial evidence=%+v error=%v", before, err)
		}
		_, err = db.ExecContext(ctx, `INSERT INTO health_samples(account_id,group_name,result,observed_at,source,evidence_key)
			VALUES('41','codex','failed','2026-09-17T07:01:00.000000000Z','traffic','new');
			UPDATE app_state SET value_json='{"version":2}' WHERE key='model-check-configuration'`)
		if err != nil {
			t.Fatalf("active snapshot blocked the external writer: %v", err)
		}
		return store.WithReadSnapshot(snapshot, func(nested context.Context) error {
			if nestedAt, found := store.ReadSnapshotTime(nested); !found || !nestedAt.Equal(capturedAt) {
				t.Fatal("nested snapshot changed its acquisition time")
			}
			after, err := store.RecentAccountResults(nested, "41", 10)
			if err != nil || len(after) != 1 || after[0].ID != before[0].ID {
				t.Errorf("nested snapshot mixed newly committed evidence into the initial batch: results=%+v error=%v", after, err)
			}
			configuration, err := store.LoadModelCheckConfiguration(nested)
			if err != nil || string(configuration) != `{"version":1}` {
				t.Errorf("single-row read escaped the evidence snapshot: configuration=%s error=%v", configuration, err)
			}
			return nil
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	after, err := store.RecentAccountResults(ctx, "41", 10)
	if err != nil || len(after) != 2 {
		t.Fatalf("finished snapshot hid the committed evidence: results=%+v error=%v", after, err)
	}
	configuration, err := store.LoadModelCheckConfiguration(ctx)
	if err != nil || string(configuration) != `{"version":2}` {
		t.Fatalf("finished snapshot hid the committed configuration: configuration=%s error=%v", configuration, err)
	}
}

func TestReadSnapshotIsPinnedBeforeCallbackCanObserveConcurrentWrites(t *testing.T) {
	store, db := readSnapshotStore(t, `{"version":1}`)
	err := store.WithReadSnapshot(t.Context(), func(snapshot context.Context) error {
		if _, err := db.ExecContext(t.Context(), `UPDATE app_state SET value_json='{"version":2}' WHERE key='model-check-configuration'`); err != nil {
			return err
		}
		configuration, err := store.LoadModelCheckConfiguration(snapshot)
		if err != nil || string(configuration) != `{"version":1}` {
			t.Errorf("callback's first read acquired a newer snapshot: configuration=%s error=%v", configuration, err)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestReadSnapshotContextKeepsDifferentStoresIsolatedWhenNested(t *testing.T) {
	first, _ := readSnapshotStore(t, `{"store":"first"}`)
	second, _ := readSnapshotStore(t, `{"store":"second"}`)
	err := first.WithReadSnapshot(t.Context(), func(firstContext context.Context) error {
		firstTime, found := first.ReadSnapshotTime(firstContext)
		if !found || firstTime.IsZero() {
			t.Fatal("first snapshot has no acquisition time")
		}
		if _, found := second.ReadSnapshotTime(firstContext); found {
			t.Fatal("first store timestamp leaked into a second store")
		}
		configuration, err := second.LoadModelCheckConfiguration(firstContext)
		if err != nil || string(configuration) != `{"store":"second"}` {
			t.Fatalf("first store snapshot leaked into a second store read: configuration=%s error=%v", configuration, err)
		}
		return second.WithReadSnapshot(firstContext, func(secondContext context.Context) error {
			secondTime, found := second.ReadSnapshotTime(secondContext)
			if !found || secondTime.Before(firstTime) {
				t.Fatal("second store has no independent acquisition time")
			}
			retainedTime, found := first.ReadSnapshotTime(secondContext)
			if !found || !retainedTime.Equal(firstTime) {
				t.Fatal("nested store replaced the first store's acquisition time")
			}
			for store, expected := range map[*business.Store]string{first: `{"store":"first"}`, second: `{"store":"second"}`} {
				configuration, err := store.LoadModelCheckConfiguration(secondContext)
				if err != nil || string(configuration) != expected {
					t.Errorf("nested store read used another database: configuration=%s expected=%s error=%v", configuration, expected, err)
				}
			}
			return nil
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, found := first.ReadSnapshotTime(t.Context()); found {
		t.Fatal("snapshot acquisition time escaped its context")
	}
}

func TestReadSnapshotReturnsCallbackErrorAndClosesTransaction(t *testing.T) {
	store, _ := readSnapshotStore(t, `{}`)
	failure := errors.New("isolated read failure")
	var snapshotContext context.Context
	err := store.WithReadSnapshot(t.Context(), func(ctx context.Context) error {
		snapshotContext = ctx
		if _, err := store.LoadModelCheckConfiguration(ctx); err != nil {
			t.Fatal(err)
		}
		return failure
	})
	if !errors.Is(err, failure) {
		t.Fatalf("callback failure was replaced: %v", err)
	}
	if _, err := store.LoadModelCheckConfiguration(snapshotContext); !errors.Is(err, sql.ErrTxDone) {
		t.Fatalf("completed callback retained a usable transaction: %v", err)
	}
	if _, err := store.LoadModelCheckConfiguration(t.Context()); err != nil {
		t.Fatalf("failed snapshot affected later reads: %v", err)
	}
}

func TestReadSnapshotCancellationReturnsContextErrorAndAllowsLaterReads(t *testing.T) {
	store, _ := readSnapshotStore(t, `{}`)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	err := store.WithReadSnapshot(ctx, func(snapshot context.Context) error {
		if _, err := store.LoadModelCheckConfiguration(snapshot); err != nil {
			t.Fatal(err)
		}
		cancel()
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled snapshot succeeded: %v", err)
	}
	called := false
	err = store.WithReadSnapshot(ctx, func(context.Context) error {
		called = true
		return nil
	})
	if called || !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled snapshot ran its callback: called=%t error=%v", called, err)
	}
	if _, err := store.LoadModelCheckConfiguration(t.Context()); err != nil {
		t.Fatalf("cancelled snapshot affected later reads: %v", err)
	}
}
