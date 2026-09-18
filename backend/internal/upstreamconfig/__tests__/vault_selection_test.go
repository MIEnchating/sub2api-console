package upstreamconfig_test

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/upstreamconfig"
)

type vaultSelectionVerifier struct{ fail bool }

func (*vaultSelectionVerifier) Verify(context.Context, configstore.AuthRecord) error { return nil }
func (v *vaultSelectionVerifier) Login(_ context.Context, record configstore.AuthRecord, _ configstore.VaultEntry) (configstore.AuthRecord, error) {
	if v.fail {
		return record, errors.New("isolated login failure")
	}
	return record, nil
}

func TestVaultSelectionConfiguration(t *testing.T) {
	for _, scenario := range []string{"previous_login", "create", "update", "failed_update"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := t.Context()
			repository, err := business.Open(filepath.Join(t.TempDir(), "business.db"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = repository.Close() })
			private, err := configstore.Open(filepath.Join(t.TempDir(), "private.db"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = private.Close() })
			name, entry, username, password := "测试上游", "共享登录资料", "operator@example.test", "private-password"
			for _, key := range []string{entry, "备用资料"} {
				if err := private.SaveVaultEntry(ctx, configstore.VaultEntry{Entry: key, Username: &username, Password: &password}, nil); err != nil {
					t.Fatal(err)
				}
			}
			verifier := &vaultSelectionVerifier{}
			service := upstreamconfig.New(repository, private, verifier)
			input := upstreamconfig.Input{Host: "api.example.test", Name: &name, BaseURL: "https://api.example.test", UpstreamType: "sub2api", AuthMode: "sub2api_user_login", RechargeRate: "1", Entry: &entry, Present: map[string]bool{"entry": true}}
			if _, err := service.Create(ctx, input, "test"); err != nil {
				t.Fatal(err)
			}
			if scenario != "create" {
				if err := private.SaveAuthRecoveryPreference(ctx, configstore.AuthRecoveryPreference{Host: input.Host, AuthMode: input.AuthMode, RecoveryMethod: "vault", VaultEntry: &entry}); err != nil {
					t.Fatal(err)
				}
			}
			want := entry
			if scenario == "update" || scenario == "failed_update" {
				next := "备用资料"
				input.Entry = &next
				verifier.fail = scenario == "failed_update"
				_, err := service.Update(ctx, input.Host, input, "test")
				if verifier.fail && err == nil {
					t.Fatal("failed login was accepted")
				}
				if !verifier.fail {
					if err != nil {
						t.Fatal(err)
					}
					want = next
				}
			}
			configuration, err := service.Get(ctx, input.Host)
			if err != nil {
				t.Fatal(err)
			}
			encoded, err := json.Marshal(configuration)
			if err != nil {
				t.Fatal(err)
			}
			var response map[string]any
			if err := json.Unmarshal(encoded, &response); err != nil {
				t.Fatal(err)
			}
			if response["entry"] != want {
				t.Fatalf("selected entry = %v, want %s", response["entry"], want)
			}
			for _, key := range []string{"username", "password", "access_token"} {
				if _, ok := response[key]; ok {
					t.Fatalf("configuration exposes %s", key)
				}
			}
		})
	}
}
