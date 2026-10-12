package dbconn_test

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/dbconn"
)

func migrationFixture(t *testing.T, config dbconn.Config) *dbconn.Database {
	t.Helper()
	db, err := dbconn.Open(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	t.Cleanup(func() {
		for _, table := range []string{"migration_items", "console_schema_migrations"} {
			if _, err := db.ExecContext(context.Background(), `DROP TABLE IF EXISTS `+table); err != nil {
				t.Error(err)
			}
		}
	})
	return db
}

func migrationsFor(driver dbconn.Driver) []dbconn.Migration {
	return []dbconn.Migration{
		{Version: 1, Statements: map[dbconn.Driver][]string{driver: {`CREATE TABLE migration_items (id INTEGER PRIMARY KEY, value VARCHAR(64) NOT NULL)`, `INSERT INTO migration_items VALUES (1,'original')`}}},
		{Version: 2, Statements: map[dbconn.Driver][]string{driver: {`ALTER TABLE migration_items ADD COLUMN note VARCHAR(64)`}}},
	}
}

func TestMigrationUpgradeAndRepeatedStartupPreserveExistingData(t *testing.T) {
	for _, config := range databaseConfigs(t) {
		t.Run(string(config.Driver), func(t *testing.T) {
			db := migrationFixture(t, config)
			migrations := migrationsFor(config.Driver)
			if err := db.Migrate(t.Context(), "contract", migrations[:1]); err != nil {
				t.Fatal(err)
			}
			if err := db.Migrate(t.Context(), "contract", migrations); err != nil {
				t.Fatal(err)
			}
			if err := db.Migrate(t.Context(), "contract", migrations); err != nil {
				t.Fatal(err)
			}
			var value string
			if err := db.QueryRowContext(t.Context(), `SELECT value FROM migration_items WHERE id=1`).Scan(&value); err != nil || value != "original" {
				t.Fatalf("preserved value: %q %v", value, err)
			}
		})
	}
}

func TestAppliedMigrationChangesAndOlderProgramAreRejected(t *testing.T) {
	for _, config := range databaseConfigs(t) {
		t.Run(string(config.Driver), func(t *testing.T) {
			db := migrationFixture(t, config)
			migrations := migrationsFor(config.Driver)
			if err := db.Migrate(t.Context(), "contract", migrations); err != nil {
				t.Fatal(err)
			}
			if err := db.Migrate(t.Context(), "contract", migrations[:1]); err == nil || !strings.Contains(err.Error(), "禁止降级") {
				t.Fatalf("downgrade: %v", err)
			}
			migrations[0].Statements[config.Driver][1] = `INSERT INTO migration_items VALUES(2,'changed')`
			if err := db.Migrate(t.Context(), "contract", migrations); err == nil || !strings.Contains(err.Error(), "校验和") {
				t.Fatalf("modified migration: %v", err)
			}
		})
	}
}

func TestFailedMigrationRollsBackOrBlocksMySQLRestart(t *testing.T) {
	for _, config := range databaseConfigs(t) {
		t.Run(string(config.Driver), func(t *testing.T) {
			db := migrationFixture(t, config)
			migrations := migrationsFor(config.Driver)
			if err := db.Migrate(t.Context(), "contract", migrations[:1]); err != nil {
				t.Fatal(err)
			}
			migrations[1].Statements[config.Driver] = append(migrations[1].Statements[config.Driver], `INSERT INTO missing_migration_table VALUES(1)`)
			if err := db.Migrate(t.Context(), "contract", migrations); err == nil {
				t.Fatal("invalid DDL accepted")
			}
			var count int
			if err := db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM migration_items WHERE value='original'`).Scan(&count); err != nil || count != 1 {
				t.Fatalf("old data lost: %d %v", count, err)
			}
			clean := migrationsFor(config.Driver)
			err := db.Migrate(t.Context(), "contract", clean)
			if config.Driver == dbconn.MySQL {
				if err == nil || !strings.Contains(err.Error(), "尚未完成") {
					t.Fatalf("dirty MySQL migration: %v", err)
				}
			} else if err != nil {
				t.Fatalf("transactional DDL did not roll back: %v", err)
			}
		})
	}
}

func TestConcurrentMigrationCallsApplyEachVersionOnce(t *testing.T) {
	for _, config := range databaseConfigs(t) {
		t.Run(string(config.Driver), func(t *testing.T) {
			db := migrationFixture(t, config)
			migrations := migrationsFor(config.Driver)
			var workers sync.WaitGroup
			for range 4 {
				workers.Go(func() {
					if err := db.Migrate(t.Context(), "contract", migrations); err != nil {
						t.Error(err)
					}
				})
			}
			workers.Wait()
			var count int
			if err := db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM migration_items`).Scan(&count); err != nil || count != 1 {
				t.Fatalf("migration ran twice: %d %v", count, err)
			}
		})
	}
}
