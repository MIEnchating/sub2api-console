package configstore

import (
	"context"
	"path/filepath"
	"testing"
)

func TestAuthRecoveryPreferenceKeepsLastVaultEntryAcrossRefreshSuccess(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "private.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	entry := "Primary"
	if err := store.SaveAuthRecoveryPreference(ctx, AuthRecoveryPreference{
		Host: "HTTPS://API.EXAMPLE/", AuthMode: "sub2api_user_login", RecoveryMethod: "vault",
		VaultEntry: &entry,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveAuthRecoveryPreference(ctx, AuthRecoveryPreference{
		Host: "api.example", AuthMode: "sub2api_user_token", RecoveryMethod: "refresh_token",
	}); err != nil {
		t.Fatal(err)
	}
	preference, err := store.AuthRecoveryPreference(ctx, "api.example")
	if err != nil {
		t.Fatal(err)
	}
	if preference == nil || preference.AuthMode != "sub2api_user_token" || preference.RecoveryMethod != "refresh_token" || preference.VaultEntry == nil || *preference.VaultEntry != entry {
		t.Fatalf("preference=%#v", preference)
	}
}

func TestDeleteAuthRecordAlsoDeletesRecoveryPreference(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "private.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	token := "token"
	if err := store.SaveAuthRecord(ctx, AuthRecord{
		Host: "api.example", BaseURL: "https://api.example", UpstreamType: "sub2api",
		AuthMode: "sub2api_user_token", AccessToken: &token,
	}, nil); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveAuthRecoveryPreference(ctx, AuthRecoveryPreference{
		Host: "api.example", AuthMode: "sub2api_user_token", RecoveryMethod: "manual",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.DeleteAuthRecord(ctx, "api.example"); err != nil {
		t.Fatal(err)
	}
	preference, err := store.AuthRecoveryPreference(ctx, "api.example")
	if err != nil || preference != nil {
		t.Fatalf("preference=%#v err=%v", preference, err)
	}
}
