package business_test

import (
	"strings"
	"testing"
)

func TestPricingHistoryUsesSelectiveIndexWithoutSortingOtherEvents(t *testing.T) {
	store, db := auditRetentionStore(t)
	if _, err := db.Exec(`INSERT INTO runtime_events VALUES
		(-1,'pricing.groups.synced','2026-09-18T00:00:00Z','succeeded','first','{"actor":"first","accounts":1,"group_links":1}'),
		(-2,'pricing.groups.synced','2026-09-18T00:00:00Z','succeeded','second','{"actor":"second","accounts":1,"group_links":1}'),
		(-3,'routing.applied','2026-09-18T01:00:00Z','succeeded','unrelated','{}')`); err != nil {
		t.Fatal(err)
	}
	rows, err := db.Query(`EXPLAIN QUERY PLAN SELECT source_id,created_at,payload_json FROM runtime_events
		WHERE event_type='pricing.groups.synced' AND status='succeeded'
		ORDER BY created_at DESC,CASE WHEN source_id < 0 THEN source_id END ASC,
		CASE WHEN source_id >= 0 THEN source_id END DESC LIMIT 100`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
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
	if !strings.Contains(plan.String(), "ix_runtime_events_pricing_recent") || strings.Contains(plan.String(), "TEMP B-TREE") {
		t.Fatalf("pricing history scans or sorts unrelated events: %s", plan.String())
	}
	records, err := store.PricingChangeRecords(t.Context(), 100)
	if err != nil || len(records) != 2 || records[0].Actor != "second" || records[1].Actor != "first" {
		t.Fatalf("pricing history ordering changed: %+v %v", records, err)
	}
}
