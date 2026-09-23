package management_test

import (
	"context"
	"encoding/json"
	"github.com/MIEnchating/sub2api-console/backend/internal/accountops"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/management"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

func TestManualRateSynchronizationUpdatesNameAndInvokesCostReconciliationAfterReadback(t *testing.T) {
	store, _ := rateCollectionStore(t)
	if _, err := store.AssignManualPriority(t.Context(), "11", 1, "10", 10, true, "test"); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	row := map[string]any{"id": 11, "name": "Relay-1", "rate_multiplier": 1}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/admin/accounts/upstream-billing-probe/batch":
			_, _ = w.Write([]byte(`{"data":{"results":[{"account_id":11,"snapshot":{"status":"ok","data":{"resolved_rate_multiplier":0.32}}}]}}`))
		case "/api/v1/admin/accounts":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"items": []any{row}, "total": 1}})
		case "/api/v1/admin/accounts/11":
			if r.Method == http.MethodPut {
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if len(body) != 2 || body["name"] != "Relay-0.32" || body["rate_multiplier"] != 0.32 {
					t.Errorf("unexpected rate write: %v", body)
				}
				for key, value := range body {
					row[key] = value
				}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": row})
		default:
			http.Error(w, "unexpected endpoint", 400)
		}
	}))
	defer server.Close()
	target := &rateCollectionTarget{settings: configstore.TargetSettings{BaseURL: server.URL, AdminKey: "test-key", TimeoutSeconds: 5}}
	service := management.New(target, store, nil, accountops.New(target, store, nil))
	reconciled := false
	service.UseRateReconciliation(func(ctx context.Context, ids []string, actor string) error {
		if len(ids) != 1 || ids[0] != "11" {
			t.Fatalf("wrong reconciliation scope: %v", ids)
		}
		account, err := store.Account(ctx, "11")
		if err != nil {
			return err
		}
		if account.Name != "Relay-0.32" || account.Multiplier == nil || *account.Multiplier != "0.32" {
			t.Fatalf("reconciliation preceded readback: %+v", account)
		}
		reconciled = true
		return nil
	})
	result, err := service.SyncAllAccountRates(t.Context(), "test")
	if err != nil || result["updated"] != 1 || !reconciled {
		t.Fatalf("result=%v err=%v reconciled=%v", result, err, reconciled)
	}
}
