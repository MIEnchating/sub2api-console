package business_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/dbconn"
	"github.com/MIEnchating/sub2api-console/backend/internal/dbtest"
)

func TestDatabaseContractRecoveryRetainsIntervalBeyondHealthCache(t *testing.T) {
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
			defer db.ExecContext(t.Context(), `DELETE FROM health_samples WHERE account_id='902'`)
			defer db.ExecContext(t.Context(), `DELETE FROM usage_records WHERE account_id='902'`)
			defer db.ExecContext(t.Context(), `DELETE FROM account_stability_samples WHERE account_id='902'`)
			base := time.Now().UTC().Truncate(time.Second).Add(-time.Hour)
			samples := make([]business.TrafficSample, 205)
			for i := range samples {
				samples[i] = business.TrafficSample{AccountID: "902", GroupName: "recovery", EvidenceKey: fmt.Sprintf("recovery-%d", i), Result: "通过", ObservedAt: base.Add(time.Duration(i) * time.Second).In(time.FixedZone("offset", -5*3600)).Format(time.RFC3339Nano), Payload: map[string]any{}}
			}
			if count, err := store.PersistTrafficSamples(t.Context(), samples); err != nil || count != 205 {
				t.Fatalf("observations: %d %v", count, err)
			}
			got, err := store.RoutingRecoveryTraffic(t.Context(), "902", base.Add(2*time.Second+time.Nanosecond), base.Add(204*time.Second))
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != 203 || got[0].Payload["request_id"] != "recovery-204" || got[len(got)-1].Payload["request_id"] != "recovery-2" {
				t.Fatalf("recovery interval lost boundary: count=%d", len(got))
			}
			var cached int
			if err := db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM health_samples WHERE account_id='902'`).Scan(&cached); err != nil || cached != 200 {
				t.Fatalf("health retention: %d %v", cached, err)
			}
		})
	}
}
