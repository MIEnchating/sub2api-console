package dbtransfer_test

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/dbconn"
	"github.com/MIEnchating/sub2api-console/backend/internal/dbtest"
	"github.com/MIEnchating/sub2api-console/backend/internal/dbtransfer"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

func TestDatabaseTransferWorkbenchDocumentsPreserveInsertionOrderAfterNewWrite(t *testing.T) {
	for _, cfg := range dbtest.Configs(t) {
		if cfg.Driver == dbconn.SQLite {
			continue
		}
		t.Run(string(cfg.Driver), func(t *testing.T) {
			emptyContractTarget(t, cfg)
			defer emptyContractTarget(t, cfg)
			root := t.TempDir()
			sources := dbtransfer.Sources{Config: filepath.Join(root, "config.db"), Business: filepath.Join(root, "business.db"), Tasks: filepath.Join(root, "tasks.db")}
			source, err := configstore.Open(sources.Config)
			if err != nil {
				t.Fatal(err)
			}
			payloads := map[string]json.RawMessage{}
			expectedIDs := []string{"fixture/new"}
			for index := range 205 {
				key := fmt.Sprintf("fixture/%03d", 204-index)
				payload := json.RawMessage(`{"name":"迁移资料","exact":9007199254740993,"optional":null}`)
				if index == 100 {
					payload = json.RawMessage(`{"text":"` + strings.Repeat("中", 400000) + `"}`)
				}
				payloads[key] = payload
				expectedIDs = append(expectedIDs, key)
				if _, err = source.SaveWorkbenchDocument(t.Context(), key, 0, payload); err != nil {
					t.Fatal(err)
				}
			}
			if err = source.Close(); err != nil {
				t.Fatal(err)
			}
			data, err := business.Open(sources.Business)
			if err != nil {
				t.Fatal(err)
			}
			if err = data.Close(); err != nil {
				t.Fatal(err)
			}
			tasks, err := taskstore.Open(sources.Tasks)
			if err != nil {
				t.Fatal(err)
			}
			if err = tasks.Close(); err != nil {
				t.Fatal(err)
			}
			report, err := dbtransfer.Transfer(t.Context(), dbtransfer.Options{Sources: sources, BackupDirectory: filepath.Join(root, "backup"), Target: cfg, Stopped: true})
			if err != nil {
				t.Fatal(err)
			}
			if report.State != "complete" {
				t.Fatalf("import state=%s", report.State)
			}
			target, err := configstore.OpenDatabase(t.Context(), cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer target.Close()
			if _, err = target.SaveWorkbenchDocument(t.Context(), "fixture/new", 0, json.RawMessage(`{"name":"新资料"}`)); err != nil {
				t.Fatal(err)
			}
			var ids []string
			err = target.VisitWorkbenchDocuments(t.Context(), "fixture/", 501, func(record configstore.WorkbenchDocumentRecord) (bool, error) {
				ids = append(ids, record.ID)
				if record.ID != "fixture/new" && (string(record.Payload) != string(payloads[record.ID]) || record.Revision != 1) {
					t.Fatalf("imported document changed: %s", record.ID)
				}
				return true, nil
			})
			if err != nil {
				t.Fatal(err)
			}
			for left, right := 1, len(expectedIDs)-1; left < right; left, right = left+1, right-1 {
				expectedIDs[left], expectedIDs[right] = expectedIDs[right], expectedIDs[left]
			}
			if !reflect.DeepEqual(ids, expectedIDs) {
				t.Fatalf("insertion order=%v", ids)
			}
		})
	}
}
