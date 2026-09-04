package configstore

import (
	"context"
	"strings"
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

func TestNewAPIPlatformUsesUnicodeCharacterLimits(t *testing.T) {
	store := openTestStore(t)
	name := strings.Repeat("平", 120)
	userID := strings.Repeat("户", 128)
	platform, err := store.SaveNewAPIPlatform(context.Background(), NewAPIPlatform{
		Name: name, BaseURL: "https://newapi.example", AdminKey: "admin-key", UserID: userID,
	})
	if err != nil {
		t.Fatalf("valid Unicode platform fields were rejected: %v", err)
	}
	if platform.Name != name || platform.UserID != userID {
		t.Fatalf("platform=%#v", platform)
	}
}
