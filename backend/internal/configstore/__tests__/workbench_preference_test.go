package configstore_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func preferenceStore(t *testing.T) (*configstore.Store, *sql.DB) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "preferences.sqlite3")
	store, err := configstore.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return store, db
}

func preferenceTemplate(id, target string, preferred bool) configstore.WorkbenchTemplate {
	return configstore.WorkbenchTemplate{ID: id, Name: id, TargetURL: target, Preferred: preferred, Config: configstore.WorkbenchTemplateConfig{"rate_multiplier": json.RawMessage("0.123456789012345678901")}}
}

func TestWorkbenchPreferenceSwitchInvalidatesOldRevisionAndIsolatesTargets(t *testing.T) {
	store, _ := preferenceStore(t)
	ctx := context.Background()
	for _, item := range []configstore.WorkbenchTemplate{preferenceTemplate("first", "https://one.example", true), preferenceTemplate("other", "https://two.example", true), preferenceTemplate("second", "https://one.example", true)} {
		if err := store.SaveWorkbenchTemplate(ctx, item); err != nil {
			t.Fatal(err)
		}
	}
	first, err := store.WorkbenchTemplate(ctx, "https://one.example", "first")
	if err != nil || first.Preferred || first.Revision != 2 {
		t.Fatalf("previous preference was not versioned and cleared: %#v, %v", first, err)
	}
	first.Revision = 1
	if err := store.SaveWorkbenchTemplate(ctx, first); !errors.Is(err, configstore.ErrWorkbenchTemplateConflict) {
		t.Fatalf("old editor overwrote preference change: %v", err)
	}
	other, err := store.WorkbenchTemplate(ctx, "https://two.example", "other")
	if err != nil || !other.Preferred || other.Revision != 1 {
		t.Fatal("preference change affected another management target")
	}
}

func TestWorkbenchPreferenceClearingAndDeletingLeaveNoImplicitReplacement(t *testing.T) {
	for _, operation := range []string{"clear", "delete"} {
		t.Run(operation, func(t *testing.T) {
			store, _ := preferenceStore(t)
			ctx := context.Background()
			item := preferenceTemplate("first", "https://one.example", true)
			if err := store.SaveWorkbenchTemplate(ctx, item); err != nil {
				t.Fatal(err)
			}
			item.Revision = 1
			item.Preferred = false
			var err error
			if operation == "clear" {
				err = store.SaveWorkbenchTemplate(ctx, item)
			} else {
				err = store.DeleteWorkbenchTemplate(ctx, item.TargetURL, item.ID, item.Revision)
			}
			if err != nil {
				t.Fatal(err)
			}
			items, err := store.WorkbenchTemplates(ctx, item.TargetURL)
			if err != nil {
				t.Fatal(err)
			}
			for _, current := range items {
				if current.Preferred {
					t.Fatal("cleared preference was implicitly replaced")
				}
			}
		})
	}
}

func TestWorkbenchConcurrentPreferencesKeepExactlyOneSelectionAndBothConfigs(t *testing.T) {
	store, _ := preferenceStore(t)
	ctx := context.Background()
	items := []configstore.WorkbenchTemplate{preferenceTemplate("first", "https://one.example", true), preferenceTemplate("second", "https://one.example", true)}
	start := make(chan struct{})
	results := make(chan error, len(items))
	var workers sync.WaitGroup
	for _, item := range items {
		workers.Go(func() { <-start; results <- store.SaveWorkbenchTemplate(ctx, item) })
	}
	close(start)
	workers.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatal(err)
		}
	}
	stored, err := store.WorkbenchTemplates(ctx, items[0].TargetURL)
	if err != nil || len(stored) != 2 {
		t.Fatalf("concurrent saves lost templates: %v", err)
	}
	preferred := 0
	for _, item := range stored {
		if item.Preferred {
			preferred++
		}
		if string(item.Config["rate_multiplier"]) != "0.123456789012345678901" {
			t.Fatal("preference mutation altered decimal config")
		}
	}
	if preferred != 1 {
		t.Fatalf("preferred count = %d", preferred)
	}
}

func TestWorkbenchPreferenceFailureRollsBackNewPreferenceAndOldRevision(t *testing.T) {
	store, db := preferenceStore(t)
	ctx := context.Background()
	first := preferenceTemplate("first", "https://one.example", true)
	if err := store.SaveWorkbenchTemplate(ctx, first); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TRIGGER reject_preference BEFORE UPDATE ON settings WHEN json_extract(OLD.value,'$.id')='first' BEGIN SELECT RAISE(FAIL,'isolated preference failure'); END`); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveWorkbenchTemplate(ctx, preferenceTemplate("second", first.TargetURL, true)); err == nil {
		t.Fatal("failed preference clearing was ignored")
	}
	items, err := store.WorkbenchTemplates(ctx, first.TargetURL)
	if err != nil || len(items) != 1 || !items[0].Preferred || items[0].Revision != 1 {
		t.Fatalf("failed transaction changed preference state: %#v, %v", items, err)
	}
}

func TestWorkbenchTemplateSourceMetadataRequiresStableIdentityAndPreviewRevision(t *testing.T) {
	store, _ := preferenceStore(t)
	valid := preferenceTemplate("source", "https://one.example", false)
	valid.SourceAccountID, valid.SourceName = "101", "Source"
	valid.SourceRevision, valid.SourceSyncedAt = strings.Repeat("a", 64), "2026-09-14T00:00:00Z"
	if err := store.SaveWorkbenchTemplate(context.Background(), valid); err != nil {
		t.Fatal(err)
	}
	stored, err := store.WorkbenchTemplate(context.Background(), valid.TargetURL, valid.ID)
	if err != nil || stored.SourceAccountID != valid.SourceAccountID || stored.SourceRevision != valid.SourceRevision || stored.SourceSyncedAt != valid.SourceSyncedAt {
		t.Fatal("source metadata did not survive private persistence")
	}
	for _, condition := range []string{"unstable-id", "missing-id", "missing-revision", "invalid-time"} {
		t.Run(condition, func(t *testing.T) {
			item := valid
			item.ID = condition
			switch condition {
			case "unstable-id":
				item.SourceAccountID = "01"
			case "missing-id":
				item.SourceAccountID = ""
			case "missing-revision":
				item.SourceRevision = ""
			case "invalid-time":
				item.SourceSyncedAt = "yesterday"
			}
			if err := store.SaveWorkbenchTemplate(context.Background(), item); err == nil {
				t.Fatal("invalid source metadata persisted")
			}
		})
	}
}
