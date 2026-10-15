package api_test

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/api"
	"github.com/MIEnchating/sub2api-console/backend/internal/config"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func TestVaultRenamePreservesSecretsAndRecoverySelection(t *testing.T) {
	store, err := configstore.Open(filepath.Join(t.TempDir(), "vault.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Initialize(t.Context(), "operator", "current-password", "https://isolated.example", "fixture-key"); err != nil {
		t.Fatal(err)
	}
	name, username, password := "original", "operator", "private-password"
	if err := store.SaveVaultEntry(t.Context(), configstore.VaultEntry{ID: name, Entry: name, Username: &username, Password: &password, Hosts: []string{"api.example.test"}, Headers: map[string]string{"X-Private": "secret"}}, nil); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveAuthRecoveryPreference(t.Context(), configstore.AuthRecoveryPreference{Host: "api.example.test", AuthMode: "sub2api_user_token", RecoveryMethod: "vault", VaultEntryID: &name}); err != nil {
		t.Fatal(err)
	}
	router := api.New(config.Config{AdminToken: "test-token"}, store, nil)
	request := httptest.NewRequest(http.MethodPost, "/api/auth-recovery/vault-entry", strings.NewReader(`{"entry":"renamed","id":"original"}`))
	request.Header.Set("Authorization", "Bearer test-token")
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("rename: %d %s", response.Code, response.Body.String())
	}
	stored, err := store.VaultEntry(t.Context(), "original")
	if err != nil || stored == nil || stored.ID != "original" || stored.Entry != "renamed" || stored.Username == nil || *stored.Username != username || stored.Password == nil || *stored.Password != password || stored.Headers["X-Private"] != "secret" || len(stored.Hosts) != 1 {
		t.Fatal("rename lost original fields", err)
	}
	preference, err := store.AuthRecoveryPreference(t.Context(), "api.example.test")
	if err != nil || preference == nil || preference.VaultEntryID == nil || *preference.VaultEntryID != "original" {
		t.Fatal("recovery selection did not follow rename", err)
	}
	if strings.Contains(response.Body.String(), password) || strings.Contains(response.Body.String(), "secret") {
		t.Fatal("response leaked credentials")
	}
}
