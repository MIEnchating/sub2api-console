package adminclient

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"
)

func TestBatchMultiplierProbeRejectsIDThatCannotFitRequestInteger(t *testing.T) {
	var requests atomic.Int32
	client, server := testClient(t, 1, func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		writeJSON(w, `{"data":{"results":[]}}`)
	})
	defer server.Close()
	_, err := client.AccountUpstreamMultipliers(context.Background(), []string{"9223372036854775808"})
	if err == nil || requests.Load() != 0 {
		t.Fatalf("unrepresentable ID reached remote: err=%v requests=%d", err, requests.Load())
	}
}

func TestManagementCatalogRejectsEntriesWithoutStableNumericIDs(t *testing.T) {
	for _, route := range []string{"accounts", "groups", "legacy-groups"} {
		for _, item := range []string{`{"name":"missing"}`, `{"id":null}`, `{"id":-1}`} {
			t.Run(route+"/"+item, func(t *testing.T) {
				client, server := testClient(t, 1, func(w http.ResponseWriter, request *http.Request) {
					if route == "legacy-groups" && request.URL.Path == "/api/v1/admin/groups" {
						http.NotFound(w, request)
						return
					}
					writeJSON(w, `{"data":{"items":[`+item+`],"total":1}}`)
				})
				defer server.Close()
				var err error
				if route == "accounts" {
					_, err = client.Accounts(context.Background())
				} else {
					_, err = client.Groups(context.Background())
				}
				if err == nil {
					t.Fatal("catalog accepted entry without a valid stable ID")
				}
			})
		}
	}
}
