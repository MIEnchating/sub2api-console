package business_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/sqliteutil"
)

func TestAccountTestModelsRejectMissingControlPolicyWithoutChangingAccount(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing-policy.db")
	store, err := business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	if err := store.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", sqliteutil.DSN(path, ""))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.ExecContext(ctx, `INSERT INTO accounts(id,name,updated_at) VALUES('41','fixture-account','original'); DELETE FROM policy_nodes WHERE policy_key='control-plane'`); err != nil {
		t.Fatal(err)
	}
	if err := store.SetAccountTestModels(ctx, "41", []string{"fixture-model"}, "operator"); err == nil {
		t.Fatal("missing control policy was accepted")
	}
	var count int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM runtime_events WHERE event_type='account.test_models'`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("rejected update recorded success: count=%d, %v", count, err)
	}
}
