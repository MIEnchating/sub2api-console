package configstore

import (
	"context"
	"errors"
	"testing"
)

func TestBuiltInDictionariesHaveValuesAndPreserveSavedOrder(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	for _, kind := range []string{"account_type", "upstream_type", "auth_status", "scheduling_strategy", "task_status", "account_status", "alert_status", "kuma_monitor_type"} {
		t.Run(kind, func(t *testing.T) {
			items, err := store.ListDictionaries(ctx, kind)
			if err != nil || len(items) < 2 {
				t.Fatalf("missing defaults: %v, %v", items, err)
			}
			ids := make([]string, len(items))
			for i := range items {
				ids[i] = items[len(items)-1-i].ID
			}
			if err := store.ReorderDictionaries(ctx, kind, ids); err != nil {
				t.Fatal(err)
			}
			refreshed, err := store.ListDictionaries(ctx, kind)
			if err != nil || refreshed[0].ID != ids[0] {
				t.Fatalf("saved order lost: %v, %v", refreshed, err)
			}
		})
	}
}

func TestDictionaryReorderRejectsInvalidIDsWithoutPartialWrites(t *testing.T) {
	for _, invalid := range []string{"missing", "duplicate", "foreign"} {
		t.Run(invalid, func(t *testing.T) {
			store := openTestStore(t)
			ctx := context.Background()
			items, err := store.ListDictionaries(ctx, "upstream_type")
			if err != nil {
				t.Fatal(err)
			}
			ids := make([]string, len(items))
			for i := range items {
				ids[i] = items[len(items)-1-i].ID
			}
			switch invalid {
			case "missing":
				ids = ids[:len(ids)-1]
			case "duplicate":
				ids[len(ids)-1] = ids[0]
			case "foreign":
				other, err := store.ListDictionaries(ctx, "account_type")
				if err != nil {
					t.Fatal(err)
				}
				ids[len(ids)-1] = other[0].ID
			}
			if err := store.ReorderDictionaries(ctx, "upstream_type", ids); !errors.Is(err, ErrDictionaryConflict) {
				t.Fatalf("expected conflict, got %v", err)
			}
			actual, err := store.ListDictionaries(ctx, "upstream_type")
			if err != nil {
				t.Fatal(err)
			}
			for i := range items {
				if actual[i] != items[i] {
					t.Fatalf("partial write: got %+v, want %+v", actual[i], items[i])
				}
			}
		})
	}
}

func TestDictionarySyncAppendsNewValuesAfterSavedOrder(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	values := []DictionaryEntry{{Name: "B", Value: "b"}, {Name: "C", Value: "c"}}
	if err := store.SyncDictionaryValues(ctx, "platform", values); err != nil {
		t.Fatal(err)
	}
	items, _ := store.ListDictionaries(ctx, "platform")
	if err := store.ReorderDictionaries(ctx, "platform", []string{items[1].ID, items[0].ID}); err != nil {
		t.Fatal(err)
	}
	if err := store.SyncDictionaryValues(ctx, "platform", append([]DictionaryEntry{{Name: "A", Value: "a"}}, values...)); err != nil {
		t.Fatal(err)
	}
	items, err := store.ListDictionaries(ctx, "platform")
	if err != nil || len(items) != 3 || items[0].Value != "c" || items[2].Value != "a" {
		t.Fatalf("new value interrupted manual order: %v, %v", items, err)
	}
}
