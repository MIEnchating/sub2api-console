package configstore_test

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"sync"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func TestWorkbenchDocumentConcurrentCreateKeepsExactlyOneWinner(t *testing.T) {
	store, err := configstore.Open(filepath.Join(t.TempDir(), "private.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	const writers = 8
	results := make(chan error, writers)
	var group sync.WaitGroup
	for range writers {
		group.Go(func() {
			_, err := store.SaveWorkbenchDocument(ctx, "preview:isolated", 0, json.RawMessage(`{"private":"test"}`))
			results <- err
		})
	}
	group.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
		} else if !errors.Is(err, configstore.ErrWorkbenchVersion) {
			t.Fatal(err)
		}
	}
	if successes != 1 {
		t.Fatalf("concurrent create winners=%d", successes)
	}
	raw, revision, err := store.WorkbenchDocument(ctx, "preview:isolated")
	if err != nil || revision != 1 || string(raw) != `{"private":"test"}` {
		t.Fatal("winning document corrupted")
	}
}

func TestWorkbenchDocumentStaleUpdateCannotOverwriteCurrentContent(t *testing.T) {
	store, err := configstore.Open(filepath.Join(t.TempDir(), "private.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	if _, err = store.SaveWorkbenchDocument(ctx, "templates:site", 0, json.RawMessage(`{"name":"first"}`)); err != nil {
		t.Fatal(err)
	}
	if _, err = store.SaveWorkbenchDocument(ctx, "templates:site", 1, json.RawMessage(`{"name":"current"}`)); err != nil {
		t.Fatal(err)
	}
	if _, err = store.SaveWorkbenchDocument(ctx, "templates:site", 1, json.RawMessage(`{"name":"stale"}`)); !errors.Is(err, configstore.ErrWorkbenchVersion) {
		t.Fatal("stale update accepted")
	}
	raw, revision, err := store.WorkbenchDocument(ctx, "templates:site")
	if err != nil || revision != 2 || string(raw) != `{"name":"current"}` {
		t.Fatal("stale update changed data")
	}
}

func TestWorkbenchDocumentDeleteRequiresCurrentRevisionAndListUsesLiteralPrefix(t *testing.T) {
	store, err := configstore.Open(filepath.Join(t.TempDir(), "private.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	for _, key := range []string{"run:owner%:one", "run:owner-other:two"} {
		if _, err = store.SaveWorkbenchDocument(ctx, key, 0, json.RawMessage(`{}`)); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := store.WorkbenchDocuments(ctx, "run:owner%:", 10)
	if err != nil || len(rows) != 1 || rows[0].ID != "run:owner%:one" {
		t.Fatal("namespace interpreted as wildcard")
	}
	if err = store.DeleteWorkbenchDocument(ctx, rows[0].ID, 0); !errors.Is(err, configstore.ErrWorkbenchVersion) {
		t.Fatal("stale delete succeeded")
	}
	if err = store.DeleteWorkbenchDocument(ctx, rows[0].ID, 1); err != nil {
		t.Fatal(err)
	}
	rows, err = store.WorkbenchDocuments(ctx, "run:owner%:", 10)
	if err != nil || len(rows) != 0 {
		t.Fatal("deleted document remained readable")
	}
}
