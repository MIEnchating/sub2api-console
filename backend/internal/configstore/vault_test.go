package configstore

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestVaultEntryUsesExactSelectionAndRedactedIndex(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "config.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	username, password := "operator@example.test", "secret"
	if err := store.SaveVaultEntry(ctx, VaultEntry{
		Entry: "Primary", Username: &username, Password: &password,
		Hosts: []string{"HTTPS://API.EXAMPLE/", "api.example"}, Headers: map[string]string{"X-Site": "one"},
	}, nil); err != nil {
		t.Fatal(err)
	}
	selected, err := store.VaultEntry(ctx, "primary")
	if err != nil {
		t.Fatal(err)
	}
	if selected != nil {
		t.Fatal("密码箱名称必须精确匹配，不能隐式忽略大小写")
	}
	selected, err = store.VaultEntry(ctx, "Primary")
	if err != nil || selected == nil || len(selected.Hosts) != 1 || selected.Hosts[0] != "api.example" {
		t.Fatalf("unexpected selected entry: %#v err=%v", selected, err)
	}
	index, err := store.VaultEntryIndex(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(index) != 1 || !index[0].HasUsername || !index[0].HasPassword || !index[0].UsernameIsEmail || len(index[0].HeaderNames) != 1 {
		t.Fatalf("unexpected redacted index: %#v", index)
	}
}

func TestVaultEntryNameUsesUnicodeCharacterLimit(t *testing.T) {
	store := openTestStore(t)
	entry := strings.Repeat("凭", 255)
	if err := store.SaveVaultEntry(context.Background(), VaultEntry{Entry: entry}, nil); err != nil {
		t.Fatalf("valid Unicode vault entry name was rejected: %v", err)
	}
	stored, err := store.VaultEntry(context.Background(), entry)
	if err != nil || stored == nil || stored.Entry != entry {
		t.Fatalf("stored=%#v err=%v", stored, err)
	}
}

func TestEmailUsernameClassificationDoesNotExposeUsername(t *testing.T) {
	valid, invalid := "operator@example.test", "operator"
	if !IsEmailUsername(&valid) || IsEmailUsername(&invalid) || IsEmailUsername(nil) {
		t.Fatal("unexpected email username classification")
	}
}

func TestVaultEntryPatchPreservesOmittedAndClearsExplicitNull(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "config.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	username, password := "operator", "secret"
	if err := store.SaveVaultEntry(ctx, VaultEntry{Entry: "entry", Username: &username, Password: &password}, nil); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveVaultEntry(ctx, VaultEntry{Entry: "entry", Username: nil}, map[string]bool{"username": true}); err != nil {
		t.Fatal(err)
	}
	stored, err := store.VaultEntry(ctx, "entry")
	if err != nil || stored == nil || stored.Username != nil || stored.Password == nil || *stored.Password != "secret" {
		t.Fatalf("unexpected patched entry: %#v err=%v", stored, err)
	}
}
