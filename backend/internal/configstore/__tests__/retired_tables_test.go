package configstore_test

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func TestReopenRemovesEmptyRetiredWorkbenchTablesButPreservesPopulatedOnes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "retired.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, table := range []string{"workbench_login_profiles", "workbench_oauth_checkpoints", "workbench_queues", "workbench_sms_receipts", "workbench_source_profiles"} {
		if _, err := db.Exec("CREATE TABLE " + table + "(id TEXT PRIMARY KEY)"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT INTO workbench_login_profiles VALUES('preserved'); CREATE TABLE unrelated(id TEXT)`); err != nil {
		t.Fatal(err)
	}
	store, err := configstore.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM sqlite_schema WHERE name IN ('workbench_oauth_checkpoints','workbench_queues','workbench_sms_receipts','workbench_source_profiles')`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("%d empty retired tables survived", count)
	}
	var id string
	if err := db.QueryRow(`SELECT id FROM workbench_login_profiles`).Scan(&id); err != nil || id != "preserved" {
		t.Fatalf("legacy contents changed: %q %v", id, err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM sqlite_schema WHERE name='unrelated'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("unrelated table changed: %d %v", count, err)
	}
}
