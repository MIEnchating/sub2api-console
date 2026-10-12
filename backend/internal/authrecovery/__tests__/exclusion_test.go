package authrecovery_test

import (
	"github.com/MIEnchating/sub2api-console/backend/internal/authrecovery"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"path/filepath"
	"testing"
)

func TestExcludedHostDoesNotEnterBatchRecoveryOrReadCredentials(t *testing.T) {
	store, err := business.Open(filepath.Join(t.TempDir(), "business.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err = store.Bootstrap(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err = store.CreateUpstreamConfiguration(t.Context(), business.UpstreamConfigurationWrite{Host: "excluded.test", BaseURL: "https://excluded.test", UpstreamType: "sub2api", AuthMode: "sub2api_user_token", RechargeRate: "1"}); err != nil {
		t.Fatal(err)
	}
	preview, err := store.UpstreamExclusionPreview(t.Context(), "excluded.test")
	if err != nil {
		t.Fatal(err)
	}
	if err = store.SetUpstreamExcluded(t.Context(), preview.UpstreamID, true, "test"); err != nil {
		t.Fatal(err)
	}
	private, err := configstore.Open(filepath.Join(t.TempDir(), "private.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer private.Close()
	service := authrecovery.New(store, private, nil, nil, nil, nil)
	result, err := service.RecoverInvalid(t.Context(), []string{"excluded.test"}, "test")
	if err != nil || result.Hosts != 0 || result.Failed != 0 {
		t.Fatalf("excluded batch: %+v %v", result, err)
	}
}
