package business_test

import (
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"testing"
)

func TestNewAccountAllocationChoiceIsSavedWithStableAccountAndInheritedByLaterReads(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		t.Run(map[bool]string{true: "enable", false: "disable"}[enabled], func(t *testing.T) {
			store, _ := concurrencyStore(t)
			_, err := store.UpdatePolicy(t.Context(), map[string]any{"advanced_policy": map[string]any{"upstream_concurrency": map[string]any{"enabled": true, "account_mode": "all"}}}, "test")
			if err != nil {
				t.Fatal(err)
			}
			err = store.CommitOnboardingProjection(t.Context(), business.OnboardingProjection{
				OperationID: "new-account", AccountID: "147", AccountName: "new", UpstreamHost: "fixture.example", UpstreamType: "sub2api", UpstreamKeyID: "91", UpstreamKeyName: "key", UpstreamGroupID: "6", UpstreamGroupName: "pro", LocalGroupID: "3", LocalGroupName: "codex", Multiplier: "0.2", AllocationOverride: &enabled,
			})
			if err != nil {
				t.Fatal(err)
			}
			status, err := store.UpstreamAllocationSetting(t.Context(), "accounts", "147")
			if err != nil || status.Effective != enabled || status.Override == nil || *status.Override != enabled {
				t.Fatalf("saved account choice: %+v %v", status, err)
			}
		})
	}
}
