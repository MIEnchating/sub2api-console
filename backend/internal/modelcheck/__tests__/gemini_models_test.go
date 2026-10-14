package modelcheck_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
)

func TestGeminiModelsFollowNativePaginationAndExcludeEmbeddingModels(t *testing.T) {
	for _, basePath := range []string{"", "/proxy/v1", "/proxy/v1beta"} {
		t.Run(basePath, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				prefix := ""
				if basePath != "" {
					prefix = "/proxy"
				}
				if r.URL.Path != prefix+"/v1beta/models" || r.Header.Get("x-goog-api-key") != fixtureSecret || r.Header.Get("Authorization") != "" || r.URL.Query().Get("pageSize") != "100" {
					t.Errorf("invalid list request: %s", r.URL)
				}
				if r.URL.Query().Get("pageToken") == "" {
					fmt.Fprint(w, `{"models":[{"name":"models/embedding","supportedGenerationMethods":["embedContent"]}],"nextPageToken":"page-2"}`)
				} else {
					if r.URL.Query().Get("pageToken") != "page-2" {
						t.Error("wrong cursor")
					}
					fmt.Fprint(w, `{"models":[{"name":"models/gemini-b","supportedGenerationMethods":["generateContent"]},{"name":"models/gemini-a","supportedGenerationMethods":["streamGenerateContent"]},{"name":"models/gemini-a","supportedGenerationMethods":["generateContent"]}]}`)
				}
			}))
			t.Cleanup(server.Close)
			models, err := new(modelcheck.Service).CustomAnimationModels(t.Context(), modelcheck.AnimationCustomEndpoint{BaseURL: server.URL + basePath, Platform: "gemini", APIKey: fixtureSecret})
			if err != nil || !reflect.DeepEqual(models, []string{"gemini-a", "gemini-b"}) {
				t.Fatalf("models=%v err=%v", models, err)
			}
		})
	}
}

func TestGeminiAccountModelsUseBoundCredential(t *testing.T) {
	f := setup(t, 1, "gemini", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-goog-api-key") != fixtureSecret || r.URL.Path != "/v1beta/models" {
			t.Error("wrong bound model request")
		}
		fmt.Fprint(w, `{"models":[{"name":"models/gemini-test","supportedGenerationMethods":["generateContent"]}]}`)
	})
	models, err := f.service.AccountAnimationModels(t.Context(), "1")
	if err != nil || !reflect.DeepEqual(models, []string{"gemini-test"}) {
		t.Fatalf("models=%v err=%v", models, err)
	}
}

func TestGeminiModelsRejectInvalidPages(t *testing.T) {
	for _, body := range []string{`{}`, `{"models":null}`, `{"models":[]} {}`, `{"models":[],"error":{"message":"denied"}}`, `{"models":[{"name":"models/../bad","supportedGenerationMethods":["generateContent"]}]}`, `{"models":[{"name":"models/gemini-test","supportedGenerationMethods":["generateContent"]}],"nextPageToken":"repeated"}`} {
		t.Run(body, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) }))
			t.Cleanup(server.Close)
			_, err := new(modelcheck.Service).CustomAnimationModels(t.Context(), modelcheck.AnimationCustomEndpoint{BaseURL: server.URL, Platform: "gemini", APIKey: fixtureSecret})
			if err == nil {
				t.Fatal("invalid native page accepted")
			}
		})
	}
}
