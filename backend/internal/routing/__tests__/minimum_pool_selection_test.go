package routing_test

import (
	"strconv"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/routing"
)

func TestSimultaneousFuseKeepsSuccessfulSlowAccountInsteadOfInvalidCredentials(t *testing.T) {
	for _, score := range []int{65, 2} {
		t.Run(strconv.Itoa(score), func(t *testing.T) {
			testSimultaneousFuseSelection(t, score)
		})
	}
}

func testSimultaneousFuseSelection(t *testing.T, slowScore int) {
	t.Helper()
	store, db := healthEvidenceStore(t)
	_, err := db.Exec(`INSERT INTO accounts(id,name,multiplier,schedulable,metadata_json,updated_at)
		VALUES('42','slow-success','1',1,'{}','now'); INSERT INTO account_groups(account_id,group_name) VALUES('42','codex')`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.UpdatePolicy(t.Context(), map[string]any{"advanced_policy": map[string]any{
		"breaker": map[string]any{"min_pool_size": 1, "latency_occurrences": 3, "latency_window": 3, "latency_degrade_only": false},
		"scoring": map[string]any{"event_scores": map[string]any{"slow_ttfb": slowScore}},
	}}, "test")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	samples := []business.TrafficSample{{AccountID: "41", GroupName: "codex", EvidenceKey: "credential-failure", Result: "失败", ObservedAt: now.Add(-time.Minute).Format(time.RFC3339Nano), Payload: map[string]any{"status_code": 401}}}
	for _, age := range []time.Duration{time.Minute, 2 * time.Minute, 3 * time.Minute} {
		observed := now.Add(-age).Format(time.RFC3339Nano)
		samples = append(samples, business.TrafficSample{AccountID: "42", GroupName: "codex", EvidenceKey: observed, Result: "通过", ObservedAt: observed, Payload: map[string]any{"status_code": 200, "first_token_ms": 20000}})
	}
	if _, err := store.PersistTrafficSamples(t.Context(), samples); err != nil {
		t.Fatal(err)
	}
	result, err := routing.NewService(store).Calculate(t.Context(), routing.Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	invalid, slow := result.AccountDecisions["41"], result.AccountDecisions["42"]
	if invalid.Schedulable || invalid.RoutingState != "fused" || !slow.Schedulable || slow.RoutingState != "survivor" {
		t.Fatalf("wrong surviving account: invalid=%+v slow=%+v", invalid, slow)
	}
}
