package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/config"
	"github.com/MIEnchating/sub2api-console/backend/internal/dbconn"
)

func TestDatabaseConfigurationDefaultsToPostgresAndRejectsMissingConnection(t *testing.T) {
	t.Setenv("SUB2API_CONSOLE_DB_DRIVER", "postgres")
	if err := os.Unsetenv("SUB2API_CONSOLE_DB_DRIVER"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SUB2API_CONSOLE_DB_DSN", "")
	if _, err := config.Load(); err == nil || !strings.Contains(err.Error(), "SUB2API_CONSOLE_DB_DSN") {
		t.Fatalf("missing connection accepted: %v", err)
	}
	t.Setenv("SUB2API_CONSOLE_DB_DSN", "postgres://user:secret@invalid.test/console")
	loaded, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Database.Driver != dbconn.Postgres || loaded.DatabaseFor(loaded.ConfigDB).Path != "" {
		t.Fatal("default database was not PostgreSQL")
	}
}

func TestExplicitSQLiteKeepsThreeSeparateFilesAndRejectsNetworkDSN(t *testing.T) {
	t.Setenv("SUB2API_CONSOLE_DB_DRIVER", "sqlite")
	t.Setenv("SUB2API_CONSOLE_DB_DSN", "")
	t.Setenv("SUB2API_CONSOLE_DATA_DIR", t.TempDir())
	loaded, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.DatabaseFor(loaded.ConfigDB).Path != loaded.ConfigDB || loaded.ConfigDB == loaded.TaskDB || loaded.ConfigDB == loaded.DataDB {
		t.Fatal("SQLite file configuration changed")
	}
	t.Setenv("SUB2API_CONSOLE_DB_DSN", "secret")
	if _, err := config.Load(); err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatalf("invalid DSN accepted or leaked: %v", err)
	}
}

func TestServerDatabaseRejectsMixedPathsAndInvalidPoolSize(t *testing.T) {
	t.Setenv("SUB2API_CONSOLE_DB_DRIVER", "mysql")
	t.Setenv("SUB2API_CONSOLE_DB_DSN", "user:secret@tcp(invalid.test:3306)/console")
	t.Run("mixed paths", func(t *testing.T) {
		t.Setenv("SUB2API_CONSOLE_CONFIG_DB", filepath.Join(t.TempDir(), "config.db"))
		if _, err := config.Load(); err == nil {
			t.Fatal("mixed paths accepted")
		}
	})
	t.Run("pool", func(t *testing.T) {
		t.Setenv("SUB2API_CONSOLE_DB_MAX_OPEN_CONNS", "2")
		t.Setenv("SUB2API_CONSOLE_DB_MAX_IDLE_CONNS", "3")
		if _, err := config.Load(); err == nil {
			t.Fatal("invalid pool accepted")
		}
	})
}

func TestLegacyDatabaseDetectionRecognizesExistingData(t *testing.T) {
	root := t.TempDir()
	cfg := config.Config{ConfigDB: filepath.Join(root, "config.db"), DataDB: filepath.Join(root, "business.db"), TaskDB: filepath.Join(root, "tasks.db")}
	if found, err := cfg.HasLegacyDatabase(); err != nil || found {
		t.Fatalf("empty directory: %v %v", found, err)
	}
	if err := os.WriteFile(cfg.DataDB, []byte("existing database"), 0600); err != nil {
		t.Fatal(err)
	}
	if found, err := cfg.HasLegacyDatabase(); err != nil || !found {
		t.Fatalf("existing data ignored: %v %v", found, err)
	}
}
