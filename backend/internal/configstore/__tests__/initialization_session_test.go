package configstore_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func TestInitializationSessionPreservesTargetAndRejectsOverwrite(t *testing.T) {
	for _, mode := range []string{"managed", "existing", "local"} {
		t.Run(mode, func(t *testing.T) {
			store, err := configstore.Open(filepath.Join(t.TempDir(), "initialize.sqlite3"))
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			ctx := context.Background()
			base, key := "https://isolated.example", "isolated-key"
			if mode == "existing" {
				if err := store.ConfigureTarget(ctx, base, key, 30); err != nil {
					t.Fatal(err)
				}
			}
			if mode != "managed" {
				base, key = "", ""
			}
			now := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
			token, err := store.InitializeSession(ctx, "operator", "private-password", base, key, mode == "local", time.Hour, now)
			if err != nil {
				t.Fatal(err)
			}
			status, err := store.PublicStatus(ctx)
			if err != nil || !status.Initialized || status.TargetConfigured != (mode != "local") {
				t.Fatalf("wrong initialized state: %v %v", status, err)
			}
			if mode != "local" {
				target, err := store.TargetSettings(ctx)
				if err != nil || target.BaseURL != "https://isolated.example" || target.AdminKey != "isolated-key" {
					t.Fatalf("target changed: %v", err)
				}
			}
			if _, err := store.InitializeSession(ctx, "replacement", "replacement-password", "", "", true, time.Hour, now); err == nil {
				t.Fatal("existing credentials overwritten")
			}
			user, err := store.SessionUser(ctx, token, now.Add(time.Second))
			if err != nil || user == nil || *user != "operator" {
				t.Fatalf("initial session unavailable: %v", err)
			}
			user, err = store.SessionUser(ctx, token, now.Add(time.Hour))
			if err != nil || user != nil {
				t.Fatalf("session exceeded expiry: %v", err)
			}
		})
	}
}
