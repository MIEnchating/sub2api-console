package routingwrite_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/routingwrite"
)

type failingAuthorizationStore struct {
	*business.Store
	failPolicyRead atomic.Bool
	failSkipAudit  bool
}

func (store *failingAuthorizationStore) ControlPolicy(ctx context.Context) (map[string]any, error) {
	if store.failPolicyRead.Load() {
		return nil, errors.New("策略存储读取失败")
	}
	return store.Store.ControlPolicy(ctx)
}

func (store *failingAuthorizationStore) RecordAccountOperation(ctx context.Context, op business.AccountOperation) error {
	if store.failSkipAudit && op.State == "skipped" {
		return errors.New("审计存储不可用")
	}
	return store.Store.RecordAccountOperation(ctx, op)
}

func TestAuthorizationReadFailurePreventsWriteAndRemainsFailureAlert(t *testing.T) {
	_, fixture := newUpstreamCapacityWriteFixture(t, writeCapacityPointer(20), 4, 4, true)
	repository := &failingAuthorizationStore{Store: fixture.store}
	var mutations atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodGet && request.URL.Path == "/api/v1/admin/accounts/41" {
			repository.failPolicyRead.Store(true)
		}
		if request.Method != http.MethodGet {
			mutations.Add(1)
		}
		fixture.ServeHTTP(w, request)
	}))
	t.Cleanup(server.Close)
	desired := int64(6)
	result, err := routingwrite.New(routingTarget{url: server.URL}, repository).Apply(t.Context(), map[string]business.AccountRoutingTarget{
		"41": {AccountID: "41", GroupNames: []string{"capacity-group"}, Concurrency: &desired},
	}, "scheduler")
	if err != nil || result.Failed != 1 || result.RemoteWrite || mutations.Load() != 0 || len(result.Results) != 1 {
		t.Fatalf("policy read failure must stop the write and report failure: %+v err=%v", result, err)
	}
	item := result.Results[0]
	if item.Skipped || item.Error == nil || *item.Error != "策略存储读取失败" {
		t.Fatalf("policy read failure was treated as an ordinary deferral: %+v", item)
	}
	if _, err := fixture.store.EvaluateAlertIncidents(t.Context()); err != nil {
		t.Fatal(err)
	}
	queue, err := fixture.store.NotificationQueueDetails(t.Context(), "test-channel", true)
	if err != nil || len(queue.ProducerFiring) != 1 || queue.ProducerFiring[0].EventType != "routing.apply_failure" {
		t.Fatalf("policy read failure must still generate an alert: %+v err=%v", queue, err)
	}
}

func TestPolicyDeferralAuditFailureRemainsExecutionFailure(t *testing.T) {
	_, fixture := newUpstreamCapacityWriteFixture(t, writeCapacityPointer(20), 4, 4, true)
	repository := &failingAuthorizationStore{Store: fixture.store, failSkipAudit: true}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodGet && request.URL.Path == "/api/v1/admin/accounts/41" {
			if _, err := fixture.store.UpdatePolicy(request.Context(), map[string]any{"auto_apply": map[string]any{"concurrency": false}}, "operator"); err != nil {
				t.Error(err)
				http.Error(w, "policy update failed", http.StatusInternalServerError)
				return
			}
		}
		fixture.ServeHTTP(w, request)
	}))
	t.Cleanup(server.Close)
	desired := int64(6)
	result, err := routingwrite.New(routingTarget{url: server.URL}, repository).Apply(t.Context(), map[string]business.AccountRoutingTarget{
		"41": {AccountID: "41", GroupNames: []string{"capacity-group"}, Concurrency: &desired},
	}, "scheduler")
	if err != nil || result.Failed != 1 || result.RemoteWrite || len(result.Results) != 1 {
		t.Fatalf("audit failure must remain an execution failure: %+v err=%v", result, err)
	}
	if item := result.Results[0]; item.Skipped || item.Error == nil || *item.Error != "保存调度跳过记录失败：审计存储不可用" {
		t.Fatalf("audit failure was hidden by the policy deferral: %+v", item)
	}
}
