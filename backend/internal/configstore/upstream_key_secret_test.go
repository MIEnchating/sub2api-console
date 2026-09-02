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

func TestDeleteUpstreamKeySecretsRemovesEveryGroupForExactHostAndKey(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "config.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	for _, value := range []UpstreamKeySecret{
		{Host: "AUTH.EXAMPLE", KeyID: "91", GroupID: "7", Secret: "first"},
		{Host: "auth.example", KeyID: "91", GroupID: "8", Secret: "second"},
		{Host: "auth.example", KeyID: "92", GroupID: "7", Secret: "other-key"},
		{Host: "other.example", KeyID: "91", GroupID: "7", Secret: "other-host"},
	} {
		if err := store.SaveUpstreamKeySecret(ctx, value); err != nil {
			t.Fatal(err)
		}
	}

	if err := store.DeleteUpstreamKeySecrets(ctx, "https://AUTH.EXAMPLE/", "91"); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteUpstreamKeySecrets(ctx, "auth.example", "91"); err != nil {
		t.Fatalf("idempotent second delete failed: %v", err)
	}
	for _, groupID := range []string{"7", "8"} {
		stored, err := store.UpstreamKeySecret(ctx, "auth.example", "91", groupID)
		if err != nil || stored != nil {
			t.Fatalf("deleted secret remained for group %s: %#v err=%v", groupID, stored, err)
		}
	}
	for _, lookup := range []struct {
		host, keyID string
	}{
		{host: "auth.example", keyID: "92"},
		{host: "other.example", keyID: "91"},
	} {
		stored, err := store.UpstreamKeySecret(ctx, lookup.host, lookup.keyID, "7")
		if err != nil || stored == nil {
			t.Fatalf("unrelated secret was removed for %s/%s: %#v err=%v", lookup.host, lookup.keyID, stored, err)
		}
	}
}

func TestDeleteUpstreamKeySecretsRejectsIncompleteScope(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "config.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	for _, scope := range [][2]string{{"", "91"}, {"auth.example", ""}} {
		if err := store.DeleteUpstreamKeySecrets(context.Background(), scope[0], scope[1]); err == nil {
			t.Fatalf("incomplete secret-delete scope was accepted: %#v", scope)
		}
	}
}
