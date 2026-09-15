package adminclient_test

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/adminclient"
)

func TestMultiplierProbeErrorsRedactUpstreamCredentials(t *testing.T) {
	for _, item := range []string{
		`{"error":"upstream rejected api_key=private-upstream-key"}`,
		`{"snapshot":{"status":"failed","last_error":"Authorization: Bearer private-upstream-key"}}`,
	} {
		t.Run(item, func(t *testing.T) {
			client, err := adminclient.New(adminclient.Config{BaseURL: "https://management.test", AdminKey: "test-only", Attempts: 1}, responseTransport(func(request *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"data":` + item + `}`)), Header: make(http.Header), Request: request}, nil
			}))
			if err != nil {
				t.Fatal(err)
			}
			_, err = client.AccountUpstreamMultiplier(context.Background(), "41")
			if err == nil || strings.Contains(err.Error(), "private-upstream-key") {
				t.Fatalf("probe failure exposed upstream credential: %v", err)
			}
		})
	}
}
