package configstore

import (
	"context"
	"database/sql"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestLegacyPasswordHashRemainsAuthenticatable(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	const encoded = "pbkdf2_sha256$310000$MDEyMzQ1Njc4OWFiY2RlZg==$G33eWH4HzmUuCbDixK08x1_nRbrOAEEKU7d108gnqpg="
	if _, err := store.db.ExecContext(
		ctx,
		`INSERT INTO settings(key,value) VALUES('console.username','operator'),('console.password_hash',?)`,
		encoded,
	); err != nil {
		t.Fatal(err)
	}

	authenticated, err := store.Authenticate(ctx, "operator", "correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if !authenticated {
		t.Fatal("Go must verify the existing password envelope")
	}
	authenticated, err = store.Authenticate(ctx, "operator", "wrong password")
	if err != nil {
		t.Fatal(err)
	}
	if authenticated {
		t.Fatal("wrong password must not authenticate")
	}
}

func TestInitializePersistsCompatibleStatusAndRejectsOverwrite(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	if err := store.Initialize(ctx, "admin", "a secure password", "https://sub2api.example", "admin-key"); err != nil {
		t.Fatal(err)
	}
	status, err := store.PublicStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !status.Initialized || !status.TargetConfigured || status.Username == nil || *status.Username != "admin" {
		t.Fatalf("unexpected status: %#v", status)
	}
	if err := store.Initialize(ctx, "other", "another secure password", "https://sub2api.example", "key"); err == nil {
		t.Fatal("reinitialization must be rejected")
	}
}

func TestInitializePreservesPasswordWhitespace(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	password := "  password with spaces  "
	if err := store.Initialize(ctx, "admin", password, "https://sub2api.example", "admin-key"); err != nil {
		t.Fatal(err)
	}
	authenticated, err := store.Authenticate(ctx, "admin", password)
	if err != nil || !authenticated {
		t.Fatalf("exact password did not authenticate: authenticated=%t err=%v", authenticated, err)
	}
	authenticated, err = store.Authenticate(ctx, "admin", strings.TrimSpace(password))
	if err != nil || authenticated {
		t.Fatalf("trimmed password unexpectedly authenticated: authenticated=%t err=%v", authenticated, err)
	}
}

func TestSessionUsesOnlyTokenDigestAndSurvivesStoreReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "console-config.sqlite3")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	now := time.Date(2026, 8, 26, 8, 0, 0, 0, time.UTC)
	token, err := store.CreateSession(ctx, "admin", 12*time.Hour, now)
	if err != nil {
		t.Fatal(err)
	}
	var storedToken string
	if err := store.db.QueryRowContext(ctx, `SELECT token_hash FROM console_sessions`).Scan(&storedToken); err != nil {
		t.Fatal(err)
	}
	if storedToken == token {
		t.Fatal("raw session token must not be stored")
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { reopened.Close() })
	username, err := reopened.SessionUser(ctx, token, now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if username == nil || *username != "admin" {
		t.Fatalf("unexpected session user: %#v", username)
	}
	expired, err := reopened.SessionUser(ctx, token, now.Add(13*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if expired != nil {
		t.Fatal("expired session must be rejected")
	}
}

func TestOpenCreatesSessionExpiryIndex(t *testing.T) {
	store := openTestStore(t)
	var count int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name='ix_console_sessions_expires_at'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("session expiry index missing: count=%d err=%v", count, err)
	}
}

func TestOpenPreservesExistingSettingsTable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "console-config.sqlite3")
	database, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO settings(key,value) VALUES('custom.setting','keep-me'),('runtime.probes_enabled','false')`); err != nil {
		t.Fatal(err)
	}
	database.Close()

	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	values, err := store.settings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if values["custom.setting"] != "keep-me" {
		t.Fatalf("existing setting was changed: %#v", values)
	}
	if _, present := values["runtime.probes_enabled"]; present {
		t.Fatalf("deprecated duplicate probe switch was not removed: %#v", values)
	}
}

func TestConfigureTargetPreservesExistingAdminKeyWhenRequestOmitsIt(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	if err := store.Initialize(ctx, "admin", "a secure password", "https://old.example", "existing-key"); err != nil {
		t.Fatal(err)
	}

	if err := store.ConfigureTarget(ctx, "https://new.example/", "", 45); err != nil {
		t.Fatal(err)
	}
	settings, err := store.RuntimeSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if settings.AdminBaseURL == nil || *settings.AdminBaseURL != "https://new.example" {
		t.Fatalf("unexpected target URL: %#v", settings.AdminBaseURL)
	}
	if settings.RequestTimeoutSeconds != 45 {
		t.Fatalf("unexpected timeout: %d", settings.RequestTimeoutSeconds)
	}
	values, err := store.settings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if values["target.admin_key"] != "existing-key" {
		t.Fatal("blank Admin Key must preserve the existing secret")
	}
}

func TestValidateBaseURLRejectsQueryAndFragment(t *testing.T) {
	for _, value := range []string{
		"https://api.example?tenant=one",
		"https://api.example?",
		"https://api.example#section",
		"https://api.example/#",
	} {
		if _, err := ValidateBaseURL(value); err == nil {
			t.Fatalf("base URL %q with query or fragment was accepted", value)
		}
	}
	if value, err := ValidateBaseURL("https://api.example/prefix/"); err != nil || value != "https://api.example/prefix" {
		t.Fatalf("path-prefixed base URL was changed: value=%q err=%v", value, err)
	}
}

func TestAccountDefaultsUseSharedDefaultsAndPersistOverrides(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	defaults, err := store.AccountDefaults(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if defaults.Concurrency != 10 || defaults.Priority != 1 {
		t.Fatalf("defaults=%#v", defaults)
	}
	configured, err := store.ConfigureAccountDefaults(ctx, 24, 7)
	if err != nil {
		t.Fatal(err)
	}
	if configured.Concurrency != 24 || configured.Priority != 7 {
		t.Fatalf("configured=%#v", configured)
	}
	loaded, err := store.AccountDefaults(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if loaded != configured {
		t.Fatalf("loaded=%#v configured=%#v", loaded, configured)
	}
	if _, err := store.ConfigureAccountDefaults(ctx, 0, 1); err == nil {
		t.Fatal("zero concurrency must be rejected")
	}
}

func TestNotificationConfigurationStoresSecretsPrivatelyAndReturnsOnlyPublicStatus(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	if err := store.ConfigureNotifications(ctx, " app ", " secret ", " target ", "group"); err != nil {
		t.Fatal(err)
	}

	status, err := store.NotificationPublicStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !status.Configured || status.AppID != "app" || !status.ClientSecretConfigured || status.HomeChannel != "target" || status.ChannelType != "group" || !status.DestinationConfigured || len(status.ConfigurationErrors) != 0 {
		t.Fatalf("unexpected notification status: %#v", status)
	}
	values, err := store.settings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if values["qqbot.client_secret"] != "secret" {
		t.Fatal("private store did not retain the QQBot secret")
	}
	if reflect.DeepEqual(status, values) {
		t.Fatal("public status must not expose private settings")
	}
	if err := store.ConfigureNotifications(ctx, "updated-app", "", "updated-target", "c2c"); err != nil {
		t.Fatal(err)
	}
	values, err = store.settings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if values["qqbot.client_secret"] != "secret" {
		t.Fatal("blank Client Secret must preserve the existing secret")
	}
}

func TestNotificationConfigurationRejectsInvalidTypeWithoutWriting(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	if err := store.ConfigureNotifications(ctx, "app", "secret", "target", "invalid"); err == nil {
		t.Fatal("invalid channel type must be rejected")
	}
	values, err := store.settings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, found := values["qqbot.app_id"]; found {
		t.Fatal("invalid notification settings must not be partially written")
	}
}

func TestLogCleanupSettingsDefaultToDisabledAndPersistRetention(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "config.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	settings, err := store.LogCleanupSettings(ctx)
	if err != nil || settings.Enabled || settings.RetentionDays != 30 || settings.LastRunAt != nil {
		t.Fatalf("unexpected defaults: %#v err=%v", settings, err)
	}
	settings, err = store.ConfigureLogCleanup(ctx, true, 45)
	if err != nil || !settings.Enabled || settings.RetentionDays != 45 {
		t.Fatalf("unexpected persisted settings: %#v err=%v", settings, err)
	}
	completed := time.Date(2026, 8, 27, 3, 0, 0, 0, time.UTC)
	if err := store.MarkLogCleanupRun(ctx, completed); err != nil {
		t.Fatal(err)
	}
	settings, err = store.LogCleanupSettings(ctx)
	if err != nil || settings.LastRunAt == nil || *settings.LastRunAt != completed.Format(time.RFC3339Nano) {
		t.Fatalf("cleanup timestamp was not persisted: %#v err=%v", settings, err)
	}
	if _, err := store.ConfigureLogCleanup(ctx, true, 0); err == nil {
		t.Fatal("zero retention must be rejected")
	}
}

func TestConcurrentInitializationCannotOverwriteWinningCredentials(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	start := make(chan struct{})
	errorsFound := make(chan error, 2)
	var workers sync.WaitGroup
	for _, username := range []string{"first-admin", "second-admin"} {
		username := username
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			errorsFound <- store.Initialize(ctx, username, "a secure password", "https://sub2api.example", "admin-key")
		}()
	}
	close(start)
	workers.Wait()
	close(errorsFound)
	succeeded, rejected := 0, 0
	for err := range errorsFound {
		if err == nil {
			succeeded++
		} else if strings.Contains(err.Error(), "已经初始化") {
			rejected++
		} else {
			t.Fatalf("unexpected initialization error: %v", err)
		}
	}
	if succeeded != 1 || rejected != 1 {
		t.Fatalf("succeeded=%d rejected=%d", succeeded, rejected)
	}
	status, err := store.PublicStatus(ctx)
	if err != nil || !status.Initialized || status.Username == nil {
		t.Fatalf("unexpected final status: %#v err=%v", status, err)
	}
}

func openTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := Open(filepath.Join(t.TempDir(), "console-config.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}
