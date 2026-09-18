package routing_test

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/routing"
)

func TestHealthProjectionDoesNotReadRemoteWriteAuditHistory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "health-query.db")
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
	if _, err := db.Exec(`INSERT INTO accounts(id,name,schedulable,multiplier,updated_at) VALUES('41','fixture',1,'1','now');
		INSERT INTO account_groups(account_id,group_name) VALUES('41','fixture');
		INSERT INTO routing_decisions(account_id,group_name,routing_state,updated_at,payload_json)
		VALUES('41','fixture','healthy','2026-09-18T00:00:00Z','{}');
		DROP TABLE operation_audit`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO health_samples(account_id,group_name,result,observed_at,source,evidence_key)
		VALUES('41','fixture','passed',?,'active_probe','probe')`, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	projections, err := routing.NewService(store).ProjectHealth(t.Context(), routing.Scope{})
	if err != nil {
		t.Fatalf("health display unnecessarily depends on remote-write audit: %v", err)
	}
	if row, found := projections["41"]; !found || row.HealthScore == nil || row.SampleCount != 1 {
		t.Fatalf("health evidence was not projected: %+v", projections)
	}
}
