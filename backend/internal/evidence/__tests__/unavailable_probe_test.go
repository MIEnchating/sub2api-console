package evidence_test

import (
	"context"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/evidence"
)

func TestAutomaticProbeReportsUnsupportedAccountWithoutHealthFailure(t *testing.T) {
	fixture := newProbeBatchFixture(t, []string{"1", "2"}, nil, "1")
	result, err := fixture.service.Collect(context.Background(), fixture.policy, nil, evidence.Options{ProbesAllowed: true, Now: fixture.now})
	if err != nil {
		t.Fatal(err)
	}
	if result.ProbesPersisted != 1 || len(result.SourceErrors) != 1 || !strings.Contains(result.SourceErrors[0], "账号 1") || !strings.Contains(result.SourceErrors[0], "不支持直连探活") {
		t.Fatalf("unsupported account disappeared from inspection details: %+v", result)
	}
	accountID := "1"
	samples, err := fixture.store.RoutingSamples(context.Background(), &accountID, nil, "active-probe", 10)
	if err != nil || len(samples) != 0 {
		t.Fatalf("unsupported account received false health evidence: %+v %v", samples, err)
	}
}
