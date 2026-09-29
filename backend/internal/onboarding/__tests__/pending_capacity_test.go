package onboarding_test

import (
	"context"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/onboarding"
)

func TestRetryPreviewCountsAlreadySyncedPendingAccountOnlyOnce(t *testing.T) {
	for _, scope := range []string{"upstream", "global"} {
		t.Run(scope, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			service, repo, _, request := newService(t, "http://127.0.0.1:1", "http://127.0.0.1:1", func(store *business.Store) onboarding.Repository {
				return &cancelAfterKeyRepository{Store: store, cancel: cancel}
			})
			limit := int64(7)
			upstreamLimit := limit
			if scope == "global" {
				upstreamLimit = 100
			}
			if _, err := repo.ApplyUpstreamSync(t.Context(), business.UpstreamSyncWrite{Host: "upstream.test", Balance: &business.UpstreamBalanceObservation{ConcurrencyLimit: &upstreamLimit, ProfileUserID: "17"}}); err != nil {
				t.Fatal(err)
			}
			if scope == "global" {
				if _, err := repo.UpdatePolicy(t.Context(), map[string]any{"advanced_policy": map[string]any{"scaling": map[string]any{"enabled": true, "global_max_concurrency": 7}}}, "test"); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := service.Onboard(ctx, request); err == nil || ctx.Err() == nil {
				t.Fatalf("expected cancellation: %v", err)
			}
			pending, err := repo.PendingOnboarding(t.Context(), "upstream.test", "6", []string{"3"})
			if err != nil || pending == nil {
				t.Fatalf("missing pending operation: %v", err)
			}
			pending.UpstreamAccountID = "77"
			if err := repo.SavePendingOnboarding(t.Context(), *pending); err != nil {
				t.Fatal(err)
			}
			// A later catalog sync may already include the confirmed remote account
			// while the original onboarding operation still awaits completion.
			if err := repo.CommitOnboardingProjection(t.Context(), business.OnboardingProjection{
				OperationID: "catalog-observation", AccountID: "77", AccountName: "already-created", Platform: "gemini",
				UpstreamHost: "upstream.test", UpstreamType: "sub2api", UpstreamKeyID: "91", UpstreamGroupID: "6", UpstreamGroupName: "Gemini",
				LocalGroupID: "3", LocalGroupName: "gemini-平价", Multiplier: "0.2", Concurrency: &limit, Schedulable: false, ReadbackConfirmed: true,
			}); err != nil {
				t.Fatal(err)
			}
			preview, err := service.PreviewConcurrency(t.Context(), []onboarding.Request{request})
			if err != nil || len(preview) != 1 || preview[0].Concurrency == nil || *preview[0].Concurrency != 7 {
				t.Fatalf("retry double-counted its existing account: %+v %v", preview, err)
			}
			if _, err := service.PreviewConcurrency(t.Context(), []onboarding.Request{request, request}); err == nil {
				t.Fatal("batch released the same account reservation twice")
			}
		})
	}
}
