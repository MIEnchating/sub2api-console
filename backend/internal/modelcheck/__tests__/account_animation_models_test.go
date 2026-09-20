package modelcheck_test

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
)

func TestAccountAnimationModelsUsesBoundKeyAndActualUpstream(t *testing.T) {
	for _, platform := range []string{"openai", "anthropic"} {
		t.Run(platform, func(t *testing.T) {
			f := setup(t, 1, platform, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/v1/models" {
					t.Error("model discovery must not synchronize management models or generate content")
				}
				key := r.Header.Get("x-api-key")
				if platform == "openai" {
					key = strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
				}
				if key != fixtureSecret {
					t.Error("discovery did not reuse the animation credential")
				}
				_, _ = w.Write([]byte(`{"data":[{"id":"actual-upstream-model"}]}`))
			})
			models, err := f.service.AccountAnimationModels(t.Context(), "1")
			if err != nil || !reflect.DeepEqual(models, []string{"actual-upstream-model"}) {
				t.Fatalf("models=%v err=%v", models, err)
			}
		})
	}
}

func TestAccountAnimationModelsRejectsMissingBindingBeforeNetwork(t *testing.T) {
	f := setup(t, 1, "openai", func(http.ResponseWriter, *http.Request) { t.Error("unexpected upstream request") })
	f.catalog.details["1"].Bindings = nil
	_, err := f.service.AccountAnimationModels(t.Context(), "1")
	if err == nil || !strings.Contains(err.Error(), "绑定") {
		t.Fatalf("missing binding accepted: %v", err)
	}
}

func TestAccountAnimationModelsPreservesUpstreamFailureWithoutDefaultModels(t *testing.T) {
	f := setup(t, 1, "openai", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"expired key"}}`))
	})
	models, err := f.service.AccountAnimationModels(t.Context(), "1")
	if err == nil || len(models) != 0 || !strings.Contains(err.Error(), "401") {
		t.Fatalf("upstream failure hidden: models=%v err=%v", models, err)
	}
}

func TestOAuthAnimationModelsUsesLiveManagedCatalogWithoutDefaultFallback(t *testing.T) {
	for _, failed := range []bool{false, true} {
		t.Run(map[bool]string{false: "live catalog", true: "discovery failure"}[failed], func(t *testing.T) {
			f := setup(t, 1, "openai", func(http.ResponseWriter, *http.Request) { t.Error("OAuth must not use API Key endpoint") })
			kind := "oauth"
			f.catalog.details["1"].AccountType = &kind
			f.catalog.details["1"].Bindings = nil
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == http.MethodGet && r.URL.Path == "/api/v1/admin/accounts/1":
					_, _ = w.Write([]byte(`{"data":{"id":1,"type":"oauth","platform":"openai"}}`))
				case r.Method == http.MethodPost && r.URL.Path == "/api/v1/admin/accounts/1/models/sync-upstream":
					if failed {
						w.WriteHeader(http.StatusBadRequest)
						_, _ = w.Write([]byte(`{"message":"OAuth catalog unavailable"}`))
						return
					}
					_, _ = w.Write([]byte(`{"data":{"models":["oauth-live-model"]}}`))
				default:
					t.Error("must not fall back to configured/default models")
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			t.Cleanup(server.Close)
			private := &oauthAccountStore{target: configstore.TargetSettings{BaseURL: server.URL, AdminKey: "isolated-admin-key", TimeoutSeconds: 2}}
			service, err := modelcheck.New(f.tasks, private, f.catalog, credentials{})
			if err != nil {
				t.Fatal(err)
			}
			models, err := service.AccountAnimationModels(t.Context(), "1")
			if failed {
				if err == nil || len(models) != 0 {
					t.Fatalf("failure hidden: %v %v", models, err)
				}
			} else if err != nil || !reflect.DeepEqual(models, []string{"oauth-live-model"}) {
				t.Fatalf("models=%v err=%v", models, err)
			}
		})
	}
}
