package business

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestValidateNewAPIQuotaUnitRequiresHistoricalBaselineButPersistsObservation(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "quota-unit.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	start := time.Now().UTC().Add(-48 * time.Hour)
	end := start.Add(24 * time.Hour)
	err = store.ValidateNewAPIQuotaUnit(context.Background(), "api.example", 500000, start, end)
	if err == nil || !strings.Contains(err.Error(), "历史基线") {
		t.Fatalf("err=%v", err)
	}
	var count int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM billing_quota_unit_observations WHERE host='api.example'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("count=%d err=%v", count, err)
	}
}

func TestValidateNewAPIQuotaUnitAcceptsStableBaselineAndRejectsWindowChange(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "quota-unit-stable.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	start := time.Now().UTC().Add(-48 * time.Hour).Truncate(time.Second)
	end := start.Add(24 * time.Hour)
	if _, err := store.db.Exec(`INSERT INTO billing_quota_unit_observations(host,observed_at,quota_per_unit) VALUES(?,?,?)`,
		"api.example", start.Add(-time.Hour).Format(time.RFC3339Nano), "500000"); err != nil {
		t.Fatal(err)
	}
	if err := store.ValidateNewAPIQuotaUnit(context.Background(), "api.example", 500000, start, end); err != nil {
		t.Fatalf("stable baseline: %v", err)
	}
	if _, err := store.db.Exec(`INSERT INTO billing_quota_unit_observations(host,observed_at,quota_per_unit) VALUES(?,?,?)`,
		"api.example", start.Add(time.Hour).Format(time.RFC3339Nano), "1000000"); err != nil {
		t.Fatal(err)
	}
	err = store.ValidateNewAPIQuotaUnit(context.Background(), "api.example", 500000, start, end)
	if err == nil || !strings.Contains(err.Error(), "报告窗口内") {
		t.Fatalf("err=%v", err)
	}
}
