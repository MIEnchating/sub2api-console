package business_test

import (
	"context"
	"errors"
	"testing"
)

func TestOnboardingCandidatesKeepNameWithoutTrafficStatistics(t *testing.T) {
	store, db := concurrencyStore(t)
	if _, err := db.Exec(`UPDATE upstreams SET metadata_json=json_set(metadata_json,'$.site_name','候选上游') WHERE host='fixture.example';
 INSERT INTO upstream_groups(host,group_id,name,platform,status,raw_rate,effective_rate,updated_at) VALUES('fixture.example','7','测试分组','openai','active','0.2','0.2','now');
 DROP TABLE usage_records;`); err != nil {
		t.Fatal(err)
	}
	candidates, err := store.OnboardingCandidates(t.Context(), "fixture.example")
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 || candidates[0].UpstreamName != "候选上游" || !candidates[0].CanCreateKey {
		t.Fatalf("unrelated traffic storage affected onboarding: %+v", candidates)
	}
}

func TestOnboardingCandidatesReadCurrentNameAfterConfigurationChanges(t *testing.T) {
	store, db := concurrencyStore(t)
	if _, err := db.Exec(`INSERT INTO upstream_groups(host,group_id,name,status,raw_rate,effective_rate,updated_at) VALUES('fixture.example','7','测试分组','active','0.2','0.2','now')`); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"原上游名称", "新上游名称"} {
		if _, err := db.Exec(`UPDATE upstreams SET metadata_json=json_set(metadata_json,'$.site_name',?) WHERE host='fixture.example'`, name); err != nil {
			t.Fatal(err)
		}
		candidates, err := store.OnboardingCandidates(t.Context(), " HTTPS://FIXTURE.EXAMPLE/ ")
		if err != nil || len(candidates) != 1 || candidates[0].UpstreamName != name {
			t.Fatalf("stale candidate name: %+v %v", candidates, err)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := store.OnboardingCandidates(ctx, "fixture.example"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation lost: %v", err)
	}
}
