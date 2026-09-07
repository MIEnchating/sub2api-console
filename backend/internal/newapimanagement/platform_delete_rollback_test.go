package newapimanagement

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

type cancelledDeletePrivate struct {
	*privateStub
	cancel context.CancelFunc
}

func (s *cancelledDeletePrivate) DeleteNewAPIPlatform(ctx context.Context, _ string) (bool, error) {
	s.cancel()
	return false, ctx.Err()
}

func TestDeletePlatformRestoresBindingsWhenPrivateDeleteIsCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	bindings := []business.NewAPIGroupBinding{{PlatformID: "platform-1", NewAPIGroupID: "vip", Sub2APIGroupID: "6"}}
	repository := contextAwareGroupRepository{&repositoryStub{bindings: bindings}}
	private := &cancelledDeletePrivate{privateStub: &privateStub{}, cancel: cancel}
	service := New(private, repository, nil, nil, nil)
	deleted, err := service.DeletePlatform(ctx, "platform-1")
	if deleted || !errors.Is(err, context.Canceled) {
		t.Fatalf("delete result=%v error=%v", deleted, err)
	}
	if !reflect.DeepEqual(repository.bindings, bindings) {
		t.Fatalf("cancelled platform delete lost bindings: %#v", repository.bindings)
	}
}
