package upstreamconfig_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/mutationguard"
	"github.com/MIEnchating/sub2api-console/backend/internal/upstreamconfig"
)

type vaultReservationVerifier struct {
	repository *business.Store
	concurrent bool
}

func (*vaultReservationVerifier) Verify(context.Context, configstore.AuthRecord) error { return nil }

func (v *vaultReservationVerifier) Login(ctx context.Context, record configstore.AuthRecord, credential configstore.VaultEntry) (configstore.AuthRecord, error) {
	resources := []string{mutationguard.Vault(credential.Entry)}
	acquired, err := v.repository.AcquireMutationLease(ctx, "competing-vault-editor", resources, time.Now().UTC(), time.Minute)
	if err != nil {
		return record, err
	}
	v.concurrent = acquired
	if acquired {
		if err := v.repository.ReleaseMutationLease(ctx, "competing-vault-editor", resources); err != nil {
			return record, err
		}
	}
	return record, nil
}

func TestHostRenameReservesTheVaultEntryBeingWritten(t *testing.T) {
	for _, explicitEntry := range []string{"", "shared-login"} {
		t.Run("entry="+explicitEntry, func(t *testing.T) {
			repository, err := business.Open(filepath.Join(t.TempDir(), "business.sqlite3"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = repository.Close() })
			private, err := configstore.Open(filepath.Join(t.TempDir(), "config.sqlite3"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = private.Close() })
			ctx := context.Background()
			name, token := "test upstream", "test-token"
			verifier := &vaultReservationVerifier{repository: repository}
			service := upstreamconfig.New(repository, private, verifier)
			_, err = service.Create(ctx, upstreamconfig.Input{
				Host: "old.example", Name: &name, BaseURL: "https://old.example", UpstreamType: "sub2api",
				AuthMode: "sub2api_user_token", RechargeRate: "1", AccessToken: &token, RefreshToken: &token,
				Present: map[string]bool{"access_token": true, "refresh_token": true},
			}, "test")
			if err != nil {
				t.Fatal(err)
			}
			username, password := "test-user", "test-password"
			input := upstreamconfig.Input{
				Name: &name, BaseURL: "https://new.example", UpstreamType: "sub2api", AuthMode: "sub2api_manual_login",
				RechargeRate: "1", Username: &username, Password: &password, SaveToVault: true,
				Present: map[string]bool{"username": true, "password": true},
			}
			entry := "new.example"
			if explicitEntry != "" {
				input.Entry = &explicitEntry
				entry = explicitEntry
			}
			if _, err := service.Update(ctx, "old.example", input, "test"); err != nil {
				t.Fatal(err)
			}
			if verifier.concurrent {
				t.Fatalf("renaming the Host left vault entry %q available to a concurrent editor", entry)
			}
			stored, err := private.VaultEntry(ctx, entry)
			if err != nil || stored == nil || stored.Username == nil || *stored.Username != username {
				t.Fatalf("renamed upstream login was not saved in its reserved vault entry: entry=%q err=%v", entry, err)
			}
		})
	}
}
