package configstore_test

import (
	"errors"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func TestDatabaseContractDictionarySyncInvalidatesStaleEditor(t *testing.T) {
	for _, config := range databaseConfigs(t) {
		t.Run(string(config.Driver), func(t *testing.T) {
			store := openContractStore(t, config)
			entry, err := store.SaveDictionary(t.Context(), configstore.DictionaryEntry{Kind: "group", Name: "旧名称", Value: "41", Enabled: true}, 0)
			if err != nil {
				t.Fatal(err)
			}
			if err := store.SyncDictionaryValues(t.Context(), "group", []configstore.DictionaryEntry{{Name: "新名称", Value: "41"}}); err != nil {
				t.Fatal(err)
			}
			entry.Description = "旧页面提交"
			if _, err := store.SaveDictionary(t.Context(), entry, entry.Version); !errors.Is(err, configstore.ErrDictionaryConflict) {
				t.Fatalf("stale editor overwrote synchronization: %v", err)
			}
		})
	}
}

func TestDatabaseContractConcurrentDictionarySyncRetainsUniqueValuesAndOrder(t *testing.T) {
	for _, config := range databaseConfigs(t) {
		t.Run(string(config.Driver), func(t *testing.T) {
			store := openContractStore(t, config)
			values := []configstore.DictionaryEntry{{Name: "分组一", Value: "41"}, {Name: "分组二", Value: "42"}}
			const writers = 8
			start, results := make(chan struct{}), make(chan error, writers)
			for range writers {
				go func() { <-start; results <- store.SyncDictionaryValues(t.Context(), "group", values) }()
			}
			close(start)
			for range writers {
				if err := <-results; err != nil {
					t.Error(err)
				}
			}
			entries, err := store.ListDictionaries(t.Context(), "group")
			if err != nil || len(entries) != 2 {
				t.Fatalf("dictionary entries: %+v %v", entries, err)
			}
			for i, entry := range entries {
				if entry.Value != values[i].Value || entry.SortOrder != i {
					t.Fatalf("dictionary order: %+v", entries)
				}
			}
		})
	}
}
