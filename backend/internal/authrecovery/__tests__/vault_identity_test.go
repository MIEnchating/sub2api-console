package authrecovery_test

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/authrecovery"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/upstreamauth"
)

func TestDeletedVaultBindingDoesNotUseSameNameReplacement(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		http.Error(w, "unexpected request", http.StatusBadRequest)
	}))
	t.Cleanup(upstream.Close)
	repository, err := business.Open(filepath.Join(t.TempDir(), "business.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repository.Close() })
	if err := repository.Bootstrap(t.Context()); err != nil {
		t.Fatal(err)
	}
	private, err := configstore.Open(filepath.Join(t.TempDir(), "private.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = private.Close() })
	record := configstore.AuthRecord{Host: "api.example.test", BaseURL: upstream.URL, UpstreamType: "sub2api", AuthMode: "sub2api_user_login"}
	if err := private.SaveAuthRecord(t.Context(), record, nil); err != nil {
		t.Fatal(err)
	}
	username, password, id := "operator@example.test", "secret", configstore.NewVaultEntryID()
	if err := private.SaveVaultEntry(t.Context(), configstore.VaultEntry{ID: id, Entry: "管理员", Username: &username, Password: &password, Hosts: []string{record.Host}}, nil); err != nil {
		t.Fatal(err)
	}
	if err := private.SaveAuthRecoveryPreference(t.Context(), configstore.AuthRecoveryPreference{Host: record.Host, AuthMode: record.AuthMode, RecoveryMethod: "vault", VaultEntryID: &id}); err != nil {
		t.Fatal(err)
	}
	if _, err := private.DeleteVaultEntry(t.Context(), id); err != nil {
		t.Fatal(err)
	}
	if err := private.SaveVaultEntry(t.Context(), configstore.VaultEntry{Entry: "管理员", Username: &username, Password: &password, Hosts: []string{record.Host}}, nil); err != nil {
		t.Fatal(err)
	}
	service := authrecovery.New(repository, private, upstreamauth.New(upstream.Client()), nil, nil, nil)
	result, err := service.RecoverInvalid(t.Context(), []string{record.Host}, "test")
	if err != nil || result.Failed != 1 || calls.Load() != 0 {
		t.Fatalf("deleted binding must require reselection without network calls: result=%+v err=%v calls=%d", result, err, calls.Load())
	}
}
