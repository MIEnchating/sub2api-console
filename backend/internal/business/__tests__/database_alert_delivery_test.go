package business_test

import (
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/dbconn"
	"github.com/MIEnchating/sub2api-console/backend/internal/dbtest"
)

func TestDatabaseContractAlertFinalizationCountsEachAttemptOnce(t *testing.T) {
	for _, config := range dbtest.Configs(t) {
		t.Run(string(config.Driver), func(t *testing.T) {
			store, err := business.OpenDatabase(t.Context(), config)
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			db, err := dbconn.Open(t.Context(), config)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if _, err := db.ExecContext(t.Context(), `INSERT INTO alert_incidents(incident_key,event_type,object_kind,object_id,cause_code,status,first_seen_at,last_seen_at) VALUES('contract-delivery','account.cost_traffic','account','41','fixture','firing','2026-10-12T00:00:00Z','2026-10-12T00:00:00Z')`); err != nil {
				t.Fatal(err)
			}
			defer func() {
				if _, err := db.ExecContext(t.Context(), `DELETE FROM alert_incidents WHERE incident_key='contract-delivery'`); err != nil {
					t.Error(err)
				}
			}()
			for attempt := int64(1); attempt <= 2; attempt++ {
				if err := store.BeginAlertDelivery(t.Context(), "channel", []business.AlertIncident{{IncidentKey: "contract-delivery"}}); err != nil {
					t.Fatal(err)
				}
				if err := store.FinalizeAlertDelivery(t.Context(), "channel", []business.AlertDeliveryOutcome{{IncidentKey: "contract-delivery", Success: attempt == 2, Detail: "fixture"}}); err != nil {
					t.Fatal(err)
				}
				var count int64
				if err := db.QueryRowContext(t.Context(), `SELECT attempts FROM alert_deliveries WHERE incident_key='contract-delivery' AND channel_key='channel'`).Scan(&count); err != nil || count != attempt {
					t.Fatalf("attempt counted twice: %d want %d, %v", count, attempt, err)
				}
			}
		})
	}
}
