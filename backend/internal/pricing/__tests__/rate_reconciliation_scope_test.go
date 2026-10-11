package pricing_test

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/pricing"
	"github.com/MIEnchating/sub2api-console/backend/internal/runtimepolicy"
)

func rateReconciliationStore(t testing.TB) (*business.Store, *sql.DB) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "rate-scope.db")
	store, err := business.Open(path)
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
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`INSERT INTO accounts(id,name,multiplier,metadata_json,updated_at) VALUES
		('41','confirmed','0.21','{"platform":"openai"}','now'),('42','unconfirmed','0.21','{"platform":"openai"}','now'),
		('43','manual','0.21','{"platform":"openai"}','now'),('44','locked','0.21','{"platform":"openai"}','now');
		INSERT INTO local_groups(name,remote_id,platform,rate_multiplier,updated_at) VALUES
		('standard','8','openai','0.2','now'),('premium','9','openai','0.3','now');
		INSERT INTO account_groups(account_id,group_name,group_id) SELECT id,'standard','8' FROM accounts;
		INSERT INTO manual_priority_accounts(account_id,priority,created_at,updated_at) VALUES('43',1,'now','now')`); err != nil {
		t.Fatal(err)
	}
	if err := store.SetAccountGroupsLocked(t.Context(), "44", true, "test"); err != nil {
		t.Fatal(err)
	}
	_, err = store.UpdatePolicy(t.Context(), map[string]any{"advanced_policy": map[string]any{"price_management": map[string]any{
		"enabled": true, "profit_margin": 0.2, "exchange_group_sets": []any{[]any{"8", "9"}},
	}}}, "test")
	if err != nil {
		t.Fatal(err)
	}
	return store, db
}

func TestRateReconciliationRejectsInvalidConfirmedIDs(t *testing.T) {
	store, _ := rateReconciliationStore(t)
	service := pricing.New(store, testTarget{configstore.TargetSettings{BaseURL: "https://management.invalid", AdminKey: "isolated-test-key"}}, nil)
	for _, id := range []string{"0", "041", "41 OR 1=1"} {
		if err := service.ReconcileAccountRates(t.Context(), []string{id}, "test"); err == nil {
			t.Fatalf("invalid confirmed account ID %q accepted", id)
		}
	}
}

func TestRateReconciliationWritesOnlyConfirmedAccountsAndPreservesProtectedAccounts(t *testing.T) {
	store, _ := rateReconciliationStore(t)
	var mu sync.Mutex
	remoteGroups := []int64{8}
	writes := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/api/v1/admin/accounts/41" {
			t.Errorf("request escaped confirmed unprotected account: %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected account", http.StatusBadRequest)
			return
		}
		if r.Method == http.MethodPut {
			var input struct {
				GroupIDs []int64 `json:"group_ids"`
			}
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
				http.Error(w, "invalid groups", http.StatusBadRequest)
				return
			}
			remoteGroups = input.GroupIDs
			writes++
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{
			"id": 41, "platform": "openai", "rate_multiplier": json.Number("0.21"), "group_ids": remoteGroups,
		}})
	}))
	t.Cleanup(server.Close)
	service := pricing.New(store, testTarget{configstore.TargetSettings{BaseURL: server.URL, AdminKey: "isolated-test-key", TimeoutSeconds: 5}}, nil)
	if err := service.ReconcileAccountRates(t.Context(), []string{"44", "41", "43", "41", "99"}, "test"); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	actualWrites, actualGroups := writes, append([]int64{}, remoteGroups...)
	mu.Unlock()
	if actualWrites != 1 || !reflect.DeepEqual(actualGroups, []int64{9}) {
		t.Fatalf("remote writes=%d groups=%v", actualWrites, actualGroups)
	}
	catalog, err := store.PricingCatalog(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, account := range catalog.Accounts {
		want := []string{"8"}
		if account.ID == "41" {
			want = []string{"9"}
		}
		if !reflect.DeepEqual(account.GroupIDs, want) {
			t.Fatalf("account %s groups=%v want=%v", account.ID, account.GroupIDs, want)
		}
	}
}

func TestRateReconciliationKeepsGlobalExchangeGroupValidationWhenAccountIsMissing(t *testing.T) {
	store, db := rateReconciliationStore(t)
	if _, err := db.Exec(`DELETE FROM local_groups WHERE remote_id='9'`); err != nil {
		t.Fatal(err)
	}
	service := pricing.New(store, testTarget{configstore.TargetSettings{BaseURL: "https://management.invalid", AdminKey: "isolated-test-key"}}, nil)
	if err := service.ReconcileAccountRates(t.Context(), []string{"99"}, "test"); err == nil || !strings.Contains(err.Error(), "互换组") {
		t.Fatalf("missing account bypassed exchange catalog validation: %v", err)
	}
}

func TestRateReconciliationWhenConfirmedCostChangedDoesNotWriteOrSyncGroups(t *testing.T) {
	store, _ := rateReconciliationStore(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("changed cost caused a remote write: %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{
			"id": 41, "platform": "openai", "rate_multiplier": json.Number("0.4"), "group_ids": []int64{8},
		}})
	}))
	t.Cleanup(server.Close)
	service := pricing.New(store, testTarget{configstore.TargetSettings{BaseURL: server.URL, AdminKey: "isolated-test-key", TimeoutSeconds: 5}}, nil)
	if err := service.ReconcileAccountRates(t.Context(), []string{"41"}, "test"); err == nil {
		t.Fatal("changed confirmed cost must fail migration")
	}
	catalog, err := store.PricingCatalogForAccount(t.Context(), "41")
	if err != nil || len(catalog.Accounts) != 1 || !reflect.DeepEqual(catalog.Accounts[0].GroupIDs, []string{"8"}) {
		t.Fatalf("failed migration changed local groups: %+v err=%v", catalog, err)
	}
}

func BenchmarkRateReconciliationWithUnrelatedAccounts(b *testing.B) {
	store, db := rateReconciliationStore(b)
	metadata := `{"platform":"openai","notes":"` + strings.Repeat("fixture ", 1024) + `"}`
	if _, err := db.Exec(`UPDATE accounts SET multiplier='0.1' WHERE id='41'`); err != nil {
		b.Fatal(err)
	}
	if _, err := db.Exec(`WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<1000)
		INSERT INTO accounts(id,name,multiplier,metadata_json,updated_at) SELECT CAST(x AS TEXT),'fixture','0.21',?,'now' FROM n WHERE x NOT IN (41,42,43,44)`, metadata); err != nil {
		b.Fatal(err)
	}
	service := pricing.New(store, testTarget{configstore.TargetSettings{BaseURL: "https://management.invalid", AdminKey: "isolated-test-key"}}, nil)
	b.ReportAllocs()
	for b.Loop() {
		if err := service.ReconcileAccountRates(b.Context(), []string{"41"}, "benchmark"); err != nil {
			b.Fatal(err)
		}
	}
}
