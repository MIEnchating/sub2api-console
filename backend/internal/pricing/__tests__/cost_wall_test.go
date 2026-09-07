package pricing_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/pricing"
	"github.com/MIEnchating/sub2api-console/backend/internal/routing"
	"github.com/MIEnchating/sub2api-console/backend/internal/runtimepolicy"
)

type testTarget struct{ settings configstore.TargetSettings }

func (target testTarget) TargetSettings(context.Context) (configstore.TargetSettings, error) {
	return target.settings, nil
}

func TestCostBlockedAccountMovesToFlagshipAndRecoversOnNextRoutingCalculation(t *testing.T) {
	ctx := context.Background()
	store, err := business.Open(filepath.Join(t.TempDir(), "pricing.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetMode(ctx, runtimepolicy.Full); err != nil {
		t.Fatal(err)
	}
	groupRows := []map[string]any{
		{"id": json.Number("8"), "name": "codex-pro-平价", "platform": "openai", "rate_multiplier": json.Number("0.2")},
		{"id": json.Number("24"), "name": "codex-pro-特价", "platform": "openai", "rate_multiplier": json.Number("0.15")},
		{"id": json.Number("25"), "name": "codex-pro-旗舰", "platform": "openai", "rate_multiplier": json.Number("0.25")},
	}
	_, err = store.SyncManagementSnapshot(ctx, []map[string]any{{
		"id": json.Number("104"), "name": "DeepSea API-0.21", "platform": "openai", "type": "apikey", "status": "active", "rate_multiplier": json.Number("0.21"), "schedulable": true, "group_ids": []any{json.Number("8")},
	}}, groupRows, "test")
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.PersistTrafficSamples(ctx, []business.TrafficSample{{AccountID: "104", GroupName: "codex-pro-平价", Result: "通过", SampleCount: 1, ObservedAt: time.Now().UTC().Format(time.RFC3339Nano), EvidenceKey: "pricing-recovery", Payload: map[string]any{"status_code": 200}}})
	if err != nil {
		t.Fatal(err)
	}
	engine := routing.NewService(store)
	before, err := engine.Calculate(ctx, routing.Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if target := before.AccountTargets["104"]; target.DesiredHealth != "cost_blocked" || target.Schedulable == nil || *target.Schedulable {
		t.Fatalf("before=%#v", target)
	}
	// Simulate the confirmed remote disable before pricing runs in a later round.
	_, err = store.SyncManagementSnapshot(ctx, []map[string]any{{"id": json.Number("104"), "schedulable": false}}, groupRows, "test")
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	groups := []int64{8}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/api/v1/admin/accounts/104" {
			http.Error(w, "unexpected target", 400)
			return
		}
		switch r.Method {
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{"id": 104, "name": "DeepSea API-0.21", "platform": "openai", "rate_multiplier": json.Number("0.21"), "group_ids": groups, "schedulable": false}})
		case http.MethodPut:
			var body struct {
				GroupIDs []int64 `json:"group_ids"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, err.Error(), 400)
				return
			}
			groups = body.GroupIDs
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{"id": 104}})
		default:
			http.Error(w, "unexpected method", 405)
		}
	}))
	defer server.Close()
	service := pricing.New(store, testTarget{configstore.TargetSettings{BaseURL: server.URL, AdminKey: "isolated-test-key", TimeoutSeconds: 5}}, nil)
	_, err = service.UpdateConfig(ctx, pricing.Config{Enabled: true, ProfitMargin: 0.25, ExchangeGroupSets: [][]string{{"8", "24", "25"}}, ExchangeGroupSetNames: []string{"pro 交换"}, IntervalSeconds: 3600, WriteConcurrency: 1}, "test")
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.ApplyNow(ctx, "test")
	if err != nil {
		t.Fatal(err)
	}
	if result.Changed != 1 || result.Failed != 0 {
		t.Fatalf("apply=%#v", result)
	}
	mu.Lock()
	remoteGroups := append([]int64{}, groups...)
	mu.Unlock()
	if !reflect.DeepEqual(remoteGroups, []int64{25}) {
		t.Fatalf("remote groups=%v", remoteGroups)
	}
	catalog, err := store.PricingCatalog(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(catalog.Accounts[0].GroupIDs, []string{"25"}) {
		t.Fatalf("local groups=%v", catalog.Accounts[0].GroupIDs)
	}
	after, err := engine.Calculate(ctx, routing.Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if target := after.AccountTargets["104"]; target.DesiredHealth == "cost_blocked" || target.Schedulable == nil || !*target.Schedulable {
		t.Fatalf("after=%#v", target)
	}
}
