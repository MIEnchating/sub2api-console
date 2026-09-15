package routing_test

import (
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/routing"
)

func TestScopedIndependentReductionReservesOutsideAccountsWithoutDedicatedCapacityReader(t *testing.T) {
	repository := reductionFixture(t, upstreamCapacityAccount("41", 4, 10), upstreamCapacityAccount("42", 9, 10))
	// The base repository contract remains valid without the optional inventory reader.
	baseOnly := struct{ routing.Repository }{Repository: repository}
	id := "41"
	result, err := routing.NewService(baseOnly).Calculate(t.Context(), routing.Scope{AccountID: &id}, true)
	if err != nil {
		t.Fatal(err)
	}
	if target := result.AccountTargets[id]; target.Concurrency == nil || *target.Concurrency != 1 {
		t.Fatalf("the out-of-scope account must reserve nine slots when only independent reduction is enabled: %+v", target)
	}
	if len(result.AccountTargets) != 1 {
		t.Fatalf("reading shared inventory must not expand the requested write scope: %+v", result.AccountTargets)
	}
}
