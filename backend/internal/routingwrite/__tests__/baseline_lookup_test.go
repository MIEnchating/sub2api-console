package routingwrite_test

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/routingwrite"
	"github.com/MIEnchating/sub2api-console/backend/internal/runtimepolicy"
)

type countedBaselineStore struct {
	*business.Store
	fullReads atomic.Int32
}

func (store *countedBaselineStore) RoutingBaselines(ctx context.Context) ([]business.RoutingBaseline, error) {
	store.fullReads.Add(1)
	return store.Store.RoutingBaselines(ctx)
}

func TestBatchReleaseRestoresEachAccountWithoutFullBaselineScans(t *testing.T) {
	store, err := business.Open(filepath.Join(t.TempDir(), "baselines.sqlite3"))
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
	fixture := &loadFactorFixture{states: map[string]map[string]any{}}
	accounts := []map[string]any{}
	targets := map[string]business.AccountRoutingTarget{}
	for _, id := range []string{"41", "42"} {
		state := map[string]any{"id": id, "name": "baseline-" + id, "schedulable": false,
			"priority": 20, "load_factor": 3, "concurrency": 4, "status": "active", "groups": []any{json.Number("7")}}
		fixture.states[id] = state
		accounts = append(accounts, state)
		load := "10"
		targets[id] = business.AccountRoutingTarget{AccountID: id, GroupNames: []string{"codex"}, LoadFactor: &load}
	}
	if _, err := store.SyncManagementSnapshot(t.Context(), accounts, []map[string]any{{"id": json.Number("7"), "name": "codex"}}, "test"); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(fixture)
	t.Cleanup(server.Close)
	repository := &countedBaselineStore{Store: store}
	service := routingwrite.New(routingTarget{url: server.URL}, repository)
	initial, err := service.Apply(t.Context(), targets, "test")
	if err != nil || initial.Failed != 0 || initial.Changed != 2 {
		t.Fatalf("initial write failed: %+v %v", initial, err)
	}
	for id, target := range targets {
		target.LoadFactor, target.ReleaseControl = nil, true
		targets[id] = target
	}
	result, err := service.Apply(t.Context(), targets, "test")
	if err != nil || result.Failed != 0 || result.Changed != 2 {
		t.Fatalf("batch release failed: %+v %v", result, err)
	}
	for _, item := range result.Results {
		load, ok := item.Effective["load_factor"].(*string)
		if !item.Restored || !ok || load == nil || *load != "3" {
			t.Fatalf("account baseline was not restored: %+v", item)
		}
	}
	if scans := repository.fullReads.Load(); scans != 0 {
		t.Fatalf("batch release performed %d full baseline scans", scans)
	}
}
