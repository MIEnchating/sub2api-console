package accountops

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func TestPoolModeSyncSkipsManuallyProtectedAccountBeforeRemoteAccess(t *testing.T) {
	repository, _, _ := accountRepository(t)
	if _, err := repository.AssignManualPriority(context.Background(), "41", 3, "100", 100, true, "operator"); err != nil {
		t.Fatal(err)
	}
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		http.Error(writer, "unexpected protected account access", http.StatusBadRequest)
	}))
	defer server.Close()
	service := New(&testTarget{value: configstore.TargetSettings{BaseURL: server.URL, AdminKey: "test", TimeoutSeconds: 2}}, repository, nil)
	item := service.syncPoolModeAccount(context.Background(), "41", configstore.AccountCreationSettings{}, "operator")
	if item.Status != "skipped" || !strings.Contains(item.Error, "人工优先位") || requests.Load() != 0 {
		t.Fatalf("protected pool-mode sync=%#v requests=%d", item, requests.Load())
	}
}
