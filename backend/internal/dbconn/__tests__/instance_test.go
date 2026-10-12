package dbconn_test

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/dbconn"
)

func TestServerInstanceConnectionLossCancelsOwnerContext(t *testing.T) {
	for _, config := range databaseConfigs(t) {
		if config.Driver == dbconn.SQLite {
			continue
		}
		t.Run(string(config.Driver), func(t *testing.T) {
			db, err := dbconn.Open(t.Context(), config)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			instance, err := db.AcquireInstance(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			defer instance.Close()
			// Fixed, opt-in test endpoints only; terminate the connection that
			// owns this test's instance lock, never an arbitrary server session.
			if config.Driver == dbconn.Postgres {
				var terminated bool
				if err := db.QueryRowContext(t.Context(), `SELECT pg_terminate_backend(pid) FROM pg_locks
				 WHERE locktype='advisory' AND classid=0 AND objid=719882047 AND objsubid=1
				 AND database=(SELECT oid FROM pg_database WHERE datname=current_database())`).Scan(&terminated); err != nil || !terminated {
					t.Fatalf("terminate instance connection: %v %v", terminated, err)
				}
			} else {
				var connectionID int64
				if err := db.QueryRowContext(t.Context(), `SELECT IS_USED_LOCK(CONCAT('console_',LEFT(SHA2(DATABASE(),256),56)))`).Scan(&connectionID); err != nil {
					t.Fatal(err)
				}
				if _, err := db.ExecContext(t.Context(), `KILL CONNECTION `+strconv.FormatInt(connectionID, 10)); err != nil {
					t.Fatal(err)
				}
			}
			deadline, cancel := context.WithTimeout(t.Context(), 15*time.Second)
			defer cancel()
			select {
			case <-instance.Context().Done():
				if context.Cause(instance.Context()) == context.Canceled {
					t.Fatal("connection loss was reported as a manual cancellation")
				}
			case <-deadline.Done():
				t.Fatal("owner context remained active after losing the instance lock")
			}
		})
	}
}

func TestServerInstanceLockLeavesSingleConnectionPoolAvailable(t *testing.T) {
	for _, config := range databaseConfigs(t) {
		if config.Driver == dbconn.SQLite {
			continue
		}
		t.Run(string(config.Driver), func(t *testing.T) {
			config.MaxOpenConns, config.MaxIdleConns = 1, 1
			db, err := dbconn.Open(t.Context(), config)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			instance, err := db.AcquireInstance(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			defer instance.Close()
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			if err := db.CheckImport(ctx); err != nil {
				t.Fatalf("instance lock starved startup checks with a single connection: %v", err)
			}
		})
	}
}

func TestServerInstanceLockExcludesSecondOwnerAndReleasesOnClose(t *testing.T) {
	for _, config := range databaseConfigs(t) {
		if config.Driver == dbconn.SQLite {
			continue
		}
		t.Run(string(config.Driver), func(t *testing.T) {
			db, err := dbconn.Open(t.Context(), config)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			first, err := db.AcquireInstance(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			defer first.Close()
			if second, err := db.AcquireInstance(t.Context()); err == nil {
				second.Close()
				t.Fatal("second instance accepted")
			}
			if err := first.Close(); err != nil {
				t.Fatal(err)
			}
			second, err := db.AcquireInstance(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			if err := second.Close(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestServerIncompleteImportBlocksSchemaInitialization(t *testing.T) {
	for _, config := range databaseConfigs(t) {
		if config.Driver == dbconn.SQLite {
			continue
		}
		t.Run(string(config.Driver), func(t *testing.T) {
			db, err := dbconn.Open(t.Context(), config)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if err := db.BeginImport(t.Context()); err != nil {
				t.Fatal(err)
			}
			defer func() {
				if _, err := db.ExecContext(t.Context(), `DROP TABLE console_data_import`); err != nil {
					t.Error(err)
				}
			}()
			if err := db.CheckImport(t.Context()); err == nil {
				t.Fatal("pending import accepted")
			}
			if err := db.CheckImport(dbconn.ImportContext(t.Context())); err != nil {
				t.Fatal(err)
			}
			if err := db.BeginImport(t.Context()); err == nil {
				t.Fatal("import target reused")
			}
			if _, err := db.ExecContext(t.Context(), `UPDATE console_data_import SET state='complete' WHERE id=1`); err != nil {
				t.Fatal(err)
			}
			if err := db.CheckImport(t.Context()); err != nil {
				t.Fatal(err)
			}
		})
	}
}
