package business_test

import (
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/sqliteutil"
)

func TestProfileConcurrencySyncPersistsSharedBudgetAndExplicitUnlimited(t *testing.T) {
	store, db := concurrencyStore(t)
	for _, limit := range []int64{12, 0} {
		result, err := store.ApplyUpstreamSync(t.Context(), business.UpstreamSyncWrite{
			Host: "fixture.example", Balance: &business.UpstreamBalanceObservation{ConcurrencyLimit: &limit, ProfileUserID: "17"},
		})
		if err != nil {
			t.Fatal(err)
		}
		metadata := concurrencyMetadata(t, db)
		status := "known"
		if limit == 0 {
			status = "unlimited"
		}
		if metadata["concurrency_limit"] != float64(limit) || metadata["concurrency_status"] != status || metadata["concurrency_checked_at"] != result.CheckedAt || metadata["concurrency_user_id"] != "17" {
			t.Fatalf("profile limit did not survive persistence: %#v", metadata)
		}
	}
}

func TestMissingProfileConcurrencyKeepsPreviouslyKnownBudgetAsStale(t *testing.T) {
	store, db := concurrencyStore(t)
	seedConcurrency(t, db)
	_, err := store.ApplyUpstreamSync(t.Context(), business.UpstreamSyncWrite{
		Host: "fixture.example", Balance: &business.UpstreamBalanceObservation{ConcurrencyStatus: "unknown", ProfileUserID: "17"},
	})
	if err != nil {
		t.Fatal(err)
	}
	metadata := concurrencyMetadata(t, db)
	if metadata["concurrency_limit"] != float64(12) || metadata["concurrency_status"] != "stale" || metadata["concurrency_checked_at"] != "last-confirmed" {
		t.Fatalf("missing field erased or refreshed the known limit: %#v", metadata)
	}
}

func TestFailedBalanceSyncKeepsKnownConcurrencyWithoutClaimingItIsFresh(t *testing.T) {
	store, db := concurrencyStore(t)
	seedConcurrency(t, db)
	if err := store.RecordUpstreamSyncFailure(t.Context(), "fixture.example", "balance", "上游请求超时，请重试同步", false); err != nil {
		t.Fatal(err)
	}
	metadata := concurrencyMetadata(t, db)
	if metadata["concurrency_limit"] != float64(12) || metadata["concurrency_status"] != "stale" || metadata["concurrency_checked_at"] != "last-confirmed" {
		t.Fatalf("failed sync changed the effective budget or its confirmed time: %#v", metadata)
	}
}

func TestCatalogFailureDoesNotInvalidateConfirmedProfileConcurrency(t *testing.T) {
	store, db := concurrencyStore(t)
	seedConcurrency(t, db)
	if err := store.RecordUpstreamSyncFailure(t.Context(), "fixture.example", "catalog", "目录读取失败，请重试", false); err != nil {
		t.Fatal(err)
	}
	if metadata := concurrencyMetadata(t, db); metadata["concurrency_status"] != "known" {
		t.Fatalf("catalog-only failure invalidated profile data: %#v", metadata)
	}
}

func TestChangedProfileUserCannotInheritAnotherUsersConcurrencyBudget(t *testing.T) {
	store, db := concurrencyStore(t)
	seedConcurrency(t, db)
	_, err := store.ApplyUpstreamSync(t.Context(), business.UpstreamSyncWrite{
		Host: "fixture.example", Balance: &business.UpstreamBalanceObservation{ConcurrencyStatus: "unknown", ProfileUserID: "18"},
	})
	if err != nil {
		t.Fatal(err)
	}
	metadata := concurrencyMetadata(t, db)
	if _, found := metadata["concurrency_limit"]; found || metadata["concurrency_status"] != "unknown" || metadata["concurrency_user_id"] != "18" {
		t.Fatalf("changed user inherited stale capacity: %#v", metadata)
	}
}

func TestNeverObservedProfileConcurrencyStaysUnknownWithoutZeroBudget(t *testing.T) {
	store, db := concurrencyStore(t)
	_, err := store.ApplyUpstreamSync(t.Context(), business.UpstreamSyncWrite{
		Host: "fixture.example", Balance: &business.UpstreamBalanceObservation{ConcurrencyStatus: "unknown", ProfileUserID: "17"},
	})
	if err != nil {
		t.Fatal(err)
	}
	metadata := concurrencyMetadata(t, db)
	if _, found := metadata["concurrency_limit"]; found || metadata["concurrency_status"] != "unknown" {
		t.Fatalf("unknown field became unlimited: %#v", metadata)
	}
}

func concurrencyStore(t *testing.T) (*business.Store, *sql.DB) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "concurrency.db")
	store, err := business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Bootstrap(t.Context()); err != nil {
		t.Fatal(err)
	}
	_, err = store.CreateUpstreamConfiguration(t.Context(), business.UpstreamConfigurationWrite{
		Host: "fixture.example", BaseURL: "https://fixture.example", UpstreamType: "sub2api", AuthMode: "sub2api_user_token", RechargeRate: "1",
	})
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", sqliteutil.DSN(path, ""))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return store, db
}

func seedConcurrency(t *testing.T, db *sql.DB) {
	t.Helper()
	_, err := db.Exec(`UPDATE upstreams SET metadata_json='{"concurrency_limit":12,"concurrency_status":"known","concurrency_checked_at":"last-confirmed","concurrency_user_id":"17"}' WHERE host='fixture.example'`)
	if err != nil {
		t.Fatal(err)
	}
}

func concurrencyMetadata(t *testing.T, db *sql.DB) map[string]any {
	t.Helper()
	var raw string
	if err := db.QueryRow(`SELECT metadata_json FROM upstreams WHERE host='fixture.example'`).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var metadata map[string]any
	if err := json.Unmarshal([]byte(raw), &metadata); err != nil {
		t.Fatal(err)
	}
	return metadata
}
