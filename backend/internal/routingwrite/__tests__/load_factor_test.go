package routingwrite_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/routingwrite"
	"github.com/MIEnchating/sub2api-console/backend/internal/runtimepolicy"
)

type routingTarget struct{ url string }

func (target routingTarget) TargetSettings(context.Context) (configstore.TargetSettings, error) {
	return configstore.TargetSettings{BaseURL: target.url, AdminKey: "isolated-test-key", TimeoutSeconds: 2}, nil
}

type loadFactorFixture struct {
	mu           sync.Mutex
	states       map[string]map[string]any
	bulkWrites   int
	singleWrites int
	ignoreClear  bool
	omitReadback bool
}

// Reproduce Sub2API's contract: bulk updates ignore a zero load factor;
// individual updates clear it and return an explicit JSON null.
func (fixture *loadFactorFixture) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	path := strings.TrimPrefix(request.URL.Path, "/api/v1/admin/accounts")
	var data any
	switch {
	case request.Method == http.MethodGet && path == "":
		items := []map[string]any{}
		for _, state := range fixture.states {
			items = append(items, state)
		}
		data = map[string]any{"items": items, "total": len(items)}
	case request.Method == http.MethodPost && path == "/bulk-update":
		fixture.bulkWrites++
		var body struct {
			AccountIDs []int64 `json:"account_ids"`
			LoadFactor int     `json:"load_factor"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		results := []map[string]any{}
		for _, id := range body.AccountIDs {
			if body.LoadFactor > 0 {
				fixture.states[strconv.FormatInt(id, 10)]["load_factor"] = body.LoadFactor
			}
			results = append(results, map[string]any{"account_id": id, "success": true})
		}
		data = map[string]any{"success_ids": body.AccountIDs, "failed_ids": []int64{}, "results": results}
	case request.Method == http.MethodGet || request.Method == http.MethodPut:
		state, found := fixture.states[strings.TrimPrefix(path, "/")]
		if !found {
			http.NotFound(w, request)
			return
		}
		if request.Method == http.MethodPut {
			fixture.singleWrites++
			var body map[string]int
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				http.Error(w, "invalid body", http.StatusBadRequest)
				return
			}
			if value, present := body["load_factor"]; present && !fixture.ignoreClear {
				state["load_factor"] = nil
				if value > 0 {
					state["load_factor"] = value
				}
			}
			if fixture.omitReadback {
				delete(state, "load_factor")
			}
		}
		data = state
	default:
		http.NotFound(w, request)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": data})
}

func newLoadFactorService(t *testing.T, count int, verify bool) (*routingwrite.Service, *loadFactorFixture, map[string]business.AccountRoutingTarget) {
	t.Helper()
	store, err := business.Open(filepath.Join(t.TempDir(), "routing.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := t.Context()
	if err := store.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetMode(ctx, runtimepolicy.Full); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdatePolicy(ctx, map[string]any{
		"auto_apply":      map[string]any{"load_factor": true},
		"advanced_policy": map[string]any{"writeback": map[string]any{"verification": verify}},
	}, "test"); err != nil {
		t.Fatal(err)
	}
	fixture := &loadFactorFixture{states: map[string]map[string]any{}}
	accounts := []map[string]any{}
	targets := map[string]business.AccountRoutingTarget{}
	for index := range count {
		id := strconv.Itoa(41 + index)
		state := map[string]any{
			"id": id, "name": "clearing-" + id, "schedulable": false,
			"priority": 1000, "load_factor": nil, "concurrency": 100, "status": "active",
			"groups": []any{json.Number("7")},
		}
		fixture.states[id] = state
		accounts = append(accounts, state)
		load := "56"
		targets[id] = business.AccountRoutingTarget{AccountID: id, GroupNames: []string{"test-group"}, LoadFactor: &load}
	}
	if _, err := store.SyncManagementSnapshot(ctx, accounts, []map[string]any{{"id": json.Number("7"), "name": "test-group"}}, "test"); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(fixture)
	t.Cleanup(server.Close)
	service := routingwrite.New(routingTarget{url: server.URL}, store)
	initial, err := service.Apply(ctx, targets, "test")
	if err != nil || initial.Failed != 0 || initial.Changed != count {
		t.Fatalf("initial managed load factor failed: %+v %v", initial, err)
	}
	fixture.bulkWrites, fixture.singleWrites = 0, 0
	for id, target := range targets {
		target.LoadFactor, target.ReleaseControl = nil, true
		targets[id] = target
	}
	return service, fixture, targets
}

func TestClearingLoadFactorUsesIndividualUpdatesAndAcceptsExplicitNull(t *testing.T) {
	for _, scenario := range []struct {
		name   string
		count  int
		verify bool
	}{
		{name: "single_with_verification", count: 1, verify: true},
		{name: "multiple_with_verification", count: 2, verify: true},
		{name: "multiple_without_optional_verification", count: 2},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			service, fixture, targets := newLoadFactorService(t, scenario.count, scenario.verify)
			result, err := service.Apply(t.Context(), targets, "test")
			if err != nil || result.Failed != 0 || result.Changed != scenario.count {
				t.Fatalf("clear should succeed: result=%+v err=%v", result, err)
			}
			for _, item := range result.Results {
				if !item.Restored || item.Effective["load_factor"].(*string) != nil {
					t.Fatalf("confirmed clear must restore baseline and project null: %+v", item)
				}
			}
			fixture.mu.Lock()
			defer fixture.mu.Unlock()
			if fixture.bulkWrites != 0 || fixture.singleWrites != scenario.count {
				t.Fatalf("clear must use supported individual route: bulk=%d individual=%d", fixture.bulkWrites, fixture.singleWrites)
			}
			for _, state := range fixture.states {
				if value, present := state["load_factor"]; !present || value != nil {
					t.Fatalf("load factor was not cleared: %v", state)
				}
			}
		})
	}
}

func TestFailedLoadFactorClearRetainsBaselineForRetry(t *testing.T) {
	service, fixture, targets := newLoadFactorService(t, 1, false)
	fixture.ignoreClear = true
	failed, err := service.Apply(t.Context(), targets, "test")
	if err != nil || failed.Failed != 1 {
		t.Fatalf("expected rejected clear: %+v %v", failed, err)
	}
	fixture.ignoreClear = false
	retried, err := service.Apply(t.Context(), targets, "test")
	if err != nil || retried.Failed != 0 || retried.Changed != 1 || !retried.Results[0].Restored {
		t.Fatalf("retry lost the saved baseline: %+v %v", retried, err)
	}
}

func TestClearingLoadFactorDoesNotAcceptUnchangedOrMissingReadback(t *testing.T) {
	for _, scenario := range []string{"unchanged", "missing"} {
		t.Run(scenario, func(t *testing.T) {
			service, fixture, targets := newLoadFactorService(t, 1, false)
			fixture.ignoreClear = scenario == "unchanged"
			fixture.omitReadback = scenario == "missing"
			result, err := service.Apply(t.Context(), targets, "test")
			if err != nil || result.Failed != 1 || result.Changed != 0 || result.Results[0].Error == nil {
				t.Fatalf("unconfirmed clear must fail even when optional verification is off: result=%+v err=%v", result, err)
			}
		})
	}
}
