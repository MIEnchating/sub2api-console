package routing_test

import (
	"strconv"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/routing"
)

func TestEachStrategyReducesRepeatedCapacityFailuresWithoutCredentialCleanup(t *testing.T) {
	for _, strategy := range []string{"price_first", "speed_first", "balanced", "reliability"} {
		t.Run(strategy, func(t *testing.T) {
			store, db := healthEvidenceStore(t)
			if _, err := store.UpdatePolicy(t.Context(), map[string]any{"global_strategy": strategy}, "test"); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(`INSERT INTO accounts(id,name,multiplier,schedulable,metadata_json,updated_at)
				VALUES('42','healthy','1',1,'{}','now'); INSERT INTO account_groups(account_id,group_name) VALUES('42','codex')`); err != nil {
				t.Fatal(err)
			}
			now := time.Now().UTC()
			samples := []business.TrafficSample{{
				AccountID: "42", GroupName: "codex", EvidenceKey: "healthy", Result: "通过",
				ObservedAt: now.Format(time.RFC3339Nano), Payload: map[string]any{"status_code": 200},
			}}
			for index, message := range []string{
				"The service is busy. Please retry later.",
				"Our servers are currently overloaded. Please try again later.",
				"The service is busy. Please retry later.",
			} {
				samples = append(samples, business.TrafficSample{
					AccountID: "41", GroupName: "codex", EvidenceKey: "capacity-" + strconv.Itoa(index), Result: "失败",
					FailureReason: &message, ObservedAt: now.Add(-time.Duration(index) * time.Second).Format(time.RFC3339Nano),
					Payload: map[string]any{"status_code": 200},
				})
			}
			if _, err := store.PersistTrafficSamples(t.Context(), samples); err != nil {
				t.Fatal(err)
			}
			result, err := routing.NewService(store).Calculate(t.Context(), routing.Scope{}, true)
			if err != nil {
				t.Fatal(err)
			}
			busy, healthy := result.AccountDecisions["41"], result.AccountDecisions["42"]
			if busy.LatestEvent != routing.EventGateway || busy.EvidencePending || busy.RoutingState != "degraded" || busy.Weight >= healthy.Weight || !busy.Schedulable {
				t.Fatalf("strategy did not reduce confirmed capacity failures: busy=%+v healthy=%+v", busy, healthy)
			}
			if target := result.AccountTargets["41"]; target.CleanupAction != nil {
				t.Fatalf("temporary capacity failure scheduled credential cleanup: %+v", target)
			}
		})
	}
}
