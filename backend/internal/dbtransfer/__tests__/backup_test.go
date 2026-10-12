package dbtransfer_test

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/dbtransfer"
	"github.com/MIEnchating/sub2api-console/backend/internal/sqliteutil"
)

func TestSnapshotCopiesCommittedWALAndKeepsSourceWritesBlocked(t *testing.T) {
	root := t.TempDir()
	paths := []string{}
	pools := []*sql.DB{}
	for _, name := range []string{"config", "business", "tasks"} {
		path := filepath.Join(root, name+".sqlite3")
		paths = append(paths, path)
		db, err := sql.Open("sqlite", sqliteutil.DSN(path, "_pragma=journal_mode(WAL)&_pragma=busy_timeout(1)"))
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		pools = append(pools, db)
		if _, err := db.Exec(`CREATE TABLE fixture(id INTEGER PRIMARY KEY,value TEXT);INSERT INTO fixture VALUES(1,'WAL内容')`); err != nil {
			t.Fatal(err)
		}
	}
	snapshot, err := dbtransfer.SnapshotSources(t.Context(), dbtransfer.Sources{Config: paths[0], Business: paths[1], Tasks: paths[2]}, filepath.Join(root, "backup"))
	if err != nil {
		t.Fatal(err)
	}
	defer snapshot.Close()
	for _, path := range []string{snapshot.Original.Config, snapshot.Original.Business, snapshot.Original.Tasks, snapshot.Prepared.Config, snapshot.Prepared.Business, snapshot.Prepared.Tasks} {
		db, err := sql.Open("sqlite", sqliteutil.DSN(path, "mode=ro"))
		if err != nil {
			t.Fatal(err)
		}
		var value string
		err = db.QueryRow(`SELECT value FROM fixture WHERE id=1`).Scan(&value)
		db.Close()
		if err != nil || value != "WAL内容" {
			t.Fatalf("backup omitted committed data: %q %v", value, err)
		}
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != 0600 {
			t.Fatalf("backup permissions: %v %v", info, err)
		}
	}
	for _, db := range pools {
		if _, err := db.Exec(`INSERT INTO fixture VALUES(2,'blocked')`); err == nil {
			t.Fatal("source write was allowed")
		}
	}
	if err := snapshot.Close(); err != nil {
		t.Fatal(err)
	}
	for _, db := range pools {
		if _, err := db.Exec(`INSERT INTO fixture VALUES(2,'released')`); err != nil {
			t.Fatal(err)
		}
	}
}
