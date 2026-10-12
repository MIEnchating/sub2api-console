package routing_test

import (
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/routing"
	"testing"
)

func TestMissingBindingCleanupOnlyQueuesConfirmedUnprotectedAccounts(t *testing.T) {
	for _, scenario := range []string{"confirmed", "disabled", "suspected", "ambiguous", "paused", "manual-priority", "manual-fused", "excluded", "upstream-excluded", "calculation-only"} {
		t.Run(scenario, func(t *testing.T) {
			a := placementAccount("41", "1", 20)
			a.CatalogBindingState = "group_missing"
			a.MissingBindingFingerprint = "confirmed-binding"
			repository := placementFixture(a)
			repository.policy["binding_cleanup"] = map[string]any{"enabled": scenario != "disabled"}
			switch scenario {
			case "suspected":
				repository.accounts[0].CatalogBindingState = "suspected"
			case "ambiguous":
				repository.accounts[0].MissingBindingFingerprint = ""
			case "paused":
				repository.accounts[0].Paused = true
			case "manual-priority":
				p := int64(1)
				repository.accounts[0].ManualPriority = &p
			case "manual-fused":
				repository.policy["scope"] = map[string]any{"manual_fused_account_ids": []any{"41"}}
			case "excluded":
				repository.policy["scope"] = map[string]any{"excluded_account_ids": []any{"41"}}
			case "upstream-excluded":
				repository.accounts[0].UpstreamExcluded = true
			}
			result, err := routing.NewService(repository).Calculate(t.Context(), routing.Scope{}, scenario != "calculation-only")
			if err != nil {
				t.Fatal(err)
			}
			target := result.AccountTargets["41"]
			if scenario == "confirmed" {
				if target.CleanupAction == nil || *target.CleanupAction != "delete" || target.CleanupReason != business.MissingBindingCleanupReason || target.MissingBindingFingerprint != "confirmed-binding" {
					t.Fatalf("confirmed final group member not queued: %+v", target)
				}
			} else if target.CleanupAction != nil {
				t.Fatalf("protected account queued: %+v", target)
			}
		})
	}
}

func TestMissingBindingCleanupLimitsUniqueAccountsPerRound(t *testing.T) {
	a, b := placementAccount("41", "1", 20), placementAccount("42", "1", 21)
	for _, account := range []*business.RoutingAccount{&a, &b} {
		account.CatalogBindingState = "key_missing"
		account.MissingBindingFingerprint = account.ID
	}
	duplicate := a
	duplicate.GroupName = "second-group"
	repository := placementFixture(a, duplicate, b)
	repository.policy["binding_cleanup"] = map[string]any{"enabled": true, "max_per_round": 1}
	result, err := routing.NewService(repository).Calculate(t.Context(), routing.Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if result.AccountTargets["41"].CleanupAction == nil || result.AccountTargets["42"].CleanupAction != nil {
		t.Fatalf("batch limit lost: %+v", result.AccountTargets)
	}
}
