package routingwrite_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/routingwrite"
	"github.com/MIEnchating/sub2api-console/backend/internal/runtimepolicy"
)

type upstreamCapacityFixture struct {
	mu                  sync.Mutex
	states              map[string]map[string]any
	failSnapshot        bool
	omitSibling         bool
	snapshotReads       int
	store               *business.Store
	failParameters      bool
	ignoreParameters    bool
	enableBeforeConfirm bool
	confirmedParameters map[string]bool
	snapshotConcurrency map[string]int
	serverURL           string
}

func (fixture *upstreamCapacityFixture) accounts() []map[string]any {
	result := make([]map[string]any, 0, len(fixture.states))
	for id, state := range fixture.states {
		if id != "42" || !fixture.omitSibling {
			result = append(result, state)
		}
	}
	return result
}

func (fixture *upstreamCapacityFixture) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	path := strings.TrimPrefix(request.URL.Path, "/api/v1/admin/accounts")
	var data any
	if request.Method == http.MethodGet && path == "" {
		fixture.snapshotReads++
		for id, value := range fixture.snapshotConcurrency {
			fixture.states[id]["concurrency"] = value
		}
		if fixture.failSnapshot {
			http.Error(w, "snapshot unavailable", http.StatusServiceUnavailable)
			return
		}
		data = map[string]any{"items": fixture.accounts(), "total": len(fixture.accounts())}
	} else if request.Method == http.MethodPost && path == "/bulk-update" {
		var body struct {
			AccountIDs  []int64 `json:"account_ids"`
			Concurrency int     `json:"concurrency"`
			Schedulable *bool   `json:"schedulable"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		results := []map[string]any{}
		for _, id := range body.AccountIDs {
			accountID := strconv.FormatInt(id, 10)
			if !fixture.ignoreParameters {
				fixture.states[accountID]["concurrency"] = body.Concurrency
			}
			if body.Schedulable != nil {
				fixture.states[accountID]["schedulable"] = *body.Schedulable
				if *body.Schedulable && fixture.confirmedParameters != nil {
					fixture.enableBeforeConfirm = true
				}
			}
			results = append(results, map[string]any{"account_id": id, "success": true})
		}
		data = map[string]any{"success_ids": body.AccountIDs, "failed_ids": []int64{}, "results": results}
	} else {
		id := strings.TrimSuffix(strings.TrimPrefix(path, "/"), "/schedulable")
		state, exists := fixture.states[id]
		if !exists {
			http.NotFound(w, request)
			return
		}
		if request.Method == http.MethodPut || request.Method == http.MethodPost {
			if request.Method == http.MethodPut && fixture.failParameters {
				http.Error(w, "parameter write rejected", http.StatusUnprocessableEntity)
				return
			}
			var fields map[string]json.RawMessage
			if err := json.NewDecoder(request.Body).Decode(&fields); err != nil {
				http.Error(w, "invalid body", http.StatusBadRequest)
				return
			}
			if raw, present := fields["concurrency"]; present && !fixture.ignoreParameters {
				var concurrency int
				if err := json.Unmarshal(raw, &concurrency); err != nil {
					http.Error(w, "invalid concurrency", http.StatusBadRequest)
					return
				}
				state["concurrency"] = concurrency
				if fixture.confirmedParameters != nil {
					fixture.confirmedParameters[id] = false
				}
			}
			if raw, present := fields["schedulable"]; present {
				var schedulable bool
				if err := json.Unmarshal(raw, &schedulable); err != nil {
					http.Error(w, "invalid schedulable", http.StatusBadRequest)
					return
				}
				state["schedulable"] = schedulable
				if schedulable && fixture.confirmedParameters != nil && !fixture.confirmedParameters[id] {
					fixture.enableBeforeConfirm = true
				}
			}
		}
		if request.Method == http.MethodGet && fixture.confirmedParameters != nil {
			fixture.confirmedParameters[id] = true
		}
		data = state
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": data})
}

func writeCapacityPointer(value int64) *int64 { return &value }

func newUpstreamCapacityWriteFixture(t *testing.T, limit *int64, current, other int, active bool) (*routingwrite.Service, *upstreamCapacityFixture) {
	t.Helper()
	store, err := business.Open(filepath.Join(t.TempDir(), "upstream-capacity.sqlite3"))
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
	if _, err := store.UpdatePolicy(t.Context(), map[string]any{
		"auto_apply": map[string]any{"concurrency": true, "schedulable": true},
	}, "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateUpstreamConfiguration(t.Context(), business.UpstreamConfigurationWrite{
		Host: "capacity.example", BaseURL: "https://capacity.example", UpstreamType: "sub2api", AuthMode: "sub2api_user_token", RechargeRate: "1",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ApplyUpstreamSync(t.Context(), business.UpstreamSyncWrite{
		Host: "capacity.example", Balance: &business.UpstreamBalanceObservation{ConcurrencyLimit: limit, ProfileUserID: "17"},
	}); err != nil {
		t.Fatal(err)
	}
	fixture := &upstreamCapacityFixture{states: map[string]map[string]any{}, store: store}
	for _, id := range []string{"41", "42"} {
		concurrency, schedulable := other, true
		if id == "41" {
			concurrency, schedulable = current, active
		}
		fixture.states[id] = map[string]any{
			"id": id, "name": "capacity-" + id, "schedulable": schedulable,
			"priority": 1000, "load_factor": 10, "concurrency": concurrency, "status": "active",
			"groups": []any{json.Number("7")}, "upstream_host": "capacity.example", "upstream_type": "sub2api",
		}
	}
	groups := []map[string]any{{"id": json.Number("7"), "name": "capacity-group"}}
	if _, err := store.SyncManagementSnapshot(t.Context(), fixture.accounts(), groups, "test"); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(fixture)
	t.Cleanup(server.Close)
	fixture.serverURL = server.URL
	return routingwrite.New(routingTarget{url: server.URL}, store), fixture
}
