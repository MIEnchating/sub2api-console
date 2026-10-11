package business_test

import (
	"strings"
	"testing"
)

func TestNotificationQueueDeliveryQueryUsesRecentPartialIndex(t *testing.T) {
	_, db := openAlertRuleStore(t)
	rows, err := db.Query(`EXPLAIN QUERY PLAN
		SELECT incident_key FROM alert_incidents INDEXED BY ix_alert_incidents_delivery_recent
		WHERE status IN ('firing','recovered')
		ORDER BY last_seen_at,incident_key`)
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
		plan.WriteByte('\n')
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	output := plan.String()
	if !strings.Contains(output, "ix_alert_incidents_delivery_recent") {
		t.Fatalf("notification queue query did not use recent partial index: %s", output)
	}
	if strings.Contains(output, "USE TEMP B-TREE FOR ORDER BY") {
		t.Fatalf("notification queue query still sorts outside the index: %s", output)
	}
}
