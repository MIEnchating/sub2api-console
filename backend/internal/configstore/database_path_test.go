package configstore

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOpenPreservesLiteralDatabaseFilename(t *testing.T) {
	for _, name := range []string{"console?mode=memory.sqlite3", "console#private.sqlite3", "console%2Fprivate.sqlite3", "console 数据.sqlite3"} {
		t.Run(name, func(t *testing.T) {
			directory := t.TempDir()
			path := filepath.Join(directory, name)
			store, err := Open(path)
			if err != nil {
				t.Fatalf("open literal filename: %v", err)
			}
			t.Cleanup(func() { _ = store.Close() })
			info, err := os.Stat(path)
			if err != nil {
				t.Fatalf("configured database file was not created: %v", err)
			}
			if info.Mode().Perm() != 0o600 {
				t.Fatalf("database permissions = %o, want 600", info.Mode().Perm())
			}
			entries, err := os.ReadDir(directory)
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range entries {
				if entry.Name() != name && entry.Name() != name+"-wal" && entry.Name() != name+"-shm" {
					t.Errorf("unexpected database artifact: %s", entry.Name())
				}
			}
		})
	}
}
