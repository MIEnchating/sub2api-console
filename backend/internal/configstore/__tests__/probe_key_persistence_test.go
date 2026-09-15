package configstore_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func probeKeyAuthFixture(t *testing.T) (*configstore.Store, configstore.AuthRecord, configstore.UpstreamKeySecret) {
	t.Helper()
	store, err := configstore.Open(filepath.Join(t.TempDir(), "probe-key-private.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	auth := configstore.AuthRecord{
		Host: "upstream.test", BaseURL: "https://upstream.test", UpstreamType: "sub2api", AuthMode: "bearer",
		Headers: map[string]string{"x-custom": " probe "}, Cookies: map[string]string{"session": "test-session"},
	}
	if err := store.SaveAuthRecord(context.Background(), auth, nil); err != nil {
		t.Fatal(err)
	}
	key := configstore.UpstreamKeySecret{Host: auth.Host, KeyID: "91", GroupID: "7", Secret: "test-bound-key"}
	return store, auth, key
}

func TestProbeKeyPersistenceWithMatchingAuthSavesAndReplacesExactPrivateKey(t *testing.T) {
	store, auth, key := probeKeyAuthFixture(t)
	for _, secret := range []string{"test-first-key", "test-rotated-key"} {
		key.Secret = secret
		if err := store.SaveUpstreamKeySecretForAuth(context.Background(), key, auth); err != nil {
			t.Fatal(err)
		}
		cached, err := store.UpstreamKeySecret(context.Background(), key.Host, key.KeyID, key.GroupID)
		if err != nil || cached == nil || cached.Secret != secret {
			t.Fatal("matching authorization did not save the exact private key")
		}
	}
}

func TestProbeKeyPersistenceWithChangedAuthRejectsOldCredential(t *testing.T) {
	for _, change := range []struct {
		name  string
		apply func(*configstore.AuthRecord)
	}{
		{"base_url", func(auth *configstore.AuthRecord) { auth.BaseURL = "https://upstream.test/changed" }},
		{"platform", func(auth *configstore.AuthRecord) { auth.UpstreamType = "newapi" }},
		{"auth_mode", func(auth *configstore.AuthRecord) { auth.AuthMode = "admin" }},
		{"access_token", func(auth *configstore.AuthRecord) { value := "test-new-access"; auth.AccessToken = &value }},
		{"refresh_token", func(auth *configstore.AuthRecord) { value := "test-new-refresh"; auth.RefreshToken = &value }},
		{"admin_key", func(auth *configstore.AuthRecord) { value := "test-new-admin"; auth.AdminKey = &value }},
		{"user_id", func(auth *configstore.AuthRecord) { value := "42"; auth.UserID = &value }},
		{"headers", func(auth *configstore.AuthRecord) { auth.Headers = map[string]string{"X-Custom": "changed"} }},
		{"cookies", func(auth *configstore.AuthRecord) { auth.Cookies = map[string]string{"session": "changed"} }},
	} {
		t.Run(change.name, func(t *testing.T) {
			store, auth, key := probeKeyAuthFixture(t)
			updated := auth
			change.apply(&updated)
			if err := store.SaveAuthRecord(context.Background(), updated, nil); err != nil {
				t.Fatal(err)
			}
			if err := store.SaveUpstreamKeySecretForAuth(context.Background(), key, auth); !errors.Is(err, configstore.ErrUpstreamKeyAuthChanged) {
				t.Fatalf("changed authorization was not rejected: %v", err)
			}
			cached, err := store.UpstreamKeySecret(context.Background(), key.Host, key.KeyID, key.GroupID)
			if err != nil || cached != nil {
				t.Fatal("stale credential persisted after authorization changed")
			}
		})
	}
}

func TestProbeKeyPersistenceAfterAuthRenameOrDeletionDoesNotRestoreOldHost(t *testing.T) {
	for _, change := range []string{"renamed", "deleted"} {
		t.Run(change, func(t *testing.T) {
			store, auth, key := probeKeyAuthFixture(t)
			if change == "renamed" {
				if err := store.RenameAuthRecord(context.Background(), auth.Host, "renamed.test"); err != nil {
					t.Fatal(err)
				}
			} else if _, err := store.DeleteAuthRecord(context.Background(), auth.Host); err != nil {
				t.Fatal(err)
			}
			if err := store.SaveUpstreamKeySecretForAuth(context.Background(), key, auth); !errors.Is(err, configstore.ErrUpstreamKeyAuthChanged) {
				t.Fatalf("removed authorization was not rejected: %v", err)
			}
			cached, err := store.UpstreamKeySecret(context.Background(), key.Host, key.KeyID, key.GroupID)
			if err != nil || cached != nil {
				t.Fatal("stale credential restored the old host after rename or deletion")
			}
		})
	}
}

func TestProbeKeyPersistenceWithPopulatedAuthKeepsNullAndEmptyIdentityDistinct(t *testing.T) {
	store, auth, key := probeKeyAuthFixture(t)
	access, refresh, admin, user := "test-access", "test-refresh", "test-admin", "42"
	auth.AccessToken, auth.RefreshToken, auth.AdminKey, auth.UserID = &access, &refresh, &admin, &user
	if err := store.SaveAuthRecord(context.Background(), auth, nil); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveUpstreamKeySecretForAuth(context.Background(), key, auth); err != nil {
		t.Fatal("matching populated authorization failed")
	}
	auth.UserID = nil
	if err := store.SaveAuthRecord(context.Background(), auth, nil); err != nil {
		t.Fatal(err)
	}
	empty := ""
	auth.UserID = &empty
	key.Secret = "test-should-not-save"
	if err := store.SaveUpstreamKeySecretForAuth(context.Background(), key, auth); !errors.Is(err, configstore.ErrUpstreamKeyAuthChanged) {
		t.Fatalf("empty identity incorrectly matched SQL NULL: %v", err)
	}
	cached, err := store.UpstreamKeySecret(context.Background(), key.Host, key.KeyID, key.GroupID)
	if err != nil || cached == nil || cached.Secret != "test-bound-key" {
		t.Fatal("rejected authorization overwrote the existing private key")
	}
}

func TestProbeKeyPersistenceWithDifferentKeyHostDoesNotWriteOutsideExpectedAuth(t *testing.T) {
	store, auth, key := probeKeyAuthFixture(t)
	key.Host = "different.test"
	if err := store.SaveUpstreamKeySecretForAuth(context.Background(), key, auth); !errors.Is(err, configstore.ErrUpstreamKeyAuthChanged) {
		t.Fatalf("mismatched key host was not rejected: %v", err)
	}
	cached, err := store.UpstreamKeySecret(context.Background(), key.Host, key.KeyID, key.GroupID)
	if err != nil || cached != nil {
		t.Fatal("credential persisted under a host outside the expected authorization")
	}
}
