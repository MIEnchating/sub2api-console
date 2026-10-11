package uptimekuma_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/uptimekuma"
)

func metricsService(t testing.TB, baseURL string, client *http.Client) *uptimekuma.Service {
	t.Helper()
	store, err := configstore.Open(filepath.Join(t.TempDir(), "isolated-metrics.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.SaveUptimeKuma(t.Context(), configstore.UptimeKumaConfig{
		BaseURL: baseURL, APIKey: "isolated-metrics-key",
	}); err != nil {
		t.Fatal(err)
	}
	return uptimekuma.New(store, client)
}

func newMetricsFixture(t testing.TB, body string) *uptimekuma.Service {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, password, ok := r.BasicAuth()
		if r.URL.Path != "/metrics" || !ok || user != "" || password != "isolated-metrics-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(server.Close)
	return metricsService(t, server.URL, server.Client())
}
