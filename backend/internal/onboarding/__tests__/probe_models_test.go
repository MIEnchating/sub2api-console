package onboarding_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestOnboardingPersistsRequestedProbeModelsForNewStableAccount(t *testing.T) {
	for name, models := range map[string][]string{"explicit models": {" gemini-2.5-flash ", "gemini-2.5-pro", "gemini-2.5-flash"}, "inherit defaults": {}} {
		t.Run(name, func(t *testing.T) {
			admin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/models/sync-upstream-preview") {
					_, _ = w.Write([]byte(`{"data":{"models":["gemini-2.5-flash","gemini-2.5-pro"]}}`))
					return
				}
				if r.Method == http.MethodPost && r.URL.Path == "/api/v1/admin/accounts" {
					var body map[string]any
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
						return
					}
					body["id"] = 77
					_ = json.NewEncoder(w).Encode(map[string]any{"data": body})
					return
				}
				http.NotFound(w, r)
			}))
			t.Cleanup(admin.Close)
			service, repo, _, request := newService(t, admin.URL, admin.URL)
			request.TestModels = models
			if _, err := service.Onboard(t.Context(), request); err != nil {
				t.Fatal(err)
			}
			account, err := repo.Account(t.Context(), "77")
			if err != nil {
				t.Fatal(err)
			}
			want := []string{}
			if len(models) > 0 {
				want = []string{"gemini-2.5-flash", "gemini-2.5-pro"}
			}
			if !reflect.DeepEqual(account.TestModels, want) {
				t.Fatalf("new account probe models=%v want=%v", account.TestModels, want)
			}
		})
	}
}

func TestOnboardingRejectsInvalidProbeModelsBeforeAnyRemoteWrite(t *testing.T) {
	for _, models := range [][]string{{" "}, {strings.Repeat("x", 257)}, make([]string, 21)} {
		service, _, keys, request := newService(t, "http://127.0.0.1:1", "http://127.0.0.1:1")
		request.TestModels = models
		_, err := service.Onboard(t.Context(), request)
		if err == nil || !strings.Contains(err.Error(), "探活模型") || keys.creates != 0 {
			t.Fatalf("invalid models reached remote work: %v creates=%d", err, keys.creates)
		}
	}
}
