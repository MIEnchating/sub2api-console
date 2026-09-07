package upstreamsync

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func TestReadNewAPIKeyUsagePreservesLargeStableTokenIDs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/api/status" {
			_, _ = writer.Write([]byte(`{"success":true,"data":{"quota_per_unit":1,"enable_data_export":true}}`))
			return
		}
		_, _ = writer.Write([]byte(`{"success":true,"data":[{"token_id":9007199254740992,"quota":1},{"token_id":9007199254740993,"quota":2}]}`))
	}))
	defer server.Close()
	result, err := NewReader(server.Client()).ReadNewAPIKeyUsage(context.Background(), configstore.AuthRecord{BaseURL: server.URL, UpstreamType: "newapi"}, time.Unix(1, 0), time.Unix(2, 0))
	if err != nil || len(result.Keys) != 2 || result.Keys["9007199254740992"].Cost != "1" || result.Keys["9007199254740993"].Cost != "2" {
		t.Fatalf("distinct Token IDs merged during billing: result=%#v err=%v", result, err)
	}
}
