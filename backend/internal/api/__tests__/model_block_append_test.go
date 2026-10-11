package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountops"
	"github.com/MIEnchating/sub2api-console/backend/internal/api"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/config"
)

func TestModelBlockAppendPreservesConcurrentAdditionsAndDeduplicates(t *testing.T) {
	f := newAccountHealthFixture(t, nil)
	router := api.New(config.Config{AdminToken: "isolated-health-token"}, f.private, f.store, api.Dependencies{AccountModelSync: accountops.New(f.private, f.store, nil)})
	if err := f.store.SaveAccountModelExclusionList(t.Context(), []string{"legacy-*"}, "seed"); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	responses := make(chan *httptest.ResponseRecorder, 2)
	var workers sync.WaitGroup
	for _, model := range []string{"model-a", "model-b"} {
		workers.Go(func() {
			<-start
			request := httptest.NewRequest(http.MethodPost, "/api/config/model-sync/blocked-models", strings.NewReader(`{"model":"`+model+`"}`))
			request.Header.Set("Authorization", "Bearer isolated-health-token")
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			responses <- response
		})
	}
	close(start)
	workers.Wait()
	close(responses)
	for response := range responses {
		if response.Code != 200 {
			t.Fatalf("append: %d %s", response.Code, response.Body.String())
		}
	}
	settings, err := f.store.AccountModelSyncSettings(t.Context())
	if err != nil || !slices.Equal(settings.BlockedPatterns, []string{"legacy-*", "model-a", "model-b"}) {
		t.Fatalf("settings=%+v err=%v", settings, err)
	}
	for _, model := range []string{"MODEL-A", "legacy-hidden"} {
		request := httptest.NewRequest(http.MethodPost, "/api/config/model-sync/blocked-models", strings.NewReader(`{"model":"`+model+`"}`))
		request.Header.Set("Authorization", "Bearer isolated-health-token")
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		var actual business.AccountModelSyncSettings
		if err := json.Unmarshal(response.Body.Bytes(), &actual); err != nil || response.Code != 200 || !slices.Equal(actual.BlockedPatterns, settings.BlockedPatterns) {
			t.Fatalf("duplicate=%d %s", response.Code, response.Body.String())
		}
	}
	var count int
	if err := f.db.QueryRow(`SELECT count(*) FROM runtime_events WHERE event_type='account_model_policy.updated'`).Scan(&count); err != nil || count != 3 {
		t.Fatalf("audit count=%d err=%v", count, err)
	}
}

func TestModelBlockAppendValidatesAuthAndConcreteModel(t *testing.T) {
	f := newAccountHealthFixture(t, nil)
	router := api.New(config.Config{AdminToken: "isolated-health-token"}, f.private, f.store, api.Dependencies{AccountModelSync: accountops.New(f.private, f.store, nil)})
	for _, item := range []struct {
		name, body string
		auth       bool
		status     int
	}{
		{"unauthenticated", `{"model":"model-a"}`, false, 401},
		{"null", `null`, true, 422},
		{"empty", `{"model":""}`, true, 422},
		{"wildcard", `{"model":"*"}`, true, 422},
		{"whitespace", `{"model":"a b"}`, true, 422},
		{"long", `{"model":"` + strings.Repeat("模", 257) + `"}`, true, 422},
		{"unknown field", `{"model":"model-a","blocked_patterns":[]}`, true, 422},
	} {
		t.Run(item.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/api/config/model-sync/blocked-models", strings.NewReader(item.body))
			request.Header.Set("Content-Type", "application/json")
			if item.auth {
				request.Header.Set("Authorization", "Bearer isolated-health-token")
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != item.status {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
	var count int
	if err := f.db.QueryRow(`SELECT count(*) FROM runtime_events WHERE event_type='account_model_policy.updated'`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("audit count=%d err=%v", count, err)
	}
}
