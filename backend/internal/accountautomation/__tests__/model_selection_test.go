package accountautomation_test

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountautomation"
)

func TestOnlyExplicitlySelectedModelsArePublished(t *testing.T) {
	service, accounts, management, store, request := automationFixture()
	accounts.models = []string{"model-a", "model-b"}
	if err := json.Unmarshal([]byte(`{"selected_models":["model-b"]}`), &request); err != nil {
		t.Fatal(err)
	}
	preview, err := service.Enqueue(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	request.DryRun, request.PreviewTaskID = false, preview.ID
	execution, err := service.Enqueue(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if store.items[execution.ID].Status != "succeeded" || !reflect.DeepEqual(management.channel.Models, []string{"model-b"}) {
		t.Fatalf("only model-b should be published: models=%v task=%#v", management.channel.Models, store.items[execution.ID])
	}
}

func TestEmptyPublicationSelectionIsRejected(t *testing.T) {
	service, _, _, _, request := automationFixture()
	if err := json.Unmarshal([]byte(`{"selected_models":[]}`), &request); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Enqueue(context.Background(), request); err == nil || !strings.Contains(err.Error(), "选择") {
		t.Fatalf("empty selection accepted: %v", err)
	}
	service.UseScheduleRepository(&memorySchedule{})
	if _, err := service.SaveSchedule(context.Background(), accountautomation.Schedule{Name: "任务", Request: request, IntervalMinutes: 60}); err == nil {
		t.Fatal("empty schedule selection accepted")
	}
}

func TestPreviewDescribesPlannedAdditionsWithoutClaimingWrites(t *testing.T) {
	service, _, _, store, request := automationFixture()
	preview, err := service.Enqueue(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	result := store.items[preview.ID]
	if !strings.Contains(result.Message, "预览") || !strings.Contains(result.Message, "未写入") {
		t.Fatalf("misleading preview: %s", result.Message)
	}
	for _, raw := range result.Result["items"].([]any) {
		item := raw.(map[string]any)
		if item["channel_id"] == "11" && (item["planned"] == nil || item["added"] != nil) {
			t.Fatalf("preview claims additions: %#v", item)
		}
	}
}

func TestFailedPublicationDoesNotReportAddedModels(t *testing.T) {
	service, _, management, store, request := automationFixture()
	preview, err := service.Enqueue(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	management.failPublish = true
	request.DryRun, request.PreviewTaskID = false, preview.ID
	execution, err := service.Enqueue(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range store.items[execution.ID].Result["items"].([]any) {
		item := raw.(map[string]any)
		if item["channel_id"] == "11" && item["added"] != nil {
			t.Fatalf("failed write claims additions: %#v", item)
		}
	}
}

func TestSelectionChangeInvalidatesPreview(t *testing.T) {
	service, accounts, management, _, request := automationFixture()
	accounts.models = []string{"model-a", "model-b"}
	preview, err := service.Enqueue(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	request.DryRun, request.PreviewTaskID = false, preview.ID
	request.SelectedModels = []string{"model-b"}
	if _, err := service.Enqueue(context.Background(), request); err == nil || !strings.Contains(err.Error(), "预览") {
		t.Fatalf("changed selection accepted: %v", err)
	}
	if management.publishCalls != 0 {
		t.Fatal("changed selection caused a write")
	}
}

func TestUnavailableSelectedModelFailsPreview(t *testing.T) {
	service, _, management, store, request := automationFixture()
	request.SelectedModels = []string{"model-removed"}
	preview, err := service.Enqueue(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	result := store.items[preview.ID]
	if result.Status != "failed" || !strings.Contains(result.Message, "model-removed") || management.publishCalls != 0 {
		t.Fatalf("missing selection was substituted: %#v", result)
	}
}

func TestDisabledPublicationDoesNotPlanOrPriceModels(t *testing.T) {
	service, _, management, store, request := automationFixture()
	request.PublishMissing = false
	preview, err := service.Enqueue(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range store.items[preview.ID].Result["items"].([]any) {
		item := raw.(map[string]any)
		if item["channel_id"] != nil && (item["planned"] != nil || item["added"] != nil) {
			t.Fatalf("disabled publication planned a write: %#v", item)
		}
	}
	if management.catalogReads != 0 {
		t.Fatal("disabled publication loaded pricing")
	}
}

func TestLegacyScheduleDoesNotDefaultToAllModels(t *testing.T) {
	service, _, _, _, request := automationFixture()
	request.SelectedModels = nil
	raw, err := json.Marshal([]accountautomation.Schedule{{ID: "old", Name: "旧任务", Request: request, Enabled: true, IntervalMinutes: 60}})
	if err != nil {
		t.Fatal(err)
	}
	service.UseScheduleRepository(&memorySchedule{raw: string(raw)})
	loaded, err := service.Schedules(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 1 || loaded[0].Request.PublishMissing || loaded[0].Request.PriceEnabled {
		t.Fatalf("legacy publication remains enabled: %#v", loaded)
	}
}

func TestSavedSchedulePublishesOnlyItsSelectedModel(t *testing.T) {
	service, accounts, management, store, request := automationFixture()
	accounts.models = []string{"model-a", "model-b"}
	request.SelectedModels = []string{"model-b"}
	store.completed = make(chan struct{}, 1)
	service.UseScheduleRepository(&memorySchedule{})
	saved, err := service.SaveSchedule(context.Background(), accountautomation.Schedule{Name: "指定模型", Request: request, IntervalMinutes: 60})
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := service.Schedules(context.Background())
	if err != nil || !reflect.DeepEqual(loaded[0].Request.SelectedModels, []string{"model-b"}) {
		t.Fatalf("selection not persisted: %#v %v", loaded, err)
	}
	if _, err := service.RunSchedule(context.Background(), saved.ID); err != nil {
		t.Fatal(err)
	}
	select {
	case <-store.completed:
	case <-time.After(5 * time.Second):
		t.Fatal("schedule did not finish")
	}
	if !reflect.DeepEqual(management.channel.Models, []string{"model-b"}) {
		t.Fatalf("schedule published other models: %v", management.channel.Models)
	}
}

func TestInvalidSelectedModelNamesAreRejected(t *testing.T) {
	for _, models := range [][]string{{""}, {"model-a", "MODEL-A"}, {"model-a\x00"}, {strings.Repeat("a", 256)}} {
		service, _, _, _, request := automationFixture()
		request.SelectedModels = models
		if _, err := service.Enqueue(context.Background(), request); err == nil {
			t.Fatalf("invalid selection accepted: %q", models)
		}
	}
}
