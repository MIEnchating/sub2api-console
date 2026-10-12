package dbconn_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/dbconn"
)

func TestMissingDriverSelectsPostgresAndRequiresExplicitConnection(t *testing.T) {
	_, err := dbconn.Open(t.Context(), dbconn.Config{})
	if err == nil || !strings.Contains(err.Error(), "PostgreSQL") {
		t.Fatalf("expected PostgreSQL configuration error, got %v", err)
	}
}

func TestInvalidConfigurationFailsWithoutExposingCredentials(t *testing.T) {
	for _, tc := range []struct {
		name   string
		config dbconn.Config
	}{
		{"unsupported driver", dbconn.Config{Driver: "password-secret"}},
		{"invalid postgres URL", dbconn.Config{Driver: dbconn.Postgres, DSN: "postgres://user:password-secret@localhost/%zz"}},
		{"invalid mysql DSN", dbconn.Config{Driver: dbconn.MySQL, DSN: "password-secret"}},
		{"sqlite path required", dbconn.Config{Driver: dbconn.SQLite}},
		{"sqlite rejects network DSN", dbconn.Config{Driver: dbconn.SQLite, DSN: "postgres://user:password-secret@localhost/db"}},
		{"negative pool size", dbconn.Config{Driver: dbconn.Postgres, MaxOpenConns: -1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, err := dbconn.Open(t.Context(), tc.config)
			if db != nil {
				db.Close()
				t.Fatal("invalid configuration opened a database")
			}
			if err == nil {
				t.Fatal("invalid configuration accepted")
			}
			if strings.Contains(err.Error(), "password-secret") {
				t.Fatal("configuration error exposed credentials")
			}
		})
	}
}

func TestCancelledOpenDoesNotCreateSQLiteFile(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	path := filepath.Join(t.TempDir(), "not-created", "db.sqlite3")
	_, err := dbconn.Open(ctx, dbconn.Config{Driver: dbconn.SQLite, Path: path})
	if err != context.Canceled {
		t.Fatalf("expected cancellation, got %v", err)
	}
	if _, err := os.Stat(filepath.Dir(path)); !os.IsNotExist(err) {
		t.Fatalf("cancelled open created directory: %v", err)
	}
}
