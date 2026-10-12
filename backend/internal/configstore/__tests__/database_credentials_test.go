package configstore_test

import (
	"errors"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"strings"
	"testing"
)

func TestDatabaseContractCredentialSaveRejectsChangedAuthorization(t *testing.T) {
	for _, config := range databaseConfigs(t) {
		t.Run(string(config.Driver), func(t *testing.T) {
			store := openContractStore(t, config)
			auth := configstore.AuthRecord{Host: "contract.example.test", BaseURL: "https://contract.example.test", UpstreamType: "sub2api", AuthMode: "bearer", Headers: map[string]string{}, Cookies: map[string]string{}}
			if err := store.SaveAuthRecord(t.Context(), auth, nil); err != nil {
				t.Fatal(err)
			}
			key := configstore.UpstreamKeySecret{Host: auth.Host, KeyID: strings.Repeat("文", 255), GroupID: strings.Repeat("组", 255), Secret: "fixture-secret"}
			if err := store.SaveUpstreamKeySecretForAuth(t.Context(), key, auth); err != nil {
				t.Fatal(err)
			}
			got, err := store.UpstreamKeySecret(t.Context(), key.Host, key.KeyID, key.GroupID)
			if err != nil || got == nil || got.Secret != key.Secret {
				t.Fatalf("bound credential lookup failed: %v", err)
			}
			next := auth
			next.BaseURL += "/new"
			if err := store.SaveAuthRecord(t.Context(), next, nil); err != nil {
				t.Fatal(err)
			}
			if err := store.SaveUpstreamKeySecretForAuth(t.Context(), key, auth); !errors.Is(err, configstore.ErrUpstreamKeyAuthChanged) {
				t.Fatalf("stale auth: %v", err)
			}
		})
	}
}
