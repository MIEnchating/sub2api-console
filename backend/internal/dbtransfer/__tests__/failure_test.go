package dbtransfer_test

import (
	"path/filepath"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/dbconn"
	"github.com/MIEnchating/sub2api-console/backend/internal/dbtest"
	"github.com/MIEnchating/sub2api-console/backend/internal/dbtransfer"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

func TestDatabaseTransferRefusesUnknownColumnsAndLeavesTargetBlocked(t *testing.T) {
	for _, config := range dbtest.Configs(t) {
		if config.Driver == dbconn.SQLite {
			continue
		}
		t.Run(string(config.Driver), func(t *testing.T) {
			emptyContractTarget(t, config)
			defer emptyContractTarget(t, config)
			root := t.TempDir()
			sources := dbtransfer.Sources{Config: filepath.Join(root, "config.db"), Business: filepath.Join(root, "business.db"), Tasks: filepath.Join(root, "tasks.db")}
			private, err := configstore.Open(sources.Config)
			if err != nil {
				t.Fatal(err)
			}
			private.Close()
			data, err := business.Open(sources.Business)
			if err != nil {
				t.Fatal(err)
			}
			data.Close()
			tasks, err := taskstore.Open(sources.Tasks)
			if err != nil {
				t.Fatal(err)
			}
			tasks.Close()
			source, err := dbconn.Open(t.Context(), dbconn.Config{Driver: dbconn.SQLite, Path: sources.Config})
			if err != nil {
				t.Fatal(err)
			}
			defer source.Close()
			if _, err := source.ExecContext(t.Context(), `ALTER TABLE settings ADD COLUMN unrecognized TEXT`); err != nil {
				t.Fatal(err)
			}
			if _, err := source.ExecContext(t.Context(), `INSERT INTO settings("key",value,unrecognized) VALUES('fixture','preserve','legacy-data')`); err != nil {
				t.Fatal(err)
			}
			report, err := dbtransfer.Transfer(t.Context(), dbtransfer.Options{Sources: sources, BackupDirectory: filepath.Join(root, "backup"), Target: config, Stopped: true})
			if err == nil || report.State == "complete" {
				t.Fatal("unknown source column silently dropped")
			}
			if store, err := configstore.OpenDatabase(t.Context(), config); err == nil {
				store.Close()
				t.Fatal("incomplete target allowed service startup")
			}
			var original string
			if err := source.QueryRowContext(t.Context(), `SELECT unrecognized FROM settings WHERE "key"='fixture'`).Scan(&original); err != nil || original != "legacy-data" {
				t.Fatal("failed transfer changed source data")
			}
		})
	}
}
