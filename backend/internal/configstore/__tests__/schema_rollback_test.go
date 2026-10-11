package configstore_test

import (
	"database/sql"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func TestOpenWithIncompatibleSessionSchemaRollsBackAddedTablesAndPreservesOldData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old-private.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	// A later index creation must reject the incompatible existing table.
	if _, err := db.Exec(`CREATE TABLE settings(key TEXT PRIMARY KEY, value TEXT NOT NULL);
		INSERT INTO settings VALUES('existing-setting','preserved');
		CREATE TABLE console_sessions(token_hash TEXT PRIMARY KEY);
		INSERT INTO console_sessions VALUES('existing-session')`); err != nil {
		t.Fatal(err)
	}

	store, err := configstore.Open(path)
	if store != nil {
		_ = store.Close()
	}
	if err == nil {
		t.Fatal("incompatible existing session schema was accepted")
	}
	rows, err := db.Query(`SELECT name FROM sqlite_schema WHERE type='table' ORDER BY name`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(names, []string{"console_sessions", "settings"}) {
		t.Errorf("failed schema upgrade left new tables behind: %v", names)
	}
	var setting, session string
	if err := db.QueryRow(`SELECT value FROM settings WHERE key='existing-setting'`).Scan(&setting); err != nil || setting != "preserved" {
		t.Fatalf("existing setting changed: %q %v", setting, err)
	}
	if err := db.QueryRow(`SELECT token_hash FROM console_sessions`).Scan(&session); err != nil || session != "existing-session" {
		t.Fatalf("existing session changed: %q %v", session, err)
	}
}
