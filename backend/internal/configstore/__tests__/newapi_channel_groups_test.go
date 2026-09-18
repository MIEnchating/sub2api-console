package configstore_test

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func TestNewAPIChannelGroupsPersistAndRejectStaleWrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private.db")
	store, err := configstore.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	initial, err := store.NewAPIChannelGroups(ctx, "primary")
	if err != nil || len(initial.Groups) != 0 || initial.Version != "" {
		t.Fatalf("unexpected initial groups: %+v %v", initial, err)
	}
	saved, err := store.SaveNewAPIChannelGroups(ctx, "primary", []configstore.NewAPIChannelGroup{{ID: "group-prod", Name: "生产", ChannelIDs: []string{"42", "41"}}}, "")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(saved.Groups[0].ChannelIDs, []string{"41", "42"}) || saved.Version == "" {
		t.Fatalf("groups were not normalized: %+v", saved)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = configstore.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	_, err = store.SaveNewAPIChannelGroups(ctx, "primary", nil, "")
	if err == nil {
		t.Fatal("stale empty-version write accepted")
	}
	restored, err := store.NewAPIChannelGroups(ctx, "primary")
	if err != nil || !reflect.DeepEqual(restored, saved) {
		t.Fatalf("groups were not persisted: %+v %v", restored, err)
	}
	_, err = store.SaveNewAPIChannelGroups(ctx, "primary", nil, "stale")
	if err == nil {
		t.Fatal("stale version accepted")
	}
	_, err = store.SaveNewAPIChannelGroups(ctx, "primary", nil, saved.Version)
	if err != nil {
		t.Fatal(err)
	}
}

func TestChannelGroupsAreClearedWhenPlatformTargetChanges(t *testing.T) {
	store, err := configstore.Open(filepath.Join(t.TempDir(), "private.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	platform := configstore.NewAPIPlatform{ID: "primary", Name: "测试", BaseURL: "https://groups-fixture.invalid", UserID: "1", AdminKey: "test-key"}
	if _, err := store.SaveNewAPIPlatform(ctx, platform); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveNewAPIChannelGroups(ctx, platform.ID, []configstore.NewAPIChannelGroup{{ID: "prod", Name: "生产", ChannelIDs: []string{"42"}}}, ""); err != nil {
		t.Fatal(err)
	}
	platform.BaseURL = "https://other-groups-fixture.invalid"
	if _, err := store.SaveNewAPIPlatform(ctx, platform); err != nil {
		t.Fatal(err)
	}
	groups, err := store.NewAPIChannelGroups(ctx, platform.ID)
	if err != nil || len(groups.Groups) != 0 {
		t.Fatalf("old target groups retained: %+v %v", groups, err)
	}
}

func TestNewAPIChannelGroupsRejectInvalidInput(t *testing.T) {
	store, err := configstore.Open(filepath.Join(t.TempDir(), "private.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	for _, groups := range [][]configstore.NewAPIChannelGroup{
		{{ID: "all", Name: "生产", ChannelIDs: []string{"1"}}},
		{{ID: "group-a", Name: "生产", ChannelIDs: []string{"1", "1"}}},
		{{ID: "group-a", Name: "生产", ChannelIDs: []string{"99999999999999999999"}}},
		{{ID: "group-a", Name: "", ChannelIDs: []string{"1"}}},
		{{ID: "group-a", Name: "重复", ChannelIDs: []string{"0"}}},
		{{ID: "group-a", Name: "重复", ChannelIDs: []string{"1"}}, {ID: "group-b", Name: "重复", ChannelIDs: []string{"2"}}},
	} {
		if _, err := store.SaveNewAPIChannelGroups(context.Background(), "primary", groups, ""); err == nil {
			t.Fatalf("invalid groups accepted: %+v", groups)
		}
	}
	if _, err := store.NewAPIChannelGroups(context.Background(), "bad platform"); err == nil {
		t.Fatal("invalid platform accepted")
	}
}
