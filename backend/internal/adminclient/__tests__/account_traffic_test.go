package adminclient_test

import (
	"context"
	"fmt"
	"github.com/MIEnchating/sub2api-console/backend/internal/adminclient"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestAccountTrafficValidatesLiveCounterContract(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		invalid    bool
	}{
		{"active and queued are distinct", `{"enabled":true,"timestamp":"2026-09-18T09:00:00Z","account":{"41":{"account_id":41,"current_in_use":2,"waiting_in_queue":7,"max_capacity":10}}}`, false},
		{"disabled", `{"enabled":false}`, false},
		{"missing counter", `{"enabled":true,"timestamp":"2026-09-18T09:00:00Z","account":{"41":{"account_id":41,"waiting_in_queue":7,"max_capacity":10}}}`, true},
		{"mismatched ID", `{"enabled":true,"timestamp":"2026-09-18T09:00:00Z","account":{"41":{"account_id":42,"current_in_use":2,"waiting_in_queue":0,"max_capacity":10}}}`, true},
		{"negative counter", `{"enabled":true,"timestamp":"2026-09-18T09:00:00Z","account":{"41":{"account_id":41,"current_in_use":-1,"waiting_in_queue":0,"max_capacity":10}}}`, true},
		{"missing accounts", `{"enabled":true,"timestamp":"2026-09-18T09:00:00Z"}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/v1/admin/ops/concurrency" {
					t.Errorf("path %s", r.URL.Path)
				}
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprintf(w, `{"data":%s}`, tc.body)
			}))
			defer server.Close()
			client, err := adminclient.New(adminclient.Config{BaseURL: server.URL, AdminKey: "fixture", Timeout: time.Second, Attempts: 1}, nil)
			if err != nil {
				t.Fatal(err)
			}
			snapshot, err := client.AccountTraffic(context.Background())
			if (err != nil) != tc.invalid {
				t.Fatalf("snapshot=%+v err=%v", snapshot, err)
			}
			if tc.name == "active and queued are distinct" && (len(snapshot.Accounts) != 1 || snapshot.Accounts[0].CurrentRequests != 2 || snapshot.Accounts[0].WaitingRequests != 7 || snapshot.Accounts[0].AccountID != "41") {
				t.Fatalf("wrong counters %+v", snapshot)
			}
		})
	}
}
