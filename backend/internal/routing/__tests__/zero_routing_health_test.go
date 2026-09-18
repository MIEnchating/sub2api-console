package routing_test

import (
	"strconv"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/routing"
)

func TestPendingFailureKeepsPreviouslyZeroRoutingHealthAtZeroWeight(t *testing.T) {
	store, db := healthEvidenceStore(t)
	_, err := db.Exec(`UPDATE accounts SET routing_state='survivor' WHERE id='41';
		INSERT INTO accounts(id,name,multiplier,schedulable,metadata_json,updated_at) VALUES('42','healthy','1',1,'{}','now');
		INSERT INTO account_groups(account_id,group_name) VALUES('42','codex');
		INSERT INTO routing_decisions(account_id,group_name,schedulable,routing_state,updated_at,payload_json)
		VALUES('41','codex',1,'survivor',?, '{"routing_health_score":0,"health_score":0}')`, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	samples := []business.TrafficSample{
		{AccountID: "41", GroupName: "codex", EvidenceKey: "new-failure", Result: "失败", ObservedAt: now.Add(-time.Second).Format(time.RFC3339Nano), Payload: map[string]any{"status_code": 502}},
		{AccountID: "41", GroupName: "codex", EvidenceKey: "old-credential", Result: "失败", ObservedAt: now.Add(-121 * time.Minute).Format(time.RFC3339Nano), Payload: map[string]any{"status_code": 401}},
		{AccountID: "42", GroupName: "codex", EvidenceKey: "healthy", Result: "通过", ObservedAt: now.Add(-time.Second).Format(time.RFC3339Nano), Payload: map[string]any{"status_code": 200}},
	}
	for index := range 58 {
		samples = append(samples, business.TrafficSample{AccountID: "41", GroupName: "codex", EvidenceKey: "history-" + strconv.Itoa(index), Result: "通过", ObservedAt: now.Add(-time.Duration(122+index) * time.Minute).Format(time.RFC3339Nano), Payload: map[string]any{"status_code": 200}})
	}
	if _, err := store.PersistTrafficSamples(t.Context(), samples); err != nil {
		t.Fatal(err)
	}
	result, err := routing.NewService(store).Calculate(t.Context(), routing.Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	decision := result.AccountDecisions["41"]
	if !decision.EvidencePending || decision.RoutingHealthScore != 0 || decision.HealthScore <= 40 || decision.Weight != 0 {
		t.Fatalf("zero routing health bypassed gate: %+v", decision)
	}
}
