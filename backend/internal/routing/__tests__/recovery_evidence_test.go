package routing_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/routing"
)

func TestFusedRecoveryRejectsSuccessesBeforeFuseAndAcceptsNewEvidence(t *testing.T) {
	for _, source := range []string{"traffic", "active-probe"} {
		t.Run(source, func(t *testing.T) {
			ctx := context.Background()
			store, db := healthEvidenceStore(t)
			now := time.Now().UTC()
			fusedSince := now.Add(-5 * time.Minute).Format(time.RFC3339Nano)
			payload, err := json.Marshal(map[string]any{"state_since": fusedSince, "fused_until": fusedSince})
			if err != nil {
				t.Fatal(err)
			}
			_, err = db.Exec(`UPDATE accounts SET schedulable=0,routing_state='fused' WHERE id='41';
				INSERT INTO routing_decisions(account_id,group_name,schedulable,routing_state,updated_at,payload_json)
				VALUES('41','codex',0,'fused',?,?)`, fusedSince, string(payload))
			if err != nil {
				t.Fatal(err)
			}
			persist := func(ages []time.Duration) {
				t.Helper()
				for _, age := range ages {
					observed := now.Add(-age).Format(time.RFC3339Nano)
					if source == "traffic" {
						_, err = store.PersistTrafficSamples(ctx, []business.TrafficSample{{AccountID: "41", GroupName: "codex", Result: "通过", EvidenceKey: observed, ObservedAt: observed, Payload: map[string]any{}}})
					} else {
						_, err = store.PersistProbeSamples(ctx, []business.ProbeSample{{AccountID: "41", GroupName: "codex", Result: "通过", ObservedAt: observed}})
					}
					if err != nil {
						t.Fatal(err)
					}
				}
			}
			persist([]time.Duration{6 * time.Minute, 7 * time.Minute})
			service := routing.NewService(store)
			result, err := service.Calculate(ctx, routing.Scope{}, true)
			if err != nil {
				t.Fatal(err)
			}
			decision := result.AccountDecisions["41"]
			if decision.RoutingState != "fused" || decision.Schedulable || decision.Recovery == nil || decision.Recovery.Ready {
				t.Fatalf("pre-fuse %s successes allowed recovery: %+v", source, decision)
			}
			persist([]time.Duration{time.Minute, 2 * time.Minute})
			result, err = service.Calculate(ctx, routing.Scope{}, true)
			if err != nil {
				t.Fatal(err)
			}
			if decision = result.AccountDecisions["41"]; decision.RoutingState != "healthy" || !decision.Schedulable {
				t.Fatalf("post-fuse %s successes did not permit recovery: %+v", source, decision)
			}
		})
	}
}
