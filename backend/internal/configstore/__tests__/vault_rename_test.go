package configstore_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func TestVaultRenameFailureLeavesOriginalAndSelectionIntact(t *testing.T) {
	for _, config := range databaseConfigs(t) {
		t.Run(string(config.Driver), func(t *testing.T) {
			store := openContractStore(t, config)
			name, secret := "original", "private-value"
			original := configstore.VaultEntry{ID: name, Entry: name, Username: &secret, Password: &secret, Hosts: []string{"api.example.test"}, Headers: map[string]string{"X-Private": secret}}
			if err := store.SaveVaultEntry(t.Context(), original, nil); err != nil {
				t.Fatal(err)
			}
			if err := store.SaveVaultEntry(t.Context(), configstore.VaultEntry{ID: "existing", Entry: "existing"}, nil); err != nil {
				t.Fatal(err)
			}
			if err := store.SaveAuthRecoveryPreference(t.Context(), configstore.AuthRecoveryPreference{Host: "api.example.test", AuthMode: "sub2api_user_token", RecoveryMethod: "vault", VaultEntryID: &name}); err != nil {
				t.Fatal(err)
			}
			for _, tc := range []struct {
				name, source string
				entry        configstore.VaultEntry
				present      map[string]bool
				want         error
			}{
				{"duplicate", name, configstore.VaultEntry{ID: "existing", Entry: "existing"}, map[string]bool{}, configstore.ErrVaultEntryExists},
				{"missing source", "missing", configstore.VaultEntry{ID: "renamed", Entry: "renamed"}, map[string]bool{}, configstore.ErrVaultEntryMissing},
				{"invalid field", name, configstore.VaultEntry{ID: "renamed", Entry: "renamed", Hosts: []string{""}}, map[string]bool{"hosts": true}, nil},
			} {
				t.Run(tc.name, func(t *testing.T) {
					err := store.UpdateVaultEntry(t.Context(), tc.source, tc.entry, tc.present)
					if err == nil || (tc.want != nil && !errors.Is(err, tc.want)) {
						t.Fatalf("unexpected error: %v", err)
					}
					got, err := store.VaultEntry(t.Context(), name)
					if err != nil || !reflect.DeepEqual(got, &original) {
						t.Fatal("failed edit changed original", err)
					}
					renamed, err := store.VaultEntry(t.Context(), "renamed")
					if err != nil || renamed != nil {
						t.Fatal("failed edit created target", err)
					}
					target, err := store.VaultEntry(t.Context(), "existing")
					if err != nil || target == nil || target.Password != nil {
						t.Fatal("failed edit overwrote target", err)
					}
					preference, err := store.AuthRecoveryPreference(t.Context(), "api.example.test")
					if err != nil || preference == nil || preference.VaultEntryID == nil || *preference.VaultEntryID != name {
						t.Fatal("failed edit changed selection", err)
					}
				})
			}
		})
	}
}

func TestVaultRenameAppliesExplicitChangesAndPreservesOmittedFields(t *testing.T) {
	for _, config := range databaseConfigs(t) {
		t.Run(string(config.Driver), func(t *testing.T) {
			store := openContractStore(t, config)
			secret := "retained-secret"
			if err := store.SaveVaultEntry(t.Context(), configstore.VaultEntry{ID: "original", Entry: "original", Username: &secret, Password: &secret, Headers: map[string]string{"X-Private": secret}}, nil); err != nil {
				t.Fatal(err)
			}
			if err := store.UpdateVaultEntry(t.Context(), "original", configstore.VaultEntry{ID: "新名称", Entry: "新名称", Hosts: []string{"new.example.test"}}, map[string]bool{"username": true, "hosts": true}); err != nil {
				t.Fatal(err)
			}
			got, err := store.VaultEntry(t.Context(), "original")
			if err != nil || got == nil || got.Username != nil || got.Password == nil || *got.Password != secret || got.Headers["X-Private"] != secret || !reflect.DeepEqual(got.Hosts, []string{"new.example.test"}) {
				t.Fatal("rename patch did not preserve omitted fields", err)
			}
		})
	}
}
