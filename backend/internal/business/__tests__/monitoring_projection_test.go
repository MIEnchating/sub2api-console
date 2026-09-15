package business_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/sqliteutil"
)

func TestMonitoringProjectionHonorsZeroDegradeThreshold(t *testing.T) {
	store, db := monitoringProjectionStore(t)
	ctx := context.Background()
	if _, err := store.UpdatePolicy(ctx, map[string]any{
		"advanced_policy": map[string]any{"degrade": map[string]any{"score_threshold": 0}},
	}, "operator"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO account_health_evaluations(account_id,group_name,health_score,sample_count,evaluated_at)
		VALUES('41','fixture',50,1,'2026-09-14T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	account, err := store.Account(ctx, "41")
	if err != nil {
		t.Fatal(err)
	}
	if account.Health != business.AccountStateHealthy {
		t.Fatalf("health with score 50 and threshold 0 = %q, want healthy", account.Health)
	}
}

func TestMonitoringProjectionPreservesManagementDisabledStatusWithoutSamples(t *testing.T) {
	store, db := monitoringProjectionStore(t)
	if _, err := db.Exec(`UPDATE accounts SET schedulable=0,metadata_json='{"status":"disabled"}' WHERE id='41'`); err != nil {
		t.Fatal(err)
	}
	account, err := store.Account(context.Background(), "41")
	if err != nil {
		t.Fatal(err)
	}
	if account.Health != business.AccountStateDisabled {
		t.Fatalf("management-disabled account health = %q, want disabled", account.Health)
	}
}

func monitoringProjectionStore(t *testing.T) (*business.Store, *sql.DB) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "monitoring.db")
	store, err := business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Bootstrap(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetMode(context.Background(), "监控模式"); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", sqliteutil.DSN(path, ""))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`INSERT INTO accounts(id,name,schedulable,routing_state,metadata_json,updated_at)
		VALUES('41','monitoring-account',1,'healthy','{}','2026-09-14T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	return store, db
}
