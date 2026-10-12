package dbtransfer_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/dbconn"
	"github.com/MIEnchating/sub2api-console/backend/internal/dbtest"
	"github.com/MIEnchating/sub2api-console/backend/internal/dbtransfer"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

// This destructive test helper accepts only the fixed opt-in dbtest endpoints.
// It never consumes the application DSN or any caller-supplied endpoint.
func emptyContractTarget(t *testing.T, config dbconn.Config) {
	t.Helper()
	db, err := dbconn.Open(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	query := `SELECT table_name FROM information_schema.tables WHERE table_schema=current_schema() AND table_type='BASE TABLE'`
	if config.Driver == dbconn.MySQL {
		query = `SELECT table_name FROM information_schema.tables WHERE table_schema=DATABASE() AND table_type='BASE TABLE'`
	}
	rows, err := db.Query(query)
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		names = append(names, name)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		t.Fatal(err)
	}
	conn, err := db.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if config.Driver == dbconn.MySQL {
		if _, err := conn.ExecContext(context.Background(), `SET FOREIGN_KEY_CHECKS=0`); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range names {
		query := `DROP TABLE "` + strings.ReplaceAll(name, `"`, `""`) + `" CASCADE`
		if config.Driver == dbconn.MySQL {
			query = "DROP TABLE `" + strings.ReplaceAll(name, "`", "``") + "`"
		}
		if _, err := conn.ExecContext(context.Background(), query); err != nil {
			t.Fatal(err)
		}
	}
	if config.Driver == dbconn.MySQL {
		if _, err := conn.ExecContext(context.Background(), `SET FOREIGN_KEY_CHECKS=1`); err != nil {
			t.Fatal(err)
		}
	}
	// PostgreSQL functions outlive tables and belong to this isolated test schema.
	if config.Driver == dbconn.Postgres {
		if _, err := conn.ExecContext(context.Background(), `DROP SCHEMA public CASCADE`); err != nil {
			t.Fatal(err)
		}
		if _, err := conn.ExecContext(context.Background(), `CREATE SCHEMA public`); err != nil {
			t.Fatal(err)
		}
	} else {
		if _, err := conn.ExecContext(context.Background(), `DROP FUNCTION IF EXISTS console_time_value_v1`); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDatabaseTransferPreservesDataAndRebuildsServerIndexes(t *testing.T) {
	for _, config := range dbtest.Configs(t) {
		if config.Driver == dbconn.SQLite {
			continue
		}
		t.Run(string(config.Driver), func(t *testing.T) {
			emptyContractTarget(t, config)
			defer emptyContractTarget(t, config)
			root := t.TempDir()
			sources := dbtransfer.Sources{Config: filepath.Join(root, "config.sqlite3"), Business: filepath.Join(root, "business.sqlite3"), Tasks: filepath.Join(root, "tasks.sqlite3")}
			private, err := configstore.Open(sources.Config)
			if err != nil {
				t.Fatal(err)
			}
			if err := private.SaveJSONSetting(t.Context(), "migration.fixture", `{"exact":9007199254740993,"text":"中文"}`); err != nil {
				t.Fatal(err)
			}
			if err := private.Close(); err != nil {
				t.Fatal(err)
			}
			data, err := business.Open(sources.Business)
			if err != nil {
				t.Fatal(err)
			}
			if err := data.Bootstrap(t.Context()); err != nil {
				t.Fatal(err)
			}
			if _, err := data.SyncManagementSnapshot(t.Context(), []map[string]any{{"id": "41", "name": "迁移账号", "rate_multiplier": "0.1234567890123456789", "group_ids": []any{"91"}}}, []map[string]any{{"id": "91", "name": "迁移分组", "platform": "openai"}}, "contract"); err != nil {
				t.Fatal(err)
			}
			stamp := time.Date(2026, 10, 12, 12, 0, 0, 123456789, time.UTC)
			if _, err := data.PersistTrafficSamples(t.Context(), []business.TrafficSample{{AccountID: "41", GroupName: "迁移分组", EvidenceKey: "request", Result: "通过", ObservedAt: stamp.Format(time.RFC3339Nano), Payload: map[string]any{"input_tokens": json.Number("9007199254740993")}}}); err != nil {
				t.Fatal(err)
			}
			sourceDB, err := dbconn.Open(t.Context(), dbconn.Config{Driver: dbconn.SQLite, Path: sources.Business})
			if err != nil {
				t.Fatal(err)
			}
			for _, query := range []string{`UPDATE usage_records SET id=0 WHERE request_id='request'`, `UPDATE health_samples SET id=70000 WHERE evidence_key='request'`, `UPDATE runtime_events SET source_id=-12345 WHERE source_id=(SELECT MIN(source_id) FROM runtime_events)`} {
				if _, err := sourceDB.ExecContext(t.Context(), query); err != nil {
					sourceDB.Close()
					t.Fatal(err)
				}
			}
			if err := sourceDB.Close(); err != nil {
				t.Fatal(err)
			}
			if err := data.Close(); err != nil {
				t.Fatal(err)
			}
			tasks, err := taskstore.Open(sources.Tasks)
			if err != nil {
				t.Fatal(err)
			}
			task := taskstore.Task{ID: "animation", Skill: "sub2api-model-animation", Operation: "account-model-animation", Status: "succeeded", Progress: 100, Message: "完成", CreatedAt: stamp.Format(time.RFC3339Nano), UpdatedAt: stamp.Format(time.RFC3339Nano), Result: map[string]any{"account_ids": []string{"41"}, "animations": []any{map[string]any{"account_id": "41", "status": "succeeded", "completed_at": stamp.Format(time.RFC3339Nano)}}}}
			if err := tasks.Save(t.Context(), task); err != nil {
				t.Fatal(err)
			}
			if err := tasks.Close(); err != nil {
				t.Fatal(err)
			}
			report, err := dbtransfer.Transfer(t.Context(), dbtransfer.Options{Sources: sources, BackupDirectory: filepath.Join(root, "backup"), Target: config, Stopped: true})
			if err != nil {
				t.Fatal(err)
			}
			if report.State != "complete" || len(report.Tables) < 20 {
				t.Fatalf("incomplete report: %+v", report)
			}
			for _, entry := range report.Tables {
				if len(entry.SHA256) != 64 {
					t.Fatalf("missing content digest: %+v", entry)
				}
			}
			private, err = configstore.OpenDatabase(t.Context(), config)
			if err != nil {
				t.Fatal(err)
			}
			defer private.Close()
			value, err := private.JSONSetting(t.Context(), "migration.fixture")
			if err != nil || value != `{"exact":9007199254740993,"text":"中文"}` {
				t.Fatalf("configuration changed: %q %v", value, err)
			}
			data, err = business.OpenDatabase(t.Context(), config)
			if err != nil {
				t.Fatal(err)
			}
			defer data.Close()
			traffic, err := data.RoutingRecoveryTraffic(t.Context(), "41", stamp.Add(-time.Second), stamp.Add(time.Second))
			if err != nil || len(traffic) != 1 {
				t.Fatalf("recovery index not rebuilt: %+v %v", traffic, err)
			}
			if id, err := data.RecordRuntimeEvent(t.Context(), "contract.import", "succeeded", "导入后写入", map[string]any{}); err != nil || id >= -12345 {
				t.Fatalf("event counter not aligned: %d %v", id, err)
			}
			if _, err := data.PersistTrafficSamples(t.Context(), []business.TrafficSample{{AccountID: "41", GroupName: "迁移分组", EvidenceKey: "after-import", Result: "通过", ObservedAt: stamp.Add(time.Second).Format(time.RFC3339Nano), Payload: map[string]any{}}}); err != nil {
				t.Fatal(err)
			}
			targetDB, err := dbconn.Open(t.Context(), config)
			if err != nil {
				t.Fatal(err)
			}
			var nextID int64
			if err := targetDB.QueryRowContext(t.Context(), `SELECT id FROM health_samples WHERE evidence_key='after-import'`).Scan(&nextID); err != nil || nextID <= 70000 {
				targetDB.Close()
				t.Fatalf("identity sequence not aligned: %d %v", nextID, err)
			}
			var zero int
			if err := targetDB.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM usage_records WHERE id=0 AND request_id='request'`).Scan(&zero); err != nil || zero != 1 {
				targetDB.Close()
				t.Fatalf("zero ID not preserved: %d %v", zero, err)
			}
			if err := targetDB.Close(); err != nil {
				t.Fatal(err)
			}
			tasks, err = taskstore.OpenDatabase(t.Context(), config)
			if err != nil {
				t.Fatal(err)
			}
			defer tasks.Close()
			history, err := tasks.AnimationHistory(t.Context())
			if err != nil || len(history) != 1 || history[0].ID != "animation" {
				t.Fatalf("history projection not rebuilt: %+v %v", history, err)
			}
			if _, err := dbtransfer.Transfer(t.Context(), dbtransfer.Options{Sources: sources, BackupDirectory: filepath.Join(root, "retry-backup"), Target: config, Stopped: true}); err == nil {
				t.Fatal("populated target overwritten")
			}
		})
	}
}
