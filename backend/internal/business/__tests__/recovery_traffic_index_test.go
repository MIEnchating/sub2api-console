package business_test

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

func TestRecoveryTrafficTimeKeyRetainsItsPersistedBinaryFormat(t *testing.T) {
	_, db := concurrencyStore(t)
	for _, scenario := range []struct{ stamp, key string }{
		{"1969-12-31T23:59:59.999999999Z", "7FFFFFFFFFFFFFFF3B9AC9FF"},
		{"1970-01-01T00:00:00Z", "800000000000000000000000"},
		{"1970-01-01T01:00:00+01:00", "800000000000000000000000"},
		{"1970-01-01T00:00:00.000000001Z", "800000000000000000000001"},
	} {
		var key string
		if err := db.QueryRow(`SELECT hex(console_recovery_time_v1(?))`, scenario.stamp).Scan(&key); err != nil {
			t.Fatal(err)
		}
		if key != scenario.key {
			t.Fatalf("persisted key for %s=%s, want %s", scenario.stamp, key, scenario.key)
		}
	}
}

func TestRecoveryTrafficIntervalAndBoundaryUseIndexedActualTimeRanges(t *testing.T) {
	_, db := concurrencyStore(t)
	for _, predicate := range []string{
		`console_recovery_time_v1(observed_at)>=console_recovery_time_v1(?)`,
		`console_recovery_time_v1(observed_at)<console_recovery_time_v1(?)`,
	} {
		rows, err := db.Query(`EXPLAIN QUERY PLAN SELECT id FROM usage_records
			WHERE LOWER(REPLACE(source,'_','-'))='traffic' AND account_id='41' AND `+predicate+`
			ORDER BY console_recovery_time_v1(observed_at) DESC,id DESC LIMIT 100`, "2026-10-01T12:00:00Z")
		if err != nil {
			t.Fatal(err)
		}
		var plan strings.Builder
		for rows.Next() {
			var id, parent, unused int
			var detail string
			if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
				t.Fatal(err)
			}
			plan.WriteString(detail)
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		if err := rows.Close(); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(plan.String(), "ix_usage_records_recovery_time") || !strings.Contains(plan.String(), "<expr>") || strings.Contains(plan.String(), "TEMP B-TREE") {
			t.Fatalf("recovery time selection must use an indexed range without sorting: %s", plan.String())
		}
	}
}

func TestRecoveryTrafficReopenBuildsTimeIndexAndPreservesLegacyEvidence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "recovery-upgrade.db")
	store, err := business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`DROP INDEX ix_usage_records_recovery_time;
		PRAGMA user_version=11;
		INSERT INTO usage_records(account_id,request_id,group_name,source,is_error,observed_at,payload_json)
		VALUES('41','legacy','codex','traffic',1,'2026-10-01T20:00:00+08:00','{}')`); err != nil {
		t.Fatal(err)
	}
	store, err = business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	var version, indexes int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil || version != 13 {
		t.Fatalf("recovery index requires schema 12: version=%d error=%v", version, err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_schema WHERE name='ix_usage_records_recovery_time'`).Scan(&indexes); err != nil || indexes != 1 {
		t.Fatalf("missing recovery index: count=%d error=%v", indexes, err)
	}
	since := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	rows, err := store.RoutingRecoveryTraffic(t.Context(), "41", since, since.Add(time.Minute))
	if err != nil || len(rows) != 1 || rows[0].ObservedAt != "2026-10-01T20:00:00+08:00" || rows[0].Result != "失败" {
		t.Fatalf("upgrade modified legacy evidence: rows=%+v error=%v", rows, err)
	}
}

func TestRecoveryTrafficIndexCreationFailureRollsBackOtherSchemaChangesAndVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "recovery-conflict.db")
	store, err := business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`DROP INDEX ix_usage_records_account_recent;
		DROP INDEX ix_usage_records_recovery_time;
		CREATE TABLE ix_usage_records_recovery_time(id INTEGER);
		PRAGMA user_version=11;
		INSERT INTO usage_records(account_id,request_id,group_name,source,is_error,observed_at,payload_json)
		VALUES('41','preserved','codex','traffic',1,'2026-10-01T12:00:00Z','{}')`); err != nil {
		t.Fatal(err)
	}
	if store, err := business.Open(path); err == nil {
		store.Close()
		t.Fatal("conflicting recovery index name accepted")
	}
	var indexes, version, records int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_schema WHERE name='ix_usage_records_account_recent'`).Scan(&indexes); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM usage_records WHERE request_id='preserved'`).Scan(&records); err != nil {
		t.Fatal(err)
	}
	if indexes != 0 || version != 11 || records != 1 {
		t.Fatalf("failed upgrade escaped rollback: indexes=%d version=%d records=%d", indexes, version, records)
	}
}
