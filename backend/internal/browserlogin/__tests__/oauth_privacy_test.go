package browserlogin_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin"
)

func oauthWireSession(t *testing.T) (*http.Client, string, <-chan struct{}) {
	t.Helper()
	fixture := &oauthBrowserFixture{closed: make(chan struct{}), result: browserlogin.OAuthResult{Code: "private-code", State: "state-1"}}
	socket := startOAuthWorkerSocket(t, &oauthFactoryFixture{browser: fixture, options: make(chan browserlogin.OAuthOptions, 1)})
	client := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", socket)
	}}}
	t.Cleanup(client.CloseIdleConnections)
	options, err := json.Marshal(validOAuthOptions("state-1"))
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Post("http://worker/oauth/sessions", "application/json", bytes.NewReader(options))
	if err != nil {
		t.Fatal(err)
	}
	var opened struct {
		ID string `json:"id"`
	}
	err = json.NewDecoder(response.Body).Decode(&opened)
	response.Body.Close()
	if err != nil || response.StatusCode != http.StatusOK || opened.ID == "" {
		t.Fatalf("OAuth session failed to open: %v", err)
	}
	return client, opened.ID, fixture.closed
}

func TestOAuthScreenshotResponseExcludesAuthorizationMaterialAndDisablesCaching(t *testing.T) {
	client, id, _ := oauthWireSession(t)
	response, err := client.Get("http://worker/oauth/sessions/" + id)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("OAuth screenshot failed: %v", err)
	}
	if bytes.Contains(body, []byte("private-code")) || bytes.Contains(body, []byte("state-1")) || bytes.Contains(body, []byte(`"oauth"`)) {
		t.Fatal("screenshot response contains authorization material")
	}
	if response.Header.Get("Cache-Control") != "no-store" {
		t.Fatal("OAuth screenshot response is cacheable")
	}
}

func TestRegularSessionNamespaceCannotDeleteOAuthBrowser(t *testing.T) {
	client, id, closed := oauthWireSession(t)
	request, err := http.NewRequest(http.MethodDelete, "http://worker/sessions/"+id, nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode == http.StatusOK {
		t.Fatal("regular session namespace accepted OAuth deletion")
	}
	select {
	case <-closed:
		t.Fatal("wrong session namespace closed OAuth browser")
	default:
	}
}

func TestUnknownOAuthSessionCannotReadScreenshotOrAuthorizationCode(t *testing.T) {
	client, _, _ := oauthWireSession(t)
	for _, operation := range []struct{ method, path string }{
		{method: http.MethodGet, path: "/oauth/sessions/wrong-id"},
		{method: http.MethodPost, path: "/oauth/sessions/wrong-id/authorization-code"},
	} {
		t.Run(operation.path, func(t *testing.T) {
			request, err := http.NewRequest(operation.method, "http://worker"+operation.path, nil)
			if err != nil {
				t.Fatal(err)
			}
			response, err := client.Do(request)
			if err != nil {
				t.Fatal(err)
			}
			response.Body.Close()
			if response.StatusCode == http.StatusOK {
				t.Fatal("unknown OAuth session accepted")
			}
		})
	}
}
