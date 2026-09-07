package accountops

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/probe"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

type modelFinalTaskStore struct{ task taskstore.Task }

func (store *modelFinalTaskStore) Save(_ context.Context, task taskstore.Task) error {
	store.task = task
	return nil
}

func TestModelApplyReportsRemoteWriteWhenReadbackDoesNotMatch(t *testing.T) {
	repository, _, _ := accountRepository(t)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if request.Method == http.MethodPut {
			_, _ = writer.Write([]byte(`{"success":true}`))
			return
		}
		_, _ = writer.Write([]byte(`{"data":{"id":41,"credentials":{"model_mapping":{"old":"old"}}}}`))
	}))
	defer server.Close()
	tasks := &modelFinalTaskStore{}
	service := New(&testTarget{value: configstore.TargetSettings{BaseURL: server.URL, AdminKey: "test", TimeoutSeconds: 2}}, repository, tasks)
	item := service.applyAccountModels(context.Background(), business.AccountModelCatalog{AccountID: "41", AccountName: "alpha"}, []string{"new"}, "operator")
	if item.Status != "failed" {
		t.Fatalf("mismatched readback result=%#v", item)
	}
	task := taskstore.Task{ID: "apply-result", Result: map[string]any{}}
	service.finishModelApplyTask(context.Background(), &task, []string{"41"}, []modelSyncItem{item}, probe.RunSummary{}, "")
	if tasks.task.Result["remote_write"] != true {
		t.Fatalf("successful remote write was hidden after readback failure: %#v", tasks.task.Result)
	}
}

func TestModelTaskCancellationRetainsCompletedItemsAndRemoteWriteEvidence(t *testing.T) {
	tasks := &modelFinalTaskStore{}
	service := &Service{tasks: tasks}
	items := []modelSyncItem{{AccountID: "41", Status: "succeeded", ModelCount: 1}}
	task := taskstore.Task{ID: "interrupted-apply", Result: map[string]any{"account_ids": []string{"41", "42"}, "completed": 1, "total": 2, "items": items, "remote_write": true}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	service.failModelSyncTask(ctx, &task, []string{"41", "42"}, errors.New("application interrupted"))
	completedItems, ok := tasks.task.Result["items"].([]modelSyncItem)
	if tasks.task.Status != "cancelled" || tasks.task.Result["remote_write"] != true || !ok || len(completedItems) != 1 || completedItems[0].AccountID != "41" {
		t.Fatalf("cancellation discarded completed mutation evidence: %#v", tasks.task)
	}
}
