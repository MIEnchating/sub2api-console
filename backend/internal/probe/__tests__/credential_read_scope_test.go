package probe_test

import (
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/probe"
)

func TestCachedCredentialProbeDoesNotRequireDisplayHealthEvaluations(t *testing.T) {
	fixture := newCredentialFixture(t)
	fixture.cacheKey(t)
	// Credential checks must not load the dashboard's historical score projection.
	if _, err := fixture.database.Exec(`DROP TABLE account_health_evaluations`); err != nil {
		t.Fatal(err)
	}
	summary, err := probe.New(fixture.repository, fixture.settings, nil).RunNow(t.Context(), probe.Request{AccountID: pointer("41")})
	if err != nil {
		t.Fatal(err)
	}
	if summary.Passed != 1 || fixture.generated.Load() != 1 {
		t.Fatalf("bound credential resolution depended on display history: %+v", summary)
	}
}
