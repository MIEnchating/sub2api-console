package configstore_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/sqliteutil"
)

func TestCorruptPasswordDigestRejectsAuthenticationAndSessionCreation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "authentication.db")
	store, err := configstore.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	db, err := sql.Open("sqlite", sqliteutil.DSN(path, ""))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`INSERT INTO settings(key,value) VALUES('console.username','operator'),('console.password_hash','pbkdf2_sha256$310000$MDEyMzQ1Njc4OWFiY2RlZg==$')`); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	authenticated, err := store.Authenticate(ctx, "operator", "arbitrary password")
	if err != nil || authenticated {
		t.Errorf("empty digest authenticated an arbitrary password: %t, %v", authenticated, err)
	}
	if token, err := store.CreateAuthenticatedSession(ctx, "operator", "arbitrary password", time.Hour, time.Now()); !errors.Is(err, configstore.ErrInvalidCredentials) || token != "" {
		t.Errorf("empty digest created a session: issued=%t, error=%v", token != "", err)
	}
}
