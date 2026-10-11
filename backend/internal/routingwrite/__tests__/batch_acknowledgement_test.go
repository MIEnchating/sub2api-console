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

type batchAcknowledgementFixture struct {
	mu               sync.Mutex
	states           map[string]map[string]any
	reply            map[string]any
	skipApply        string
	writes           int
	readbacks        int
	lists            int
	preparationReads int
	onPrepare        func(*http.Request)
	onWrite          func()
	store            *business.Store
	onReadback       func(*http.Request)
}

func (fixture *batchAcknowledgementFixture) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	fixture.mu.Lock()
	path := strings.TrimPrefix(request.URL.Path, "/api/v1/admin/accounts")
	readback := request.Method == http.MethodGet && path != "" && fixture.writes > 0
	if readback {
		fixture.readbacks++
	}
	preparing := request.Method == http.MethodGet && path != "" && fixture.writes == 0
	if preparing {
		fixture.preparationReads++
	}
	fixture.mu.Unlock()
	if preparing && fixture.onPrepare != nil {
		fixture.onPrepare(request)
	}
	if readback && fixture.onReadback != nil {
		fixture.onReadback(request)
	}
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	var data any
	switch {
	case request.Method == http.MethodPost && path == "/bulk-update":
		var body struct {
			AccountIDs []int64 `json:"account_ids"`
			Priority   int     `json:"priority"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		fixture.writes++
		if fixture.onWrite != nil {
			fixture.onWrite()
		}
		for _, id := range body.AccountIDs {
			accountID := strconv.FormatInt(id, 10)
			if accountID != fixture.skipApply {
				fixture.states[accountID]["priority"] = body.Priority
			}
		}
		data = fixture.reply
	case request.Method == http.MethodGet && path == "":
		fixture.lists++
		items := []map[string]any{}
		for _, state := range fixture.states {
			items = append(items, state)
		}
		data = map[string]any{"items": items, "total": len(items)}
	case request.Method == http.MethodGet:
		state, exists := fixture.states[strings.TrimPrefix(path, "/")]
		if !exists {
			http.NotFound(w, request)
			return
		}
		data = state
	default:
		http.Error(w, "unexpected request", http.StatusBadRequest)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": data})
}

func newBatchAcknowledgementService(t testing.TB, fixture *batchAcknowledgementFixture, count int, verify bool) (*routingwrite.Service, map[string]business.AccountRoutingTarget) {
	t.Helper()
	store, err := business.Open(filepath.Join(t.TempDir(), "batch-acknowledgement.sqlite3"))
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
		"auto_apply":      map[string]any{"priority": true},
		"advanced_policy": map[string]any{"writeback": map[string]any{"verification": verify, "concurrency": 2}},
	}, "test"); err != nil {
		t.Fatal(err)
	}
	fixture.store = store
	fixture.states = map[string]map[string]any{}
	accounts := []map[string]any{}
	targets := map[string]business.AccountRoutingTarget{}
	for index := range count {
		id := strconv.Itoa(41 + index)
		state := map[string]any{
			"id": id, "name": "batch-" + id, "schedulable": true,
			"priority": 1000, "load_factor": 10, "concurrency": 10, "status": "active",
			"groups": []any{json.Number("7")},
		}
		fixture.states[id] = state
		accounts = append(accounts, state)
		priority := int64(500)
		targets[id] = business.AccountRoutingTarget{AccountID: id, GroupNames: []string{"batch-group"}, Priority: &priority}
	}
	if _, err := store.SyncManagementSnapshot(t.Context(), accounts, []map[string]any{{"id": json.Number("7"), "name": "batch-group"}}, "test"); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(fixture)
	t.Cleanup(server.Close)
	return routingwrite.New(routingTarget{url: server.URL}, store), targets
}

func TestBatchAcknowledgementsAvoidUnnecessaryReadbackAndPreserveAmbiguity(t *testing.T) {
	t.Parallel()
	for _, scenario := range []struct {
		name        string
		reply       string
		skipApply   string
		readbacks   int
		failed      int
		failureText string
	}{
		{name: "id-only results confirm without extra reads", reply: `{"results":[{"id":41,"success":true},{"id":42,"success":true}]}`},
		{name: "null primary IDs use valid aliases", reply: `{"results":[{"account_id":null,"id":41,"success":true},{"account_id":null,"id":42,"success":true}]}`},
		{name: "matching summary and details confirm without extra reads", reply: `{"success_ids":[41,42],"failed_ids":[],"results":[{"account_id":41,"success":true},{"account_id":42,"success":true}]}`},
		{name: "matching failure preserves detailed rejection", reply: `{"success_ids":[41],"failed_ids":[42],"results":[{"account_id":41,"success":true},{"account_id":42,"success":false,"error":"account rejected"}]}`, skipApply: "42", failed: 1, failureText: "account rejected"},
		{name: "missing account requires readback", reply: `{"success_ids":[41]}`, readbacks: 1},
		{name: "duplicate summary ID still requires readback", reply: `{"success_ids":[41,41,42]}`, readbacks: 1},
		{name: "duplicate result still requires readback", reply: `{"results":[{"account_id":41,"success":true},{"account_id":41,"success":true},{"account_id":42,"success":true}]}`, readbacks: 1},
		{name: "contradictory outcomes cannot confirm unchanged account", reply: `{"success_ids":[41,42],"results":[{"account_id":41,"success":false}]}`, skipApply: "41", readbacks: 1, failed: 1},
		{name: "unknown account requires fresh confirmation", reply: `{"success_ids":[41,42,99]}`, readbacks: 2},
		{name: "conflicting ID aliases cannot confirm unchanged account", reply: `{"results":[{"account_id":41,"id":42,"success":true},{"account_id":42,"success":true}]}`, skipApply: "41", readbacks: 2, failed: 1},
		{name: "empty response requires readback", reply: `{}`, readbacks: 2},
		{name: "invalid explicit ID cannot use a valid alias", reply: `{"results":[{"account_id":0,"id":41,"success":true},{"account_id":42,"success":true}]}`, skipApply: "41", readbacks: 2, failed: 1},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			fixture := &batchAcknowledgementFixture{skipApply: scenario.skipApply}
			if err := json.Unmarshal([]byte(scenario.reply), &fixture.reply); err != nil {
				t.Fatal(err)
			}
			service, targets := newBatchAcknowledgementService(t, fixture, 2, false)
			result, err := service.Apply(t.Context(), targets, "test")
			if err != nil || result.Failed != scenario.failed {
				t.Fatalf("unexpected write result: %+v err=%v", result, err)
			}
			if scenario.failureText != "" {
				var failure string
				for _, item := range result.Results {
					if item.Error != nil {
						failure = *item.Error
					}
				}
				if !strings.Contains(failure, scenario.failureText) {
					t.Fatalf("lost per-account rejection: %q", failure)
				}
			}
			fixture.mu.Lock()
			defer fixture.mu.Unlock()
			if fixture.writes != 1 || fixture.readbacks != scenario.readbacks || fixture.lists != 0 {
				t.Fatalf("writes=%d readbacks=%d lists=%d; want 1/%d/0", fixture.writes, fixture.readbacks, fixture.lists, scenario.readbacks)
			}
		})
	}
}

func TestBatchAcknowledgementsReuseCompletedReadbacksForVerification(t *testing.T) {
	t.Parallel()
	fixture := &batchAcknowledgementFixture{reply: map[string]any{}}
	service, targets := newBatchAcknowledgementService(t, fixture, 2, true)
	result, err := service.Apply(t.Context(), targets, "test")
	if err != nil || result.Failed != 0 || result.Changed != 2 {
		t.Fatalf("fresh readbacks must confirm both accounts: %+v err=%v", result, err)
	}
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	if fixture.readbacks != 2 || fixture.lists != 0 {
		t.Fatalf("already confirmed accounts must not fetch the catalog again: readbacks=%d lists=%d", fixture.readbacks, fixture.lists)
	}
}

func TestBatchAcknowledgementsStillHonorExplicitVerification(t *testing.T) {
	t.Parallel()
	fixture := &batchAcknowledgementFixture{reply: map[string]any{"success_ids": []int{41, 42}}, skipApply: "42"}
	service, targets := newBatchAcknowledgementService(t, fixture, 2, true)
	result, err := service.Apply(t.Context(), targets, "test")
	if err != nil || result.Failed != 1 {
		t.Fatalf("verification must reject unchanged fields: %+v err=%v", result, err)
	}
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	if fixture.lists != 1 || fixture.readbacks != 0 {
		t.Fatalf("expected one bulk verification, lists=%d readbacks=%d", fixture.lists, fixture.readbacks)
	}
}
