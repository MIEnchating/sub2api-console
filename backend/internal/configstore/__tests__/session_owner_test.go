package configstore_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func TestSessionOwnerRequiresLiveHashedSessionAndRevocationNotifiesWaiters(t *testing.T) {
	store, err := configstore.Open(filepath.Join(t.TempDir(), "config.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	token, err := store.CreateSession(ctx, "admin", time.Hour, now)
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256([]byte(token))
	owner := hex.EncodeToString(hash[:])
	if active, err := store.ActiveSessionOwner(ctx, owner, now); err != nil || !active {
		t.Fatal("live hashed session unavailable", err)
	}
	if active, err := store.ActiveSessionOwner(ctx, token, now); err != nil || active {
		t.Fatal("raw token accepted as owner", err)
	}
	if active, err := store.ActiveSessionOwner(ctx, owner, now.Add(time.Hour)); err != nil || active {
		t.Fatal("expired session accepted", err)
	}
	changed := store.SessionOwnerChanges()
	if err := store.RevokeSession(ctx, token); err != nil {
		t.Fatal(err)
	}
	select {
	case <-changed:
	default:
		t.Fatal("session revocation did not notify waiting maintenance")
	}
	if active, err := store.ActiveSessionOwner(ctx, owner, now); err != nil || active {
		t.Fatal("revoked session accepted", err)
	}
}
