package newapimanagement

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

type contextAwareGroupRepository struct{ *repositoryStub }

func (repository contextAwareGroupRepository) UpdateNewAPILocalGroupRatio(ctx context.Context, id, ratio string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return repository.repositoryStub.UpdateNewAPILocalGroupRatio(ctx, id, ratio)
}
func (repository contextAwareGroupRepository) ReplaceNewAPIGroupBindings(ctx context.Context, id string, items []business.NewAPIGroupBinding) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return repository.repositoryStub.ReplaceNewAPIGroupBindings(ctx, id, items)
}

func TestSaveBindingsRestoresAllRatiosAfterNewAPICommitsButResponseFails(t *testing.T) {
	for _, cancelRequest := range []bool{false, true} {
		name := "failed response"
		if cancelRequest {
			name = "cancelled request"
		}
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var mu sync.Mutex
			managementRatio, newAPIRatio := "0.35", `{"vip":1}`
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path == "/api/option/" {
					if r.Method == http.MethodGet {
						_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]string{"GroupRatio": newAPIRatio}})
						return
					}
					var body struct {
						Value string `json:"value"`
					}
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
						w.WriteHeader(400)
						return
					}
					newAPIRatio = body.Value
					if body.Value == `{"vip":0.42}` {
						if cancelRequest {
							cancel()
						}
						w.WriteHeader(503)
						_, _ = w.Write([]byte(`{"success":false,"error":"response lost after commit"}`))
						return
					}
					_, _ = w.Write([]byte(`{"success":true}`))
					return
				}
				if r.URL.Path == "/api/v1/admin/groups/6" {
					if r.Method == http.MethodPut {
						var body map[string]json.Number
						if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
							t.Error(err)
							w.WriteHeader(400)
							return
						}
						managementRatio = body["rate_multiplier"].String()
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{"id": 6, "rate_multiplier": json.Number(managementRatio)}})
					return
				}
				http.NotFound(w, r)
			}))
			defer server.Close()
			ratio := "0.35"
			repository := contextAwareGroupRepository{&repositoryStub{groups: []business.NewAPILocalGroup{{ID: "6", Name: "测试分组", Ratio: &ratio}}}}
			private := &privateStub{platform: configstore.NewAPIPlatform{ID: "platform-1", BaseURL: server.URL, AdminKey: "test-key", UserID: "1"}, target: configstore.TargetSettings{BaseURL: server.URL, AdminKey: "test-key", TimeoutSeconds: 2}}
			service := New(private, repository, server.Client(), nil, nil)
			_, err := service.SaveBindings(ctx, "platform-1", []GroupBindingInput{{NewAPIGroupID: "vip", NewAPIGroupName: "VIP", Sub2APIGroupID: "6", Sub2APIRatio: "0.42", SyncRatio: true}})
			if err == nil {
				t.Fatal("uncertain write must report failure")
			}
			mu.Lock()
			defer mu.Unlock()
			if managementRatio != "0.35" || newAPIRatio != `{"vip":1}` || *repository.groups[0].Ratio != "0.35" {
				t.Fatalf("ratios after failed save: management=%s NewAPI=%s local=%s", managementRatio, newAPIRatio, *repository.groups[0].Ratio)
			}
		})
	}
}
