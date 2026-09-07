package management

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func TestBaseURLSyncReportsRemoteWriteWhenReadbackDisagrees(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"id":11,"credentials":{"base_url":"https://old.example"}}}`))
	}))
	defer server.Close()
	repository := &captureRepository{maintenance: []business.BoundAccountMaintenance{{AccountID: "11", UpstreamHost: "upstream.example", NamingBaseURL: "https://relay.example"}}}
	service := New(staticTarget{value: configstore.TargetSettings{BaseURL: server.URL, AdminKey: "test", TimeoutSeconds: 1}}, repository, &memoryTasks{})
	result, err := service.syncAccountBaseURLs(context.Background(), []string{"11"}, "operator")
	if err == nil || result["remote_write"] != true {
		t.Fatalf("failed readback hid remote write: result=%#v error=%v", result, err)
	}
}
