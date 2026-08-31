package configstore

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"
)

func TestAuthRecordPartialUpdatePreservesOmittedSecretsAndClearsExplicitNull(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "config.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	access, refresh, adminKey, userID := "access", "refresh", "admin", "24"
	if err := store.SaveAuthRecord(ctx, AuthRecord{
		Host: "HTTPS://API.EXAMPLE/", BaseURL: "https://api.example/", UpstreamType: "newapi", AuthMode: "newapi_admin_key",
		AccessToken: &access, RefreshToken: &refresh, AdminKey: &adminKey, UserID: &userID,
		Headers: map[string]string{"X-Custom": "value"}, Cookies: map[string]string{"session": "cookie"},
	}, nil); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveAuthRecord(ctx, AuthRecord{
		Host: "api.example", BaseURL: "https://api.example", UpstreamType: "newapi", AuthMode: "newapi_user_token",
		AccessToken: nil,
	}, map[string]bool{"base_url": true, "upstream_type": true, "auth_mode": true, "access_token": true}); err != nil {
		t.Fatal(err)
	}
	record, err := store.AuthRecord(ctx, "https://api.example/")
	if err != nil {
		t.Fatal(err)
	}
	if record == nil || record.AccessToken != nil || record.RefreshToken == nil || *record.RefreshToken != "refresh" || record.AdminKey == nil || *record.AdminKey != "admin" || !reflect.DeepEqual(record.Headers, map[string]string{"X-Custom": "value"}) || !reflect.DeepEqual(record.Cookies, map[string]string{"session": "cookie"}) {
		t.Fatalf("partial update lost presence semantics: %#v", record)
	}
	index, err := store.AuthRecordIndex(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(index) != 1 || index[0].HasAccessToken || !index[0].HasRefreshToken || !index[0].HasAdminKey || !reflect.DeepEqual(index[0].HeaderNames, []string{"X-Custom"}) || !reflect.DeepEqual(index[0].CookieNames, []string{"session"}) {
		t.Fatalf("unexpected redacted index: %#v", index)
	}
}

func TestAuthRecordRejectsHeaderInjectionWithoutChangingStoredRecord(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "config.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	access := "access"
	if err := store.SaveAuthRecord(ctx, AuthRecord{
		Host: "api.example", BaseURL: "https://api.example", UpstreamType: "sub2api", AuthMode: "sub2api_user_token",
		AccessToken: &access, Headers: map[string]string{}, Cookies: map[string]string{},
	}, nil); err != nil {
		t.Fatal(err)
	}
	err = store.SaveAuthRecord(ctx, AuthRecord{
		Host: "api.example", BaseURL: "https://api.example", UpstreamType: "sub2api", AuthMode: "custom_headers",
		Headers: map[string]string{"X-Test": "value\r\nInjected: true"},
	}, nil)
	if err == nil {
		t.Fatal("header injection was accepted")
	}
	record, readErr := store.AuthRecord(ctx, "api.example")
	if readErr != nil || record == nil || record.AuthMode != "sub2api_user_token" {
		t.Fatalf("failed update changed stored record: record=%#v err=%v", record, readErr)
	}
}

func TestDeleteAuthRecordAlsoDeletesCachedUpstreamKeys(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "config.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	access := "access"
	if err := store.SaveAuthRecord(ctx, AuthRecord{
		Host: "api.example", BaseURL: "https://api.example", UpstreamType: "sub2api", AuthMode: "sub2api_user_token",
		AccessToken: &access, Headers: map[string]string{}, Cookies: map[string]string{},
	}, nil); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveUpstreamKeySecret(ctx, UpstreamKeySecret{
		Host: "api.example", KeyID: "91", GroupID: "7", Secret: "secret",
	}); err != nil {
		t.Fatal(err)
	}
	deleted, err := store.DeleteAuthRecord(ctx, "https://API.EXAMPLE/")
	if err != nil || !deleted {
		t.Fatalf("deleted=%v err=%v", deleted, err)
	}
	key, err := store.UpstreamKeySecret(ctx, "api.example", "91", "7")
	if err != nil || key != nil {
		t.Fatalf("cached key survived auth deletion: key=%#v err=%v", key, err)
	}
}
