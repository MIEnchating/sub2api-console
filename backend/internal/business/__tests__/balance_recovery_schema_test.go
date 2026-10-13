package business_test

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

func TestExistingDatabaseAddsBalanceRecoveryStateWithoutChangingAccounts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "upgrade.db")
	store, err := business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Bootstrap(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`DROP TABLE balance_recovery_states; INSERT INTO accounts(id,name,schedulable,metadata_json,updated_at) VALUES('41','preserved',0,'{"status":"error"}','original')`); err != nil {
		t.Fatal(err)
	}
	store, err = business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	state := business.BalanceRecoveryState{AccountID: "41", TargetFingerprint: strings.Repeat("a", 64), BindingFingerprint: strings.Repeat("b", 64), Schedulable: false, Depleted: true}
	if err := store.SaveBalanceRecoveryState(t.Context(), state); err != nil {
		t.Fatal(err)
	}
	var name, metadata, updated string
	var schedulable int
	if err := db.QueryRow(`SELECT name,schedulable,metadata_json,updated_at FROM accounts WHERE id='41'`).Scan(&name, &schedulable, &metadata, &updated); err != nil {
		t.Fatal(err)
	}
	if name != "preserved" || schedulable != 0 || metadata != `{"status":"error"}` || updated != "original" {
		t.Fatal("schema upgrade altered existing account")
	}
}
