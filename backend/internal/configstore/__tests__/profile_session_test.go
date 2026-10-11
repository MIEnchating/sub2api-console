package configstore_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func profileStore(t *testing.T) (*configstore.Store, string, string, time.Time) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "profile.sqlite3")
	store, err := configstore.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	if err := store.Initialize(ctx, "operator", "current-password", "https://isolated.example", "fixture-key"); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	token, err := store.CreateAuthenticatedSession(ctx, "operator", "current-password", time.Hour, now)
	if err != nil {
		t.Fatal(err)
	}
	return store, token, path, now
}

func TestProfileRenamePreservesCurrentSessionExpiry(t *testing.T) {
	store, token, _, now := profileStore(t)
	ctx := context.Background()
	name, saved, err := store.UpdateSessionCredentials(ctx, token, "current-password", " renamed ", nil, 12*time.Hour, now.Add(time.Minute))
	if err != nil || name != "renamed" || saved != token {
		t.Fatalf("rename failed: %s %v", name, err)
	}
	user, err := store.SessionUser(ctx, token, now.Add(59*time.Minute))
	if err != nil || user == nil || *user != name {
		t.Fatalf("session unavailable: %v", err)
	}
	user, err = store.SessionUser(ctx, token, now.Add(time.Hour))
	if err != nil || user != nil {
		t.Fatalf("session expiry extended: %v", err)
	}
}

func TestProfileFailedRevocationRollsBackCredentialsAndSession(t *testing.T) {
	store, token, path, now := profileStore(t)
	ctx := context.Background()
	other, err := store.CreateSession(ctx, "operator", time.Hour, now)
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TRIGGER reject_session_delete BEFORE DELETE ON console_sessions BEGIN SELECT RAISE(ABORT, 'isolated failure'); END`); err != nil {
		t.Fatal(err)
	}
	replacement := "replacement-password"
	_, _, err = store.UpdateSessionCredentials(ctx, token, "current-password", "renamed", &replacement, time.Hour, now)
	if err == nil {
		t.Fatal("expected failed revocation")
	}
	if ok, err := store.Authenticate(ctx, "operator", "current-password"); err != nil || !ok {
		t.Fatalf("credentials changed despite rollback: %v", err)
	}
	for _, session := range []string{token, other} {
		user, err := store.SessionUser(ctx, session, now)
		if err != nil || user == nil || *user != "operator" {
			t.Fatalf("session changed despite rollback: %v", err)
		}
	}
}

func TestProfileRejectsExpiredRevokedOrIncorrectCredentials(t *testing.T) {
	for _, scenario := range []string{"expired", "revoked", "wrong-password", "same-password"} {
		t.Run(scenario, func(t *testing.T) {
			store, token, _, now := profileStore(t)
			ctx := context.Background()
			password := "current-password"
			var replacement *string
			switch scenario {
			case "expired":
				now = now.Add(time.Hour)
			case "revoked":
				if err := store.RevokeSession(ctx, token); err != nil {
					t.Fatal(err)
				}
			case "wrong-password":
				password = "wrong-password"
			case "same-password":
				replacement = &password
			}
			_, _, err := store.UpdateSessionCredentials(ctx, token, password, "renamed", replacement, time.Hour, now)
			if err == nil {
				t.Fatal("invalid update accepted")
			}
			if ok, err := store.Authenticate(ctx, "operator", "current-password"); err != nil || !ok {
				t.Fatalf("rejected update changed credentials: %v", err)
			}
		})
	}
}

func TestProfileBearerUpdateCreatesOnlyOneNewSession(t *testing.T) {
	store, old, _, now := profileStore(t)
	ctx := context.Background()
	_, token, err := store.UpdateSessionCredentials(ctx, "", "current-password", "renamed", nil, time.Hour, now)
	if err != nil || token == "" || token == old {
		t.Fatalf("new session missing: %v", err)
	}
	user, err := store.SessionUser(ctx, token, now)
	if err != nil || user == nil || *user != "renamed" {
		t.Fatalf("new session unavailable: %v", err)
	}
	user, err = store.SessionUser(ctx, old, now)
	if err != nil || user != nil {
		t.Fatalf("old session retained: %v", err)
	}
}
