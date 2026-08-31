package configstore

import (
	"context"
	"path/filepath"
	"testing"
)

func TestUpstreamKeySecretUsesExactBindingAndReplacesRotatedSecret(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "config.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	value := UpstreamKeySecret{
		Host: "HTTPS://API.EXAMPLE/", KeyID: "91", GroupID: "7", Secret: "first-secret",
	}
	if err := store.SaveUpstreamKeySecret(ctx, value); err != nil {
		t.Fatal(err)
	}
	stored, err := store.UpstreamKeySecret(ctx, "api.example", "91", "7")
	if err != nil {
		t.Fatal(err)
	}
	if stored == nil || stored.Host != "api.example" || stored.Secret != "first-secret" {
		t.Fatalf("stored secret=%#v", stored)
	}
	missing, err := store.UpstreamKeySecret(ctx, "api.example", "91", "8")
	if err != nil || missing != nil {
		t.Fatalf("different binding returned secret=%#v err=%v", missing, err)
	}
	value.Host, value.Secret = "api.example", "rotated-secret"
	if err := store.SaveUpstreamKeySecret(ctx, value); err != nil {
		t.Fatal(err)
	}
	stored, err = store.UpstreamKeySecret(ctx, "api.example", "91", "7")
	if err != nil || stored == nil || stored.Secret != "rotated-secret" {
		t.Fatalf("rotated secret=%#v err=%v", stored, err)
	}
}

func TestUpstreamKeySecretRejectsIncompleteValues(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "config.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	for name, value := range map[string]UpstreamKeySecret{
		"host":   {KeyID: "91", GroupID: "7", Secret: "secret"},
		"key":    {Host: "api.example", GroupID: "7", Secret: "secret"},
		"group":  {Host: "api.example", KeyID: "91", Secret: "secret"},
		"secret": {Host: "api.example", KeyID: "91", GroupID: "7"},
	} {
		t.Run(name, func(t *testing.T) {
			if err := store.SaveUpstreamKeySecret(context.Background(), value); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}
