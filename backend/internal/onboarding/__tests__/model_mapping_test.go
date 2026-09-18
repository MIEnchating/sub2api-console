package onboarding_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/onboarding"
)

func TestModelMappingValidation(t *testing.T) {
	for _, tc := range []struct {
		name  string
		input map[string]string
		valid bool
	}{
		{"empty preserves defaults", nil, true},
		{"exact aliases", map[string]string{" alias ": " upstream ", "second": "upstream"}, true},
		{"empty source", map[string]string{" ": "upstream"}, false},
		{"empty target", map[string]string{"alias": " "}, false},
		{"duplicate trimmed source", map[string]string{"alias": "one", " alias ": "two"}, false},
		{"wildcard", map[string]string{"alias*": "upstream"}, false},
		{"whitespace", map[string]string{"alias": "up stream"}, false},
		{"control", map[string]string{"alias": "up\x00stream"}, false},
		{"long name", map[string]string{"alias": strings.Repeat("模", 257)}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := onboarding.NormalizeModelMapping(tc.input)
			if (err == nil) != tc.valid {
				t.Fatalf("mapping=%v err=%v", got, err)
			}
			if tc.name == "exact aliases" && !reflect.DeepEqual(got, map[string]string{"alias": "upstream", "second": "upstream"}) {
				t.Fatalf("not normalized: %v", got)
			}
		})
	}
}

func TestCustomModelMappingsReachCreatedAccountAndLocalCatalog(t *testing.T) {
	posts := 0
	admin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/admin/accounts/models/sync-upstream-preview" {
			_, _ = w.Write([]byte(`{"data":{"models":["gemini-2.5-flash","gemini-2.5-pro"]}}`))
			return
		}
		if r.URL.Path == "/api/v1/admin/accounts" && r.Method == http.MethodPost {
			posts++
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
				return
			}
			credentials := body["credentials"].(map[string]any)
			want := map[string]any{"gemini-2.5-flash": "gemini-2.5-pro", "gemini-2.5-pro": "gemini-2.5-pro", "alias": "gemini-2.5-flash"}
			if !reflect.DeepEqual(credentials["model_mapping"], want) {
				t.Errorf("mapping=%v", credentials["model_mapping"])
			}
			body["id"] = 77
			_ = json.NewEncoder(w).Encode(map[string]any{"data": body})
			return
		}
		http.NotFound(w, r)
	}))
	defer admin.Close()
	service, repo, _, request := newService(t, admin.URL, "http://127.0.0.1:1")
	request.ModelMapping = map[string]string{" alias ": "gemini-2.5-flash", "gemini-2.5-flash": "gemini-2.5-pro"}
	result, err := service.Onboard(context.Background(), request)
	if err != nil || posts != 1 || result["model_count"] != 3 {
		t.Fatalf("result=%v posts=%d err=%v", result, posts, err)
	}
	catalog, err := repo.AccountModelCatalogs(context.Background(), []string{"77"})
	if err != nil || len(catalog) != 1 || !reflect.DeepEqual(catalog[0].Models, []string{"alias", "gemini-2.5-flash", "gemini-2.5-pro"}) {
		t.Fatalf("catalog=%+v err=%v", catalog, err)
	}
}

func TestMappingChangeCannotResumePendingCreation(t *testing.T) {
	admin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "isolated model discovery failure", http.StatusBadRequest)
	}))
	defer admin.Close()
	service, _, keys, request := newService(t, admin.URL, "http://127.0.0.1:1")
	request.ModelMapping = map[string]string{"alias": "original"}
	if _, err := service.Onboard(context.Background(), request); err == nil {
		t.Fatal("expected discovery failure")
	}
	request.ModelMapping["alias"] = "changed"
	_, err := service.Onboard(context.Background(), request)
	if err == nil || !strings.Contains(err.Error(), "意图") || keys.creates != 1 || keys.reveals != 0 {
		t.Fatalf("changed intent must stop before key reuse: err=%v keys=%+v", err, keys)
	}
}
