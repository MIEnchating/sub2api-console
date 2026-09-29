package onboarding_test

import (
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"strings"
	"testing"
)

func TestExistingManualAccountCannotSwitchGroupsThroughOnboarding(t *testing.T) {
	service, store, keys, request := newService(t, "http://127.0.0.1:1", "http://127.0.0.1:1")
	platform := "gemini"
	if err := store.CommitOnboardingProjection(t.Context(), business.OnboardingProjection{
		OperationID: "seed-manual", AccountID: "41", AccountName: "manual", Platform: platform,
		UpstreamHost: "upstream.test", UpstreamType: "sub2api", UpstreamKeyID: "91", UpstreamKeyName: "key",
		UpstreamGroupID: "6", UpstreamGroupName: "Gemini", LocalGroupID: "3", LocalGroupName: "gemini-平价",
		LocalGroups: []business.LocalOnboardingGroup{{ID: "3", Name: "gemini-平价", Platform: &platform}},
		Multiplier:  "0.2", Actor: "test", ReadbackConfirmed: true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AssignManualPriority(t.Context(), "41", 1, "1", 1, true, "test"); err != nil {
		t.Fatal(err)
	}
	request.AccountIDs = []string{"41"}
	if _, err := service.Onboard(t.Context(), request); err == nil || !strings.Contains(err.Error(), "手动控制") {
		t.Fatalf("manual group switch not rejected: %v", err)
	}
	if keys.creates != 0 || keys.reveals != 0 {
		t.Fatalf("protected operation reached key client: %+v", keys)
	}
}
