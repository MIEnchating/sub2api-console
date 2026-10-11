package accountautomation_test

import (
	"context"
	"testing"
)

func TestEnqueueResponseRemainsQueuedAfterWorkerCompletes(t *testing.T) {
	service, _, _, store, request := automationFixture()
	response, err := service.Enqueue(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if store.items[response.ID].Status != "succeeded" {
		t.Fatal("worker did not complete")
	}
	if response.Status != "queued" || response.Result["phase"] != "queued" {
		t.Fatalf("worker mutated the enqueue response: %#v", response)
	}
}
