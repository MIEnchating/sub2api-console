package business_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

func TestTrafficPruningPreservesUnknownDatesBoundaryAndOtherAccounts(t *testing.T) {
	store, db := auditRetentionStore(t)
	latest := time.Date(2026, 9, 18, 0, 0, 0, 0, time.UTC)
	cutoff := latest.Add(-30 * 24 * time.Hour).Format("2006-01-02T15:04:05.000000000Z")
	if _, err := db.Exec(`INSERT INTO usage_records(request_id,account_id,source,observed_at) VALUES
		('unknown','41','traffic',NULL),('boundary','41','traffic',?),
		('expired','41','traffic','2026-01-01'),('other-account','42','traffic','2026-01-01'),
		('other-source','41','usage-log-state','2026-01-01')`, cutoff); err != nil {
		t.Fatal(err)
	}
	if _, err := store.PersistTrafficSamples(t.Context(), []business.TrafficSample{{AccountID: "41", GroupName: "fixture", Result: "通过", ObservedAt: latest.Format(time.RFC3339Nano), EvidenceKey: "current", Payload: map[string]any{}}}); err != nil {
		t.Fatal(err)
	}
	rows, err := db.Query(`SELECT request_id FROM usage_records ORDER BY request_id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var actual []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		actual = append(actual, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if want := []string{"boundary", "current", "other-account", "other-source", "unknown"}; !reflect.DeepEqual(actual, want) {
		t.Fatalf("retained request IDs = %v, want %v", actual, want)
	}
}
