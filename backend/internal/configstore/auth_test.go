package configstore

import (
	"context"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
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

func TestRenameAuthRecordMovesRecoverySecretsAndVaultHost(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "rename-auth.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	host := "speed.ai-pixel.online"
	token := "token"
	if err := store.SaveAuthRecord(ctx, AuthRecord{Host: host, BaseURL: "https://ai-pixel.online", UpstreamType: "sub2api", AuthMode: "sub2api_user_token", AccessToken: &token}, nil); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveVaultEntry(ctx, VaultEntry{Entry: "pixel", Hosts: []string{host}}, map[string]bool{"hosts": true}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO auth_recovery_preferences(host,auth_mode,recovery_method,succeeded_at)
		VALUES('speed.ai-pixel.online','sub2api_user_token','manual','now');
		INSERT INTO upstream_key_secrets(host,key_id,group_id,secret,updated_at)
		VALUES('speed.ai-pixel.online','key-1','25','secret','now')`); err != nil {
		t.Fatal(err)
	}
	if err := store.RenameAuthRecord(ctx, host, "ai-pixel.online"); err != nil {
		t.Fatal(err)
	}
	record, err := store.AuthRecord(ctx, "ai-pixel.online")
	if err != nil || record == nil || record.Host != "ai-pixel.online" || record.BaseURL != "https://ai-pixel.online" {
		t.Fatalf("record=%#v err=%v", record, err)
	}
	entry, err := store.VaultEntry(ctx, "pixel")
	if err != nil || entry == nil || len(entry.Hosts) != 1 || entry.Hosts[0] != "ai-pixel.online" {
		t.Fatalf("entry=%#v err=%v", entry, err)
	}
	var preferenceHost, secretHost string
	if err := store.db.QueryRowContext(ctx, `SELECT host FROM auth_recovery_preferences LIMIT 1`).Scan(&preferenceHost); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRowContext(ctx, `SELECT host FROM upstream_key_secrets LIMIT 1`).Scan(&secretHost); err != nil {
		t.Fatal(err)
	}
	if preferenceHost != "ai-pixel.online" || secretHost != "ai-pixel.online" {
		t.Fatalf("preference=%q secret=%q", preferenceHost, secretHost)
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

func TestNormalizedHeadersRejectsInvalidAmbiguousAndOversizedValues(t *testing.T) {
	tests := map[string]map[string]string{
		"invalid field name":   {"Bad Header": "value"},
		"forbidden field name": {"Host": "attacker.example"},
		"control character":    {"X-Test": "value\x00suffix"},
		"duplicate field name": {"X-Test": "one", "x-test": "two"},
		"oversized field value": {
			"X-Test": strings.Repeat("a", maximumCustomHeaderValueBytes+1),
		},
		"oversized total": {
			"X-One": strings.Repeat("a", maximumCustomHeaderValueBytes),
			"X-Two": strings.Repeat("b", maximumCustomHeaderValueBytes),
		},
	}
	many := make(map[string]string, maximumCustomHeaderCount+1)
	for index := 0; index <= maximumCustomHeaderCount; index++ {
		many["X-Test-"+strings.Repeat("a", index)] = "value"
	}
	tests["too many fields"] = many

	for name, headers := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := normalizedHeaders(headers); err == nil {
				t.Fatal("invalid headers were accepted")
			}
		})
	}
}

func TestNormalizedCookiesRejectsInvalidAndOversizedValues(t *testing.T) {
	tests := map[string]map[string]string{
		"invalid cookie name":  {"bad name": "value"},
		"invalid cookie value": {"session": "first; injected=second"},
		"oversized cookie value": {
			"session": strings.Repeat("a", maximumCustomHeaderValueBytes+1),
		},
		"oversized total": {
			"first":  strings.Repeat("a", maximumCustomHeaderValueBytes),
			"second": strings.Repeat("b", maximumCustomHeaderValueBytes),
		},
	}
	many := make(map[string]string, maximumCustomHeaderCount+1)
	for index := 0; index <= maximumCustomHeaderCount; index++ {
		many[fmt.Sprintf("cookie_%d", index)] = "value"
	}
	tests["too many cookies"] = many

	for name, cookies := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := normalizedCookies(cookies); err == nil {
				t.Fatal("invalid cookies were accepted")
			}
		})
	}
}

func TestNormalizedHeadersCanonicalizesValidFieldNames(t *testing.T) {
	headers, err := normalizedHeaders(map[string]string{"x-custom-header": "中文 value\taccepted"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(headers, map[string]string{"X-Custom-Header": "中文 value\taccepted"}) {
		t.Fatalf("headers=%#v", headers)
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
