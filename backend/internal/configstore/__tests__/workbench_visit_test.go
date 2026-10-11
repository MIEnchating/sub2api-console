package configstore_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func TestWorkbenchVisitKeepsLiteralScopeInsertionOrderAndLimit(t *testing.T) {
	store, _ := namespaceStore(t)
	const prefix = "run:会话%_':"
	for _, key := range []string{prefix + "z", prefix + "a", prefix + "m", "run:other:n"} {
		if _, err := store.SaveWorkbenchDocument(t.Context(), key, 0, json.RawMessage(`{"private":"original"}`)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.SaveWorkbenchDocument(t.Context(), prefix+"a", 1, json.RawMessage(`{"number":9007199254740993}`)); err != nil {
		t.Fatal(err)
	}
	var records []configstore.WorkbenchDocumentRecord
	err := store.VisitWorkbenchDocuments(t.Context(), prefix, 2, func(record configstore.WorkbenchDocumentRecord) (bool, error) {
		records = append(records, record)
		return true, nil
	})
	if err != nil || !reflect.DeepEqual(namespaceIDs(records), []string{prefix + "m", prefix + "a"}) {
		t.Fatalf("records=%v err=%v", namespaceIDs(records), err)
	}
	if records[1].Revision != 2 || string(records[1].Payload) != `{"number":9007199254740993}` {
		t.Fatal("visitor changed stored revision or payload precision")
	}
}

func TestWorkbenchVisitStopsOnMatchOrErrorAndReleasesConnection(t *testing.T) {
	for _, failure := range []bool{false, true} {
		t.Run(map[bool]string{false: "match", true: "failure"}[failure], func(t *testing.T) {
			store, _ := namespaceStore(t)
			for _, key := range []string{"run:owner:older", "run:owner:newer"} {
				if _, err := store.SaveWorkbenchDocument(t.Context(), key, 0, json.RawMessage(`{}`)); err != nil {
					t.Fatal(err)
				}
			}
			wantErr := errors.New("invalid private record")
			var seen []string
			err := store.VisitWorkbenchDocuments(t.Context(), "run:owner:", 501, func(record configstore.WorkbenchDocumentRecord) (bool, error) {
				seen = append(seen, record.ID)
				if failure {
					return true, wantErr
				}
				return false, nil
			})
			if !reflect.DeepEqual(seen, []string{"run:owner:newer"}) || (failure && !errors.Is(err, wantErr)) || (!failure && err != nil) {
				t.Fatalf("seen=%v err=%v", seen, err)
			}
			if _, err := store.SaveWorkbenchDocument(t.Context(), "run:owner:newer", 1, json.RawMessage(`{"updated":true}`)); err != nil {
				t.Fatalf("visitor did not release database for subsequent save: %v", err)
			}
		})
	}
}

func TestWorkbenchVisitRejectsInvalidRangeAndReturnsEmptyForMissingScope(t *testing.T) {
	store, _ := namespaceStore(t)
	for _, scenario := range []struct {
		prefix string
		limit  int
	}{{"", 1}, {strings.Repeat("x", 201), 1}, {"run:owner:", 0}, {"run:owner:", 502}, {"run:missing:", 501}} {
		err := store.VisitWorkbenchDocuments(t.Context(), scenario.prefix, scenario.limit, func(configstore.WorkbenchDocumentRecord) (bool, error) {
			t.Fatal("invalid or missing scope visited a document")
			return false, nil
		})
		if (scenario.prefix == "run:missing:") != (err == nil) {
			t.Fatalf("prefix=%q limit=%d err=%v", scenario.prefix, scenario.limit, err)
		}
	}
}

func TestWorkbenchVisitCancelledOrClosedStoreReturnsReadFailure(t *testing.T) {
	store, _ := namespaceStore(t)
	visit := func(configstore.WorkbenchDocumentRecord) (bool, error) {
		t.Fatal("failed read visited a record")
		return false, nil
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := store.VisitWorkbenchDocuments(ctx, "run:owner:", 10, visit); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel error=%v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := store.VisitWorkbenchDocuments(t.Context(), "run:owner:", 10, visit); err == nil {
		t.Fatal("closed store reported successful empty visit")
	}
}

func TestWorkbenchVisitCancellationInVisitorDoesNotReturnSuccessfulMatch(t *testing.T) {
	store, _ := namespaceStore(t)
	if _, err := store.SaveWorkbenchDocument(t.Context(), "run:owner:one", 0, json.RawMessage(`{}`)); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	err := store.VisitWorkbenchDocuments(ctx, "run:owner:", 10, func(configstore.WorkbenchDocumentRecord) (bool, error) {
		cancel()
		return false, nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled match error=%v", err)
	}
}
