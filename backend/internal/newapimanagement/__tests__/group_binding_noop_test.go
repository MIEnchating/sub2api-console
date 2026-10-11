package newapimanagement_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/newapimanagement"
)

type bindingRepository struct {
	newapimanagement.Repository
	items []business.NewAPIGroupBinding
}

func (r *bindingRepository) NewAPILocalGroups(context.Context) ([]business.NewAPILocalGroup, error) {
	ratio := "0.5"
	return []business.NewAPILocalGroup{{ID: "6", Name: "标准", Ratio: &ratio}}, nil
}
func (r *bindingRepository) ReplaceNewAPIGroupBindings(_ context.Context, _ string, items []business.NewAPIGroupBinding) error {
	r.items = items
	return nil
}
func (r *bindingRepository) NewAPIGroupBindings(context.Context, string) ([]business.NewAPIGroupBinding, error) {
	return r.items, nil
}

func TestSaveBindingsSkipsEquivalentRemoteRatioWithoutSkippingBindingSave(t *testing.T) {
	var writes atomic.Int32
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writes.Add(1)
			fmt.Fprint(w, `{"success":false,"message":"read-only upstream"}`)
			return
		}
		fmt.Fprint(w, `{"success":true,"data":{"GroupRatio":"{\"vip\":0.500,\"other\":2}"}}`)
	}))
	defer remote.Close()
	store, err := configstore.Open(filepath.Join(t.TempDir(), "private.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	_, err = store.SaveNewAPIPlatform(context.Background(), configstore.NewAPIPlatform{ID: "primary", Name: "测试", BaseURL: remote.URL, AdminKey: "test-key", UserID: "1"})
	if err != nil {
		t.Fatal(err)
	}
	repository := &bindingRepository{}
	service := newapimanagement.New(store, repository, remote.Client(), nil, nil)
	items, err := service.SaveBindings(context.Background(), "primary", []newapimanagement.GroupBindingInput{{NewAPIGroupID: "vip", NewAPIGroupName: "VIP", Sub2APIGroupID: "6", Sub2APIRatio: "0.5", SyncRatio: true}})
	if err != nil {
		t.Fatalf("unchanged ratio should not require write permission: %v", err)
	}
	if writes.Load() != 0 || len(items) != 1 || !items[0].SyncRatio {
		t.Fatalf("writes=%d bindings=%+v", writes.Load(), items)
	}
}
