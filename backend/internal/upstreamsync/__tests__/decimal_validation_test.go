package upstreamsync_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/upstreamsync"
)

func TestBalanceRejectsNonDecimalAndUnboundedUpstreamNumbers(t *testing.T) {
	for _, value := range []string{"1/3", "0x10", "1_000", "1e1001"} {
		t.Run(value, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path == "/api/v1/user/profile" {
					_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]string{"balance": value}})
					return
				}
				_, _ = w.Write([]byte(`{"code":0,"data":{}}`))
			}))
			t.Cleanup(server.Close)
			result, err := upstreamsync.NewReader(server.Client()).ReadBalance(t.Context(), configstore.AuthRecord{BaseURL: server.URL, UpstreamType: "sub2api"})
			if err == nil || result.RawBalance != nil {
				t.Fatal("invalid upstream numeric syntax was accepted as a balance")
			}
		})
	}
}
