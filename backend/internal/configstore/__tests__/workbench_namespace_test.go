package configstore_test

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func namespaceStore(t testing.TB) (*configstore.Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "isolated-namespaces.sqlite")
	store, err := configstore.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store, path
}

func TestWorkbenchNamespaceReadsUseLiteralBytePrefixAndKeepTheirOrdering(t *testing.T) {
	for _, prefix := range []string{"run:owner%_':", "run:会话:", "run:😀:", "run:\x7f", "\xff\xff"} {
		t.Run(prefix, func(t *testing.T) {
			store, _ := namespaceStore(t)
			for _, id := range []string{prefix + "b", prefix + "a", prefix, prefix + "c", "other:latest"} {
				if _, err := store.SaveWorkbenchDocument(t.Context(), id, 0, json.RawMessage(`{"version":"first"}`)); err != nil {
					t.Fatal(err)
				}
			}
			// Updating an older record must not make it the newest insertion.
			if _, err := store.SaveWorkbenchDocument(t.Context(), prefix+"b", 1, json.RawMessage(`{"number":9007199254740993}`)); err != nil {
				t.Fatal(err)
			}
			latest, err := store.WorkbenchDocuments(t.Context(), prefix, 2)
			if err != nil || len(latest) != 2 || latest[0].ID != prefix+"c" || latest[1].ID != prefix {
				t.Fatalf("latest IDs=%v error=%v", namespaceIDs(latest), err)
			}
			page, err := store.WorkbenchDocumentPage(t.Context(), prefix, "", 2)
			if err != nil || !reflect.DeepEqual(namespaceIDs(page), []string{prefix, prefix + "a"}) {
				t.Fatalf("first page=%v error=%v", namespaceIDs(page), err)
			}
			page, err = store.WorkbenchDocumentPage(t.Context(), prefix, prefix+"a", 2)
			if err != nil || !reflect.DeepEqual(namespaceIDs(page), []string{prefix + "b", prefix + "c"}) {
				t.Fatalf("next page=%v error=%v", namespaceIDs(page), err)
			}
			if page[0].Revision != 2 || string(page[0].Payload) != `{"number":9007199254740993}` {
				t.Fatal("selected revision or original JSON changed")
			}
			page, err = store.WorkbenchDocumentPage(t.Context(), prefix, prefix+"z", 2)
			if err != nil || page == nil || len(page) != 0 {
				t.Fatalf("exhausted page=%v error=%v", namespaceIDs(page), err)
			}
		})
	}
}

func namespaceIDs(records []configstore.WorkbenchDocumentRecord) []string {
	ids := make([]string, len(records))
	for i, record := range records {
		ids[i] = record.ID
	}
	return ids
}

func TestWorkbenchNamespaceCursorSurvivesDeletionAndDoesNotRepeatBackfilledKeys(t *testing.T) {
	store, _ := namespaceStore(t)
	for _, id := range []string{"run:owner:1", "run:owner:2", "run:owner:3", "run:owner-other:9"} {
		if _, err := store.SaveWorkbenchDocument(t.Context(), id, 0, json.RawMessage(`{}`)); err != nil {
			t.Fatal(err)
		}
	}
	page, err := store.WorkbenchDocumentPage(t.Context(), "run:owner:", "", 2)
	if err != nil || !reflect.DeepEqual(namespaceIDs(page), []string{"run:owner:1", "run:owner:2"}) {
		t.Fatalf("first page=%v error=%v", namespaceIDs(page), err)
	}
	if err := store.DeleteWorkbenchDocument(t.Context(), page[1].ID, page[1].Revision); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"run:owner:0", "run:owner:4"} {
		if _, err := store.SaveWorkbenchDocument(t.Context(), id, 0, json.RawMessage(`{}`)); err != nil {
			t.Fatal(err)
		}
	}
	page, err = store.WorkbenchDocumentPage(t.Context(), "run:owner:", "run:owner:2", 2)
	if err != nil || !reflect.DeepEqual(namespaceIDs(page), []string{"run:owner:3", "run:owner:4"}) {
		t.Fatalf("continued page=%v error=%v", namespaceIDs(page), err)
	}
}

func TestWorkbenchNamespaceReadsDoNotExpandEmptyOrMissingScope(t *testing.T) {
	store, _ := namespaceStore(t)
	if _, err := store.SaveWorkbenchDocument(t.Context(), "run:owner:1", 0, json.RawMessage(`{}`)); err != nil {
		t.Fatal(err)
	}
	for _, prefix := range []string{"", "run:missing:"} {
		latest, latestErr := store.WorkbenchDocuments(t.Context(), prefix, 501)
		page, pageErr := store.WorkbenchDocumentPage(t.Context(), prefix, "", 501)
		if len(latest) != 0 || len(page) != 0 {
			t.Fatal("scope expanded to unrelated documents")
		}
		if prefix == "" && (latestErr == nil || pageErr == nil) {
			t.Fatal("empty namespace accepted")
		}
		if prefix != "" && (latestErr != nil || pageErr != nil || latest == nil || page == nil) {
			t.Fatalf("missing scope did not return empty lists: %v %v", latestErr, pageErr)
		}
	}
}

func TestWorkbenchNamespaceCancelledReadsReturnNoDocuments(t *testing.T) {
	store, _ := namespaceStore(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	latest, latestErr := store.WorkbenchDocuments(ctx, "run:owner:", 20)
	page, pageErr := store.WorkbenchDocumentPage(ctx, "run:owner:", "", 20)
	if len(latest) != 0 || len(page) != 0 || !errors.Is(latestErr, context.Canceled) || !errors.Is(pageErr, context.Canceled) {
		t.Fatalf("cancelled reads: latest=%v page=%v", latestErr, pageErr)
	}
}
