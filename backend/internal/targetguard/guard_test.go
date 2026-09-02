package targetguard

import (
	"context"
	"errors"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

type targetStoreStub struct {
	target configstore.TargetSettings
}

func (store *targetStoreStub) TargetSettings(context.Context) (configstore.TargetSettings, error) {
	return store.target, nil
}

func TestBindRejectsChangedQueuedTargetIdentity(t *testing.T) {
	store := &targetStoreStub{target: configstore.TargetSettings{BaseURL: "https://a.example", AdminKey: "a", TimeoutSeconds: 5}}
	ctx, err := Capture(context.Background(), store)
	if err != nil {
		t.Fatal(err)
	}
	store.target = configstore.TargetSettings{BaseURL: "https://b.example", AdminKey: "b", TimeoutSeconds: 5}
	if _, err := Bind(ctx, store); !errors.Is(err, ErrChanged) {
		t.Fatalf("changed target bind error=%v", err)
	}
}

func TestBindPinsCurrentTimeoutForUnchangedIdentity(t *testing.T) {
	store := &targetStoreStub{target: configstore.TargetSettings{BaseURL: "https://a.example/", AdminKey: " key ", TimeoutSeconds: 5}}
	ctx, err := Capture(context.Background(), store)
	if err != nil {
		t.Fatal(err)
	}
	store.target = configstore.TargetSettings{BaseURL: "https://a.example", AdminKey: "key", TimeoutSeconds: 30}
	ctx, err = Bind(ctx, store)
	if err != nil {
		t.Fatal(err)
	}
	store.target = configstore.TargetSettings{BaseURL: "https://b.example", AdminKey: "other", TimeoutSeconds: 1}
	target, err := Settings(ctx, store)
	if err != nil {
		t.Fatal(err)
	}
	if target.BaseURL != "https://a.example" || target.TimeoutSeconds != 30 {
		t.Fatalf("pinned target=%#v", target)
	}
}

func TestExpectedPreservesUpstreamWorkflowTarget(t *testing.T) {
	wanted := configstore.TargetSettings{BaseURL: "https://a.example", AdminKey: "a", TimeoutSeconds: 5}
	store := &targetStoreStub{target: configstore.TargetSettings{BaseURL: "https://b.example", AdminKey: "b", TimeoutSeconds: 30}}
	target, err := Expected(Expect(context.Background(), wanted), store)
	if err != nil {
		t.Fatal(err)
	}
	if target != wanted {
		t.Fatalf("expected target=%#v want=%#v", target, wanted)
	}
}
