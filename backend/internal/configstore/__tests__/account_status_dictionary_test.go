package configstore_test

import (
	"path/filepath"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func TestAccountStatusDictionaryAddsManualFuseAndPreservesOrderOnReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private.db")
	store, err := configstore.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	items, err := store.ListDictionaries(t.Context(), "account_status")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	ids := make([]string, 0, len(items))
	for _, item := range items {
		if item.Value == "manual_fused" {
			found = item.Name == "人工熔断"
			if err := store.DeleteDictionary(t.Context(), item.ID, item.Version); err != nil {
				t.Fatal(err)
			}
		} else {
			ids = append([]string{item.ID}, ids...)
		}
	}
	if !found {
		t.Fatal("missing 人工熔断 dictionary entry")
	}
	if err := store.ReorderDictionaries(t.Context(), "account_status", ids); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = configstore.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	items, err = store.ListDictionaries(t.Context(), "account_status")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != len(ids)+1 || items[len(ids)].Value != "manual_fused" {
		t.Fatalf("new default not appended: %+v", items)
	}
	for i, id := range ids {
		if items[i].ID != id {
			t.Fatal("saved order changed")
		}
	}
}
