package upstreamsync_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/upstreamsync"
)

func TestReadCatalogOverlapsUserRatesAndKeyInventory(t *testing.T) {
	started := make(chan string, 2)
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/v1/groups/available":
			_, _ = writer.Write([]byte(`{"code":0,"data":[{"id":7,"name":"codex","rate":0.2}]}`))
		case "/api/v1/groups/rates":
			started <- request.URL.Path
			<-release
			_, _ = writer.Write([]byte(`{"code":0,"data":{"7":0.12}}`))
		case "/api/v1/keys":
			started <- request.URL.Path
			<-release
			_, _ = writer.Write([]byte(`{"code":0,"data":{"items":[{"id":41,"name":"key","group_id":7}],"total":1}}`))
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	token := "token"
	record := configstore.AuthRecord{BaseURL: server.URL, UpstreamType: "sub2api", AccessToken: &token}
	reader := upstreamsync.NewReader(server.Client())
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	result := make(chan error, 1)
	go func() {
		_, err := reader.ReadCatalog(ctx, record)
		result <- err
	}()
	for range 2 {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("catalog auxiliary reads did not start concurrently")
		}
	}
	close(release)
	if err := <-result; err != nil {
		t.Fatalf("ReadCatalog() error = %v", err)
	}
}
