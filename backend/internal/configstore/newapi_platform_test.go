package configstore

import (
	"context"
	"testing"
)

func TestNewAPIPlatformIsStoredAsOnePrimaryPlatform(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	first, err := store.SaveNewAPIPlatform(ctx, NewAPIPlatform{
		Name: "主平台", BaseURL: "https://newapi.example", AdminKey: "first-key", UserID: "1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != "primary" {
		t.Fatalf("new platform id=%q", first.ID)
	}

	second, err := store.SaveNewAPIPlatform(ctx, NewAPIPlatform{
		ID: "replacement", Name: "替换平台", BaseURL: "https://replacement.example", AdminKey: "second-key", UserID: "2",
	})
	if err != nil {
		t.Fatal(err)
	}
	platforms, err := store.NewAPIPlatforms(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(platforms) != 1 || platforms[0].ID != second.ID || platforms[0].Name != "替换平台" {
		t.Fatalf("platforms=%#v", platforms)
	}
	if current, err := store.NewAPIPlatform(ctx, first.ID); err != nil || current != nil {
		t.Fatalf("old primary still exists: current=%#v err=%v", current, err)
	}
}
