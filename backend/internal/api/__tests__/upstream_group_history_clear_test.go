package api_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/api"
	"github.com/MIEnchating/sub2api-console/backend/internal/config"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

type historyClearStore struct {
	api.Business
	cleared    string
	clearedAll bool
}

func (s *historyClearStore) ClearUpstreamGroupHistoryForUpstream(_ context.Context, id string) (int64, error) {
	s.cleared = id
	return 2, nil
}
func (s *historyClearStore) ClearUpstreamGroupHistory(context.Context) (int64, error) {
	s.clearedAll = true
	return 3, nil
}

func TestClearHistoryScopesDeletionAndRejectsEmptyScope(t *testing.T) {
	for _, tc := range []struct {
		name, query, token, wantID string
		status                     int
	}{
		{"scoped", "/stable-one", "test-token", "stable-one", 200},
		{"unauthenticated", "/stable-one", "", "", 401},
		{"empty scope", "/%20", "test-token", "", 422},
	} {
		t.Run(tc.name, func(t *testing.T) {
			private, err := configstore.Open(filepath.Join(t.TempDir(), "private.db"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = private.Close() })
			store := &historyClearStore{}
			router := api.New(config.Config{AdminToken: "test-token"}, private, store)
			request := httptest.NewRequest(http.MethodDelete, "/api/upstreams/group-history"+tc.query, nil)
			if tc.token != "" {
				request.Header.Set("Authorization", "Bearer "+tc.token)
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != tc.status || store.cleared != tc.wantID || store.clearedAll {
				t.Fatalf("status=%d scoped=%q all=%v body=%s", response.Code, store.cleared, store.clearedAll, response.Body.String())
			}
		})
	}
}
