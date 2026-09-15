package browserlogin_test

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin"
)

func TestWorkerRejectsRemovedUpstreamLoginRoutes(t *testing.T) {
	factory := &oauthFactoryFixture{browser: &oauthBrowserFixture{closed: make(chan struct{})}, options: make(chan browserlogin.OAuthOptions, 1)}
	socket := startOAuthWorkerSocket(t, factory)
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", socket)
	}}
	t.Cleanup(transport.CloseIdleConnections)
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second}

	for _, endpoint := range []struct{ method, path string }{
		{http.MethodPost, "/sessions"},
		{http.MethodGet, "/sessions/removed"},
		{http.MethodPost, "/sessions/removed/input"},
		{http.MethodPost, "/sessions/removed/credentials"},
		{http.MethodDelete, "/sessions/removed"},
	} {
		t.Run(endpoint.method+" "+endpoint.path, func(t *testing.T) {
			request, err := http.NewRequest(endpoint.method, "http://worker"+endpoint.path, strings.NewReader(`{"host":"login.example.test","base_url":"https://login.example.test"}`))
			if err != nil {
				t.Fatal(err)
			}
			response, err := client.Do(request)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			var value map[string]json.RawMessage
			if err := json.NewDecoder(response.Body).Decode(&value); err != nil {
				t.Fatal(err)
			}
			if response.StatusCode != http.StatusConflict || string(value["code"]) != `"session"` {
				t.Fatalf("removed upstream route is still handled: status %d, code %s", response.StatusCode, value["code"])
			}
			for _, field := range []string{"id", "image", "record"} {
				if _, exists := value[field]; exists {
					t.Errorf("removed route returned %s", field)
				}
			}
		})
	}

	browser, err := browserlogin.NewRemote(socket).OpenOAuth(context.Background(), validOAuthOptions("state-1"))
	if err != nil {
		t.Fatalf("removed upstream routes prevented workbench authorization: %v", err)
	}
	defer browser.Close()
	if image, err := browser.Screenshot(context.Background()); err != nil || string(image) != "oauth-frame" {
		t.Fatalf("workbench browser unavailable: %v", err)
	}
}
