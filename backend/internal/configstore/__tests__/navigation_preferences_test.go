package configstore_test

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func TestNavigationPreferencesPersistAcrossReopenAndRejectStaleWrites(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "private.db")
	store, err := configstore.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	initial, err := store.NavigationPreferences(ctx)
	if err != nil || initial.HiddenItemIDs != nil || initial.Version != "" {
		t.Fatalf("unexpected initial preferences: %+v %v", initial, err)
	}
	saved, err := store.SaveNavigationPreferences(ctx, configstore.NavigationPreferences{HiddenItemIDs: []string{"accounts", "config", "accounts"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := configstore.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	restored, err := reopened.NavigationPreferences(ctx)
	if err != nil || !reflect.DeepEqual(restored.HiddenItemIDs, []string{"accounts"}) || restored.Version != saved.Version {
		t.Fatalf("not persisted: %+v %v", restored, err)
	}
	_, err = reopened.SaveNavigationPreferences(ctx, configstore.NavigationPreferences{HiddenItemIDs: []string{}})
	if !errors.Is(err, configstore.ErrNavigationConflict) {
		t.Fatalf("stale write accepted: %v", err)
	}
	reset, err := reopened.SaveNavigationPreferences(ctx, configstore.NavigationPreferences{HiddenItemIDs: []string{}, Version: saved.Version})
	if err != nil || reset.HiddenItemIDs == nil || len(reset.HiddenItemIDs) != 0 {
		t.Fatalf("reset failed: %+v %v", reset, err)
	}
}

func TestNavigationPreferencesRejectInvalidIDsWithoutSaving(t *testing.T) {
	store, err := configstore.Open(filepath.Join(t.TempDir(), "private.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	for _, ids := range [][]string{nil, {"/accounts"}, {""}} {
		if _, err := store.SaveNavigationPreferences(context.Background(), configstore.NavigationPreferences{HiddenItemIDs: ids}); err == nil {
			t.Fatalf("invalid IDs accepted: %v", ids)
		}
	}
	result, err := store.NavigationPreferences(context.Background())
	if err != nil || result.Version != "" {
		t.Fatalf("invalid write persisted: %+v %v", result, err)
	}
}
