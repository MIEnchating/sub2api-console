package configstore_test

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func TestRetainedProbeKeyAfterReopeningPrivateStorePreservesExactBinding(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private.sqlite3")
	store, err := configstore.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	auth := configstore.AuthRecord{Host: "probe.test", BaseURL: "https://probe.test", UpstreamType: "sub2api", AuthMode: "bearer"}
	if err := store.SaveAuthRecord(t.Context(), auth, nil); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveRetainedProbeKey(t.Context(), auth, "6", "91"); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = configstore.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	id, err := store.RetainedProbeKey(t.Context(), auth, "6")
	if err != nil || id != "91" {
		t.Fatalf("binding lost after restart: id=%q err=%v", id, err)
	}
}

func TestRetainedProbeKeyChangedScopeDoesNotReusePreviousBinding(t *testing.T) {
	for _, field := range []string{"host", "group", "base_url", "auth", "user"} {
		t.Run(field, func(t *testing.T) {
			store, auth, _ := probeKeyAuthFixture(t)
			if err := store.SaveRetainedProbeKey(t.Context(), auth, "6", "91"); err != nil {
				t.Fatal(err)
			}
			group := "6"
			switch field {
			case "host":
				auth.Host = "other.test"
			case "group":
				group = "7"
			case "base_url":
				auth.BaseURL = "https://upstream.test/other"
			case "auth":
				value := "replacement-token"
				auth.AccessToken = &value
			case "user":
				value := "other-user"
				auth.UserID = &value
			}
			id, err := store.RetainedProbeKey(t.Context(), auth, group)
			if err != nil || id != "" {
				t.Fatalf("changed scope reused binding: id=%q err=%v", id, err)
			}
		})
	}
}

func TestRetainedProbeKeySavingAfterAuthReplacementRejectsStaleBinding(t *testing.T) {
	store, auth, _ := probeKeyAuthFixture(t)
	updated := auth
	updated.BaseURL = "https://upstream.test/replacement"
	if err := store.SaveAuthRecord(t.Context(), updated, nil); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveRetainedProbeKey(t.Context(), auth, "6", "91"); !errors.Is(err, configstore.ErrUpstreamKeyAuthChanged) {
		t.Fatalf("stale auth not rejected: %v", err)
	}
}
