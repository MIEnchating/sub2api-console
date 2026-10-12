package onboarding_test

import (
	"context"
	"errors"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/onboarding"
	"testing"
)

type excludedSourceAccount struct{ *business.Store }

func (s excludedSourceAccount) AccountMutationProtection(context.Context, string) (business.AccountMutationProtection, error) {
	return business.AccountMutationProtection{UpstreamExcluded: true}, nil
}
func TestExcludedSourceAccountCannotBeReboundToManagedUpstream(t *testing.T) {
	service, _, keys, request := newService(t, "http://127.0.0.1:1", "http://127.0.0.1:1", func(s *business.Store) onboarding.Repository { return excludedSourceAccount{s} })
	request.AccountIDs = []string{"41"}
	_, err := service.Onboard(t.Context(), request)
	if !errors.Is(err, business.ErrUpstreamExcluded) {
		t.Fatalf("expected exclusion before external work: %v", err)
	}
	if keys.creates != 0 || keys.reveals != 0 {
		t.Fatal("excluded account reached keys")
	}
}
