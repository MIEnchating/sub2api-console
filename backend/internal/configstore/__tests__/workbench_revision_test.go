package configstore_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func TestWorkbenchDocumentRevisionTracksCommittedCreateUpdateAndDelete(t *testing.T) {
	store, _ := namespaceStore(t)
	ctx := t.Context()
	const key = "run:owner:private"
	if revision, err := store.WorkbenchDocumentRevision(ctx, key); err != nil || revision != 0 {
		t.Fatalf("missing revision=%d err=%v", revision, err)
	}
	raw := json.RawMessage(`{"private":"` + strings.Repeat("x", 128<<10) + `"}`)
	if _, err := store.SaveWorkbenchDocument(ctx, key, 0, raw); err != nil {
		t.Fatal(err)
	}
	if revision, err := store.WorkbenchDocumentRevision(ctx, key); err != nil || revision != 1 {
		t.Fatalf("created revision=%d err=%v", revision, err)
	}
	if _, err := store.SaveWorkbenchDocument(ctx, key, 1, json.RawMessage(`{"private":"updated"}`)); err != nil {
		t.Fatal(err)
	}
	if revision, err := store.WorkbenchDocumentRevision(ctx, key); err != nil || revision != 2 {
		t.Fatalf("updated revision=%d err=%v", revision, err)
	}
	if _, err := store.SaveWorkbenchDocument(ctx, key, 1, raw); !errors.Is(err, configstore.ErrWorkbenchVersion) {
		t.Fatalf("stale save error=%v", err)
	}
	if revision, err := store.WorkbenchDocumentRevision(ctx, key); err != nil || revision != 2 {
		t.Fatalf("rejected save changed revision=%d err=%v", revision, err)
	}
	if err := store.DeleteWorkbenchDocument(ctx, key, 2); err != nil {
		t.Fatal(err)
	}
	if revision, err := store.WorkbenchDocumentRevision(ctx, key); err != nil || revision != 0 {
		t.Fatalf("deleted revision=%d err=%v", revision, err)
	}
}

func TestWorkbenchDocumentRevisionUsesExactKeyAcrossSessions(t *testing.T) {
	store, _ := namespaceStore(t)
	ctx := t.Context()
	if _, err := store.SaveWorkbenchDocument(ctx, "preview:owner%_':会话", 0, json.RawMessage(`{}`)); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"preview:owner", "preview:owner-other:会话", "preview:missing", "preview:owner%_':会话"} {
		want := int64(0)
		if key == "preview:owner%_':会话" {
			want = 1
		}
		if revision, err := store.WorkbenchDocumentRevision(ctx, key); err != nil || revision != want {
			t.Fatalf("key=%q revision=%d want=%d err=%v", key, revision, want, err)
		}
	}
}

func TestWorkbenchDocumentRevisionReturnsCancellationAndDatabaseErrors(t *testing.T) {
	store, _ := namespaceStore(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := store.WorkbenchDocumentRevision(ctx, "preview:owner"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled read error=%v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := store.WorkbenchDocumentRevision(t.Context(), "preview:owner"); err == nil {
		t.Fatal("closed database reported a missing document instead of an error")
	}
}
