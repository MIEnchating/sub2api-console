package business_test

import (
	"testing"
)

func TestRoutingSampleWindowCountsUniqueNormalizedEvidence(t *testing.T) {
	store, db := healthDecisionStore(t)
	if _, err := db.Exec(`INSERT INTO health_samples(account_id,group_name,source,evidence_key,observed_at,payload_json) VALUES
		('41','primary','active_probe','old','2026-09-18T00:00:00Z','{}'),
		('41','primary','active_probe','distinct','2026-09-18T01:00:00Z','{}'),
		('41','secondary','ACTIVE-PROBE','duplicate','2026-09-18T02:00:00Z','{}'),
		('41','primary','active_probe','duplicate','2026-09-18T03:00:00Z','{"latest":true}'),
		('41','primary','traffic','traffic','2026-09-18T04:00:00Z','{}'),
		('42','primary','active_probe','duplicate','2026-09-18T01:00:00Z','{}')`); err != nil {
		t.Fatal(err)
	}
	rows, err := store.RoutingSamples(t.Context(), nil, nil, "active_probe", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 || rows[0].AccountID != "41" || rows[0].EvidenceKey != "duplicate" || rows[0].Payload["latest"] != true || rows[1].EvidenceKey != "distinct" || rows[2].AccountID != "42" {
		t.Fatalf("per-account windows must retain newest unique probe evidence, including accounts without current metadata: %+v", rows)
	}
}
