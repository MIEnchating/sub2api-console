package management

import (
	"context"
	"errors"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAccountRateSyncReportsCancellationDuringWritesWithPartialResult(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/admin/accounts/upstream-billing-probe/batch":
			_, _ = w.Write([]byte(`{"data":{"results":[{"account_id":11,"snapshot":{"status":"ok","data":{"resolved_rate_multiplier":1.5}}}]}}`))
		case "/api/v1/admin/accounts":
			_, _ = w.Write([]byte(`{"data":{"items":[{"id":11,"name":"Relay-1.5","rate_multiplier":1.5}],"total":1}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	repository := &captureRepository{maintenance: []business.BoundAccountMaintenance{{AccountID: "11", AccountName: "Relay-1.5", UpstreamHost: "upstream.example", CurrentMultiplier: "1.5", RechargeRate: "10", NamingSiteName: "Relay", NamingBaseURL: "https://upstream.example"}}}
	writer := &captureRateWriter{beforeCheck: func(string) { cancel() }, errors: map[string]error{"11": context.Canceled}}
	service := New(staticTarget{value: configstore.TargetSettings{BaseURL: server.URL, AdminKey: "test", TimeoutSeconds: 1}}, repository, &memoryTasks{}, writer)
	result, err := service.syncAccountRates(ctx, []string{"11"}, "operator")
	if !errors.Is(err, context.Canceled) || result == nil {
		t.Fatalf("cancelled writes reported success or lost result: %#v err=%v", result, err)
	}
}
