package business_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/sqliteutil"
)

func TestRoutingAccountMetadataFailuresReleaseConnectionsForLaterReads(t *testing.T) {
	path := filepath.Join(t.TempDir(), "routing-connections.db")
	store, err := business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	db, err := sql.Open("sqlite", sqliteutil.DSN(path, ""))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`INSERT INTO accounts(id,name,metadata_json,updated_at) VALUES('41','damaged-account','{broken','original');
		INSERT INTO account_groups(account_id,group_name) VALUES('41','fixture-group')`); err != nil {
		t.Fatal(err)
	}
	for range 8 {
		if _, err := store.RoutingAccounts(context.Background(), nil, nil); err == nil {
			t.Fatal("damaged account metadata was accepted")
		}
	}
	if _, err := db.Exec(`UPDATE accounts SET metadata_json='{}' WHERE id='41'`); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	accounts, err := store.RoutingAccounts(ctx, nil, nil)
	if err != nil || len(accounts) != 1 || accounts[0].ID != "41" {
		t.Fatalf("repaired account could not be read after metadata failures: accounts=%#v, %v", accounts, err)
	}
}
