package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/config"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

type dictionaryBusiness struct {
	fakeBusiness
	err error
}

func TestBuiltInDictionaryAPIProvidesDefaultsAndRejectsIncompleteReorder(t *testing.T) {
	router, _ := testRouter(t, config.Config{AdminToken: "test-token"}, dictionaryBusiness{err: errors.New("built-in dictionaries must not query accounts")})
	response := authenticatedRequest(t, router, http.MethodGet, "/api/dictionaries?kind=account_type", nil)
	var body struct {
		Items []configstore.DictionaryEntry `json:"items"`
	}
	if response.Code != http.StatusOK {
		t.Fatalf("status %d: %s", response.Code, response.Body.String())
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Items) < 2 {
		t.Fatalf("missing built-in entries: %+v", body.Items)
	}
	response = authenticatedRequest(t, router, http.MethodPost, "/api/dictionaries/reorder", map[string]any{"kind": "account_type", "ids": []string{body.Items[0].ID}})
	if response.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", response.Code, response.Body.String())
	}
}

func TestGroupDictionaryDoesNotMatchAnUnboundNameToAnID(t *testing.T) {
	id := "10"
	rows := []business.GroupStatus{{Name: "10"}, {ID: &id, Name: "已绑定分组"}}
	sortGroupsByDictionary(rows, []configstore.DictionaryEntry{{Value: "10", Enabled: true}})
	if rows[0].ID == nil {
		t.Fatal("unbound group name matched a stable ID")
	}
}

func (f dictionaryBusiness) DictionaryValues(context.Context, string) ([]configstore.DictionaryEntry, error) {
	return []configstore.DictionaryEntry{{Name: "OpenAI", Value: "openai"}}, f.err
}

func (f dictionaryBusiness) Accounts(context.Context) ([]business.AccountStatus, error) {
	return nil, errors.New("full account projection must not run for dictionary reads")
}

func TestDictionaryReadUsesCatalogAndKeepsCacheOnFailure(t *testing.T) {
	for _, failed := range []bool{false, true} {
		t.Run(map[bool]string{false: "catalog", true: "catalog_failure"}[failed], func(t *testing.T) {
			source := dictionaryBusiness{}
			if failed {
				source.err = errors.New("catalog unavailable")
			}
			router, store := testRouter(t, config.Config{AdminToken: "test-token"}, source)
			saved, err := store.SaveDictionary(context.Background(), configstore.DictionaryEntry{Kind: "platform", Name: "OpenAI", Value: "openai", Enabled: true}, 0)
			if err != nil {
				t.Fatal(err)
			}
			response := authenticatedRequest(t, router, http.MethodGet, "/api/dictionaries?kind=platform", nil)
			expected := http.StatusOK
			if failed {
				expected = http.StatusInternalServerError
			}
			if response.Code != expected {
				t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
			}
			items, err := store.ListDictionaries(context.Background(), "platform")
			if err != nil || len(items) != 1 || items[0].ID != saved.ID {
				t.Fatalf("dictionary lost: %v, %v", items, err)
			}
		})
	}
}
