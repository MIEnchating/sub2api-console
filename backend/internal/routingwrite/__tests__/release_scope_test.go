package routingwrite_test

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/routingwrite"
	"github.com/MIEnchating/sub2api-console/backend/internal/runtimepolicy"
)

func TestReleaseWithoutBaselineSkipsRemoteAccountLookup(t *testing.T) {
	store, err := business.Open(filepath.Join(t.TempDir(), "release.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Bootstrap(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetMode(t.Context(), runtimepolicy.Full); err != nil {
		t.Fatal(err)
	}
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		http.Error(w, "account not found", http.StatusNotFound)
	}))
	t.Cleanup(server.Close)
	service := routingwrite.New(routingTarget{url: server.URL}, store)
	result, err := service.Apply(t.Context(), map[string]business.AccountRoutingTarget{
		"1169": {AccountID: "1169", GroupNames: []string{"codex"}, ReleaseControl: true, DesiredHealth: "excluded"},
	}, "scheduler")
	if err != nil {
		t.Fatal(err)
	}
	if result.Failed != 0 || result.RemoteWrite || len(result.Results) != 1 || !result.Results[0].Skipped {
		t.Fatalf("never-managed account must be skipped without a false inspection failure: %+v", result)
	}
	if requests.Load() != 0 {
		t.Fatalf("never-managed account caused %d remote requests", requests.Load())
	}
}

func TestUnmanagedReleaseDoesNotBlockRestoringManagedAccountInSameBatch(t *testing.T) {
	service, _, targets := newLoadFactorService(t, 1, true)
	targets["1169"] = business.AccountRoutingTarget{AccountID: "1169", GroupNames: []string{"test-group"}, ReleaseControl: true}
	result, err := service.Apply(t.Context(), targets, "scheduler")
	if err != nil || result.Failed != 0 || result.Changed != 1 {
		t.Fatalf("unmanaged release must not fail the managed restore batch: %+v err=%v", result, err)
	}
	for _, item := range result.Results {
		if item.AccountID == "41" && !item.Restored {
			t.Fatalf("managed account was not restored: %+v", item)
		}
		if item.AccountID == "1169" && !item.Skipped {
			t.Fatalf("never-managed account was not skipped: %+v", item)
		}
	}
}

func TestManagedReleaseStillReportsMissingRemoteAccount(t *testing.T) {
	service, fixture, targets := newLoadFactorService(t, 1, true)
	fixture.mu.Lock()
	delete(fixture.states, "41")
	fixture.mu.Unlock()
	result, err := service.Apply(t.Context(), targets, "scheduler")
	if err != nil {
		t.Fatal(err)
	}
	if result.Failed != 1 || len(result.Results) != 1 || result.Results[0].Skipped || result.Results[0].Error == nil {
		t.Fatalf("missing remote account with a captured baseline must remain a real failure: %+v", result)
	}
}
