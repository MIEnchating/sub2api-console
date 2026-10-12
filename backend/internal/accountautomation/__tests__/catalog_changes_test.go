package accountautomation_test

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/newapimanagement"
)

func TestCatalogChangesPublishOnlyAvailableSelectedModels(t *testing.T) {
	for _, tc := range []struct {
		name       string
		preview    []string
		current    []string
		selected   []string
		wantModels []string
		wantSkip   []any
	}{
		{name: "unselected model added", preview: []string{"model-a"}, current: []string{"model-a", "model-b"}, selected: []string{"model-a"}, wantModels: []string{"model-a"}},
		{name: "unselected model removed", preview: []string{"model-a", "model-b"}, current: []string{"model-a"}, selected: []string{"model-a"}, wantModels: []string{"model-a"}},
		{name: "one selected model removed", preview: []string{"model-a", "model-b"}, current: []string{"model-a"}, selected: []string{"model-a", "model-b"}, wantModels: []string{"model-a"}, wantSkip: []any{"model-b"}},
		{name: "all selected models removed", preview: []string{"model-a"}, current: []string{"model-b"}, selected: []string{"model-a"}, wantSkip: []any{"model-a"}},
		{name: "directory becomes empty", preview: []string{"model-a"}, current: []string{}, selected: []string{"model-a"}, wantSkip: []any{"model-a"}},
		{name: "empty directory remains empty", preview: []string{}, current: []string{}, selected: []string{"model-a"}, wantSkip: []any{"model-a"}},
		{name: "model spelling changes without changing identity", preview: []string{"model-a"}, current: []string{"MODEL-A"}, selected: []string{"model-a"}, wantModels: []string{"model-a"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service, accounts, management, store, request := automationFixture()
			accounts.models = tc.preview
			request.SelectedModels = tc.selected
			management.prices = append(management.prices, newapimanagement.Sub2APIModelPrice{Model: "model-b", ModelRatio: "1", CompletionRatio: "2"})
			preview, err := service.Enqueue(context.Background(), request)
			if err != nil {
				t.Fatal(err)
			}
			if got := store.items[preview.ID]; got.Status != "succeeded" {
				t.Fatalf("preview failed: %#v", got)
			}
			accounts.models = tc.current
			request.DryRun, request.PreviewTaskID = false, preview.ID
			execution, err := service.Enqueue(context.Background(), request)
			if err != nil {
				t.Fatal(err)
			}
			result := store.items[execution.ID]
			if result.Status != "succeeded" || !reflect.DeepEqual(management.channel.Models, tc.wantModels) {
				t.Fatalf("expected only available selection %v, got models=%v task=%#v", tc.wantModels, management.channel.Models, result)
			}
			wantWrites := 0
			if len(tc.wantModels) > 0 {
				wantWrites = 1
			}
			if management.publishCalls != wantWrites || management.priceCalls != wantWrites {
				t.Fatalf("expected %d publication/pricing writes, got %d/%d", wantWrites, management.publishCalls, management.priceCalls)
			}
			if len(tc.wantSkip) > 0 {
				if !reflect.DeepEqual(result.Result["skipped_models"], tc.wantSkip) {
					t.Fatalf("missing skipped models: %#v", result.Result)
				}
				for _, model := range tc.wantSkip {
					if !strings.Contains(result.Message, model.(string)) || !strings.Contains(result.Message, "跳过") {
						t.Fatalf("missing visible skip reason: %s", result.Message)
					}
				}
			}
		})
	}
}

func TestRemovedSelectionDoesNotBypassRemainingModelPriceConfirmation(t *testing.T) {
	service, accounts, management, store, request := automationFixture()
	accounts.models = []string{"model-a", "model-b"}
	request.SelectedModels = []string{"model-a", "model-b"}
	preview, err := service.Enqueue(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	accounts.models = []string{"model-a"}
	management.prices[0].ModelRatio = "0.8"
	request.DryRun, request.PreviewTaskID = false, preview.ID
	execution, err := service.Enqueue(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	result := store.items[execution.ID]
	if result.Status != "failed" || !strings.Contains(result.Message, "模型价格与预览不一致") || management.publishCalls != 0 || management.priceCalls != 0 {
		t.Fatalf("changed price must still require confirmation: %#v", result)
	}
}
