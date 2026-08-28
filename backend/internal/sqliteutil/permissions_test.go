package sqliteutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPrepareAndSecureRestrictDatabaseAndSidecarPermissions(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "data")
	path := filepath.Join(directory, "console.sqlite3")
	if err := Prepare(path); err != nil {
		t.Fatal(err)
	}
	for _, candidate := range []string{path, path + "-wal", path + "-shm"} {
		if err := os.WriteFile(candidate, []byte("test"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := Secure(path); err != nil {
		t.Fatal(err)
	}
	for _, candidate := range []string{directory, path, path + "-wal", path + "-shm"} {
		info, err := os.Stat(candidate)
		if err != nil {
			t.Fatal(err)
		}
		expected := os.FileMode(0o600)
		if info.IsDir() {
			expected = 0o700
		}
		if info.Mode().Perm() != expected {
			t.Fatalf("%s mode=%#o expected=%#o", candidate, info.Mode().Perm(), expected)
		}
	}
}
