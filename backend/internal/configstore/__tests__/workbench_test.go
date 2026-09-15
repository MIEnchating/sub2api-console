package configstore_test

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func TestWorkbenchTemplateCASPreservesNewerRevisionAndIsolatesTargets(t *testing.T) {
	store, err := configstore.Open(filepath.Join(t.TempDir(), "config.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	item := configstore.WorkbenchTemplate{ID: "template-1", Name: "Plus", TargetURL: "https://target.example", Config: configstore.WorkbenchTemplateConfig{"rate_multiplier": json.RawMessage(`0.123456789012345678901`)}}
	if err = store.SaveWorkbenchTemplate(ctx, item); err != nil {
		t.Fatal(err)
	}
	current, err := store.WorkbenchTemplate(ctx, item.TargetURL, item.ID)
	if err != nil || current.Revision != 1 {
		t.Fatalf("saved revision = %d, %v", current.Revision, err)
	}
	if string(current.Config["rate_multiplier"]) != "0.123456789012345678901" {
		t.Fatal("persisted decimal lost precision")
	}
	if err = store.SaveWorkbenchTemplate(ctx, item); !errors.Is(err, configstore.ErrWorkbenchTemplateConflict) {
		t.Fatalf("stale create = %v", err)
	}
	current.Name = "Updated"
	if err = store.SaveWorkbenchTemplate(ctx, current); err != nil {
		t.Fatal(err)
	}
	if err = store.DeleteWorkbenchTemplate(ctx, item.TargetURL, item.ID, 1); !errors.Is(err, configstore.ErrWorkbenchTemplateConflict) {
		t.Fatalf("stale delete = %v", err)
	}
	other, err := store.WorkbenchTemplates(ctx, "https://other.example")
	if err != nil || len(other) != 0 {
		t.Fatalf("other target list = %d, %v", len(other), err)
	}
	if _, err = store.WorkbenchTemplate(ctx, "https://other.example", item.ID); !errors.Is(err, configstore.ErrWorkbenchTemplateConflict) {
		t.Fatalf("cross target read = %v", err)
	}
	if err = store.DeleteWorkbenchTemplate(ctx, item.TargetURL, item.ID, 2); err != nil {
		t.Fatal(err)
	}
	list, err := store.WorkbenchTemplates(ctx, item.TargetURL)
	if err != nil || len(list) != 0 {
		t.Fatalf("deleted list = %d, %v", len(list), err)
	}
	if err = store.SaveWorkbenchTemplate(ctx, current); !errors.Is(err, configstore.ErrWorkbenchTemplateConflict) {
		t.Fatalf("stale update recreated deleted record: %v", err)
	}
}

func TestWorkbenchTemplatePersistenceRejectsSecretsAndInvalidConfig(t *testing.T) {
	store, err := configstore.Open(filepath.Join(t.TempDir(), "config.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	for name, config := range map[string]configstore.WorkbenchTemplateConfig{
		"credential key":         {"credential_extras": json.RawMessage(`{"refresh_token":"rt_secret"}`)},
		"identity extra":         {"extra": json.RawMessage(`{"email":"owner@example.com"}`)},
		"fractional concurrency": {"concurrency": json.RawMessage(`1.5`)},
		"negative multiplier":    {"rate_multiplier": json.RawMessage(`"-0.1"`)},
		"invalid group":          {"group_ids": json.RawMessage(`["abc"]`)},
	} {
		t.Run(name, func(t *testing.T) {
			err := store.SaveWorkbenchTemplate(context.Background(), configstore.WorkbenchTemplate{ID: "test", Name: "Template", TargetURL: "https://target.example", Config: config})
			if err == nil {
				t.Fatal("invalid template persisted")
			}
		})
	}
}
