package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/api"
	"github.com/MIEnchating/sub2api-console/backend/internal/config"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func TestDictionaryReorderRejectsMalformedRequestsWithoutChangingOrder(t *testing.T) {
	for _, invalid := range []string{"trailing_json", "unknown_field", "non_json_content_type"} {
		t.Run(invalid, func(t *testing.T) {
			private, err := configstore.Open(filepath.Join(t.TempDir(), "config.sqlite3"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = private.Close() })
			ctx := context.Background()
			before, err := private.ListDictionaries(ctx, "account_type")
			if err != nil {
				t.Fatal(err)
			}
			ids := make([]string, len(before))
			for index, entry := range before {
				ids[index] = entry.ID
			}
			slices.Reverse(ids)
			payload := map[string]any{"kind": "account_type", "ids": ids}
			if invalid == "unknown_field" {
				payload["unexpected"] = true
			}
			raw, err := json.Marshal(payload)
			if err != nil {
				t.Fatal(err)
			}
			body := string(raw)
			if invalid == "trailing_json" {
				body += `{}`
			}
			request := httptest.NewRequest(http.MethodPost, "/api/dictionaries/reorder", strings.NewReader(body))
			request.Header.Set("Authorization", "Bearer test-admin")
			request.Header.Set("Content-Type", "application/json")
			if invalid == "non_json_content_type" {
				request.Header.Set("Content-Type", "text/plain")
			}
			router := api.New(config.Config{AdminToken: "test-admin"}, private, nil)
			response := httptest.NewRecorder()

			router.ServeHTTP(response, request)

			if response.Code != http.StatusBadRequest {
				t.Errorf("malformed reorder returned %d: %s", response.Code, response.Body.String())
			}
			var failure struct {
				Code string `json:"code"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &failure); err != nil || failure.Code != "bad_request" {
				t.Errorf("missing stable error code: %s", response.Body.String())
			}
			after, err := private.ListDictionaries(ctx, "account_type")
			if err != nil {
				t.Fatal(err)
			}
			if !slices.EqualFunc(before, after, func(a, b configstore.DictionaryEntry) bool {
				return a.ID == b.ID && a.SortOrder == b.SortOrder && a.Version == b.Version
			}) {
				t.Fatal("malformed reorder changed dictionary order or versions")
			}
		})
	}
}
