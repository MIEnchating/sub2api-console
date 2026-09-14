package configstore

import (
	"context"
	"errors"
	"testing"
)

func TestDictionaryCRUDAndReorder(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	first, err := store.SaveDictionary(ctx, DictionaryEntry{Kind: "platform", Name: "主平台", Value: "primary", Enabled: true}, 0)
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.SaveDictionary(ctx, DictionaryEntry{Kind: "platform", Name: "备用平台", Value: "backup", Enabled: true}, 0)
	if err != nil {
		t.Fatal(err)
	}
	items, err := store.ListDictionaries(ctx, "platform")
	if err != nil || len(items) != 2 {
		t.Fatalf("items=%#v err=%v", items, err)
	}
	updated, err := store.SaveDictionary(ctx, DictionaryEntry{ID: first.ID, Kind: first.Kind, Name: "主平台（更新）", Value: first.Value, Enabled: true, Version: first.Version}, first.Version)
	if err != nil || updated.Version != first.Version+1 {
		t.Fatalf("updated=%#v err=%v", updated, err)
	}
	if _, err := store.SaveDictionary(ctx, DictionaryEntry{ID: first.ID, Kind: first.Kind, Name: "冲突", Value: first.Value, Version: first.Version}, first.Version); !errors.Is(err, ErrDictionaryConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}
	if err := store.ReorderDictionaries(ctx, "platform", []string{second.ID, first.ID}); err != nil {
		t.Fatal(err)
	}
	items, err = store.ListDictionaries(ctx, "platform")
	if err != nil || items[0].ID != second.ID {
		t.Fatalf("reordered=%#v err=%v", items, err)
	}
	if err := store.DeleteDictionary(ctx, second.ID, items[0].Version); err != nil {
		t.Fatal(err)
	}
}

func TestSyncDictionaryValuesPreservesManualOrder(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	first, err := store.SaveDictionary(ctx, DictionaryEntry{Kind: "platform", Name: "第一", Value: "first", Enabled: true}, 0)
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.SaveDictionary(ctx, DictionaryEntry{Kind: "platform", Name: "第二", Value: "second", Enabled: true}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ReorderDictionaries(ctx, "platform", []string{second.ID, first.ID}); err != nil {
		t.Fatal(err)
	}
	if err := store.SyncDictionaryValues(ctx, "platform", []DictionaryEntry{
		{Value: first.Value, Name: "第一"},
		{Value: second.Value, Name: "第二"},
	}); err != nil {
		t.Fatal(err)
	}
	items, err := store.ListDictionaries(ctx, "platform")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].ID != second.ID {
		t.Fatalf("sync reset dictionary order: %#v", items)
	}
}
