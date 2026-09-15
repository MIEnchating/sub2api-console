package adminclient_test

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/adminclient"
)

func TestBillingAndProbeRejectNonDecimalOrUnboundedResponses(t *testing.T) {
	for _, value := range []string{"1/3", "0x10", "1_000", "1e1001"} {
		for _, operation := range []string{"billing", "probe"} {
			t.Run(operation+"/"+value, func(t *testing.T) {
				payload := map[string]any{"data": map[string]any{"total_account_cost": value, "total_actual_cost": "1"}}
				if operation == "probe" {
					payload = map[string]any{"data": map[string]any{"snapshot": map[string]any{"status": "ok", "data": map[string]string{"resolved_rate_multiplier": value}}}}
				}
				body, err := json.Marshal(payload)
				if err != nil {
					t.Fatal(err)
				}
				client, err := adminclient.New(adminclient.Config{BaseURL: "https://management.test", AdminKey: "test-only", Attempts: 1}, responseTransport(func(request *http.Request) (*http.Response, error) {
					return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(string(body))), Header: make(http.Header), Request: request}, nil
				}))
				if err != nil {
					t.Fatal(err)
				}
				if operation == "billing" {
					_, err = client.AccountUsageTotals(t.Context(), "41", "2026-09-14", "UTC")
				} else {
					_, err = client.AccountUpstreamMultiplier(t.Context(), "41")
				}
				if err == nil {
					t.Fatal("invalid numeric response was accepted")
				}
			})
		}
	}
}

func TestGroupMultiplierRejectsFractionBeforeSendingWrite(t *testing.T) {
	requested := false
	client, err := adminclient.New(adminclient.Config{BaseURL: "https://management.test", AdminKey: "test-only", Attempts: 1}, responseTransport(func(request *http.Request) (*http.Response, error) {
		requested = true
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"data":{"id":1,"rate_multiplier":"0.5"}}`)), Header: make(http.Header), Request: request}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.UpdateGroupRateMultiplier(t.Context(), "1", "1/2"); err == nil || requested {
		t.Fatal("fractional syntax reached a remote multiplier write")
	}
}
