package business_test

import (
	"context"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

func persistSnapshotStabilitySample(t *testing.T, store *business.Store) {
	t.Helper()
	_, err := store.PersistTrafficSamples(t.Context(), []business.TrafficSample{{
		AccountID: "41", GroupName: "codex", Result: "通过", EvidenceKey: "snapshot-sample",
		ObservedAt: time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano), Payload: map[string]any{},
	}})
	if err != nil {
		t.Fatal(err)
	}
}

func assertSnapshotStabilitySamples(t *testing.T, store *business.Store, ctx context.Context, expected int) {
	t.Helper()
	accounts, err := store.Accounts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, account := range accounts {
		if account.ID == "41" {
			if account.Stability == nil || account.Stability.Long.Samples != expected {
				t.Fatalf("stability sample count: got %+v, want %d", account.Stability, expected)
			}
			return
		}
	}
	t.Fatal("fixture account missing")
}

func TestReadSnapshotDoesNotConsumeNewerStabilityCache(t *testing.T) {
	store, _ := accountOwnershipFixture(t, 1)
	err := store.WithReadSnapshot(t.Context(), func(snapshot context.Context) error {
		persistSnapshotStabilitySample(t, store)
		assertSnapshotStabilitySamples(t, store, t.Context(), 1)
		assertSnapshotStabilitySamples(t, store, snapshot, 0)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestOldReadSnapshotDoesNotPublishStabilityCacheForCurrentReads(t *testing.T) {
	store, _ := accountOwnershipFixture(t, 1)
	err := store.WithReadSnapshot(t.Context(), func(snapshot context.Context) error {
		persistSnapshotStabilitySample(t, store)
		assertSnapshotStabilitySamples(t, store, snapshot, 0)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	assertSnapshotStabilitySamples(t, store, t.Context(), 1)
}

func TestStabilityCacheObservesCommittedSampleUpdatesAndDeletes(t *testing.T) {
	store, db := accountOwnershipFixture(t, 1)
	persistSnapshotStabilitySample(t, store)
	assertSnapshotStabilitySamples(t, store, t.Context(), 1)
	if _, err := db.Exec(`UPDATE account_stability_samples SET outcome='failed' WHERE account_id='41'`); err != nil {
		t.Fatal(err)
	}
	accounts, err := store.Accounts(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if accounts[0].Stability.Long.Failed != 1 {
		t.Fatalf("cached outcome did not follow committed update: %+v", accounts[0].Stability)
	}
	if _, err := db.Exec(`DELETE FROM account_stability_samples WHERE account_id='41'`); err != nil {
		t.Fatal(err)
	}
	assertSnapshotStabilitySamples(t, store, t.Context(), 0)
}

func TestExistingDatabaseAddsStabilityCacheVersionWithoutDiscardingSamples(t *testing.T) {
	store, db := accountOwnershipFixture(t, 1)
	persistSnapshotStabilitySample(t, store)
	var seq int
	var name, path string
	if err := db.QueryRow(`PRAGMA database_list`).Scan(&seq, &name, &path); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DROP TRIGGER IF EXISTS account_stability_insert_version;
DROP TRIGGER IF EXISTS account_stability_update_version;
DROP TRIGGER IF EXISTS account_stability_delete_version;
DROP TABLE IF EXISTS account_stability_version`); err != nil {
		t.Fatal(err)
	}
	reopened, err := business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	var revision int64
	if err := db.QueryRow(`SELECT revision FROM account_stability_version WHERE id=1`).Scan(&revision); err != nil {
		t.Fatalf("existing database did not gain stability version: %v", err)
	}
	assertSnapshotStabilitySamples(t, reopened, t.Context(), 1)
	if _, err := db.Exec(`DELETE FROM account_stability_samples WHERE account_id='41'`); err != nil {
		t.Fatal(err)
	}
	assertSnapshotStabilitySamples(t, reopened, t.Context(), 0)
}

func TestRolledBackStabilityUpdateKeepsVersionAndCommittedStatistics(t *testing.T) {
	store, db := accountOwnershipFixture(t, 1)
	persistSnapshotStabilitySample(t, store)
	var before int64
	if err := db.QueryRow(`SELECT revision FROM account_stability_version WHERE id=1`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	tx, err := db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM account_stability_samples WHERE account_id='41'`); err != nil {
		t.Fatal(err)
	}
	var during int64
	if err := tx.QueryRow(`SELECT revision FROM account_stability_version WHERE id=1`).Scan(&during); err != nil {
		t.Fatal(err)
	}
	if during <= before {
		t.Fatal("sample deletion did not advance its transaction's version")
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	var after int64
	if err := db.QueryRow(`SELECT revision FROM account_stability_version WHERE id=1`).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Fatalf("rolled back write changed revision: %d -> %d", before, after)
	}
	assertSnapshotStabilitySamples(t, store, t.Context(), 1)
}
