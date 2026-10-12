package onboarding_test

import (
	"context"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/onboarding"
	"github.com/google/uuid"
)

func TestNewKeyUsesBoundUpstreamGroupNameAndUUID(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	service, store, _, request := newService(t, "http://127.0.0.1:1", "http://127.0.0.1:1", func(store *business.Store) onboarding.Repository {
		return &cancelAfterKeyRepository{Store: store, cancel: cancel}
	})
	platform, status, rate := "gemini", "active", "0.2"
	groupName := "满血-Gemini 高级组"
	if _, err := store.ApplyUpstreamSync(t.Context(), business.UpstreamSyncWrite{Host: request.Host, Catalog: &business.UpstreamCatalogSnapshot{Groups: []business.UpstreamCatalogGroup{{GroupID: "6", Name: groupName, Platform: &platform, Status: &status, RawRate: &rate}}}}); err != nil {
		t.Fatal(err)
	}
	_, _ = service.Onboard(ctx, request)
	pending, err := store.PendingOnboarding(t.Context(), request.Host, request.UpstreamGroupID, []string{request.LocalGroupID})
	if err != nil || pending == nil {
		t.Fatalf("pending=%+v err=%v", pending, err)
	}
	if pending.UpstreamKeyName == nil || !strings.HasPrefix(*pending.UpstreamKeyName, groupName+"-") {
		t.Fatalf("expected readable group name, got %+v", pending.UpstreamKeyName)
	}
	if pending.UpstreamKeyID != "91" || pending.OperationID == "" || pending.OperationID == *pending.UpstreamKeyName {
		t.Fatalf("local identity mapping lost: %+v", pending)
	}
	suffix := strings.TrimPrefix(*pending.UpstreamKeyName, groupName+"-")
	if _, err := uuid.Parse(suffix); err != nil {
		t.Fatalf("name does not end in UUID: %s", *pending.UpstreamKeyName)
	}
	if pending.OperationID != "account-onboarding-"+suffix {
		t.Fatalf("operation mapping does not match UUID: %s", pending.OperationID)
	}
}
