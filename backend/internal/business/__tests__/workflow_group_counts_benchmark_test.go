package business_test

import (
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

func BenchmarkOnboardingProjectionGroupCounts(b *testing.B) {
	store, db := groupCountBenchmarkFixture(b, true)
	if _, err := db.Exec(`INSERT INTO upstreams(host,base_url,upstream_type,auth_status,updated_at)
		VALUES('onboarding.example','https://onboarding.example','sub2api','ready','before');
		INSERT INTO upstream_identities(upstream_id,created_at,updated_at) VALUES('benchmark-upstream','before','before');
		INSERT INTO upstream_identity_hosts(host,upstream_id,is_primary,updated_at)
		VALUES('onboarding.example','benchmark-upstream',1,'before')`); err != nil {
		b.Fatal(err)
	}
	projection := business.OnboardingProjection{
		OperationID: "onboarding-count-benchmark", AccountID: "2001", AccountName: "new-account",
		UpstreamHost: "onboarding.example", UpstreamType: "sub2api", UpstreamKeyID: "91", UpstreamKeyName: "key",
		UpstreamGroupID: "6", UpstreamGroupName: "upstream-group", LocalGroupID: "1", LocalGroupName: "group-1",
		Multiplier: "0.2", ReadbackConfirmed: true,
	}
	if err := store.CommitOnboardingProjection(b.Context(), projection); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		if err := store.CommitOnboardingProjection(b.Context(), projection); err != nil {
			b.Fatal(err)
		}
	}
}
