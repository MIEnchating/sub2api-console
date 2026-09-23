package api_test

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/api"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/config"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

func TestDetectionTaskDetailGroupsPreserveHistoryAndScope(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store, err := business.Open(filepath.Join(dir, "business.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	_, err = store.SyncManagementSnapshot(ctx, []map[string]any{
		{"id": json.Number("41"), "name": "original account", "groups": []any{json.Number("7")}},
		{"id": json.Number("42"), "name": "added later", "groups": []any{json.Number("7")}},
	}, []map[string]any{{"id": json.Number("7"), "name": "current name"}}, "test")
	if err != nil {
		t.Fatal(err)
	}
	private, err := configstore.Open(filepath.Join(dir, "private.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = private.Close() })
	tasks, err := taskstore.Open(filepath.Join(dir, "tasks.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tasks.Close() })
	router := api.New(config.Config{AdminToken: "isolated-token"}, private, store, api.Dependencies{Tasks: tasks})
	for _, tc := range []struct {
		name     string
		snapshot bool
		group    string
		auth     bool
	}{
		{"legacy current grouping", false, "7", true},
		{"historical snapshot", true, "7", true},
		{"removed group", false, "99", true},
		{"anonymous", false, "7", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			task := taskstore.Task{ID: "task-1", Skill: "sub2api-model-animation", Operation: "managed-model-detection", Status: "succeeded", Progress: 100, Message: "完成", CreatedAt: "2026-09-23T00:00:00Z", UpdatedAt: "2026-09-23T00:01:00Z", Result: map[string]any{"configuration": map[string]any{"group_ids": []string{tc.group}}, "account_ids": []string{"41"}}}
			if tc.snapshot {
				task.Result["group_ids_by_account"] = map[string][]string{"41": {"7"}}
				task.Result["group_names_by_id"] = map[string]string{"7": "historical name"}
			}
			if err := tasks.Save(ctx, task); err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest("GET", "/api/tasks/task-1", nil)
			if tc.auth {
				request.Header.Set("Authorization", "Bearer isolated-token")
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if !tc.auth {
				if response.Code != 401 {
					t.Fatalf("anonymous %d", response.Code)
				}
				return
			}
			if response.Code != 200 {
				t.Fatalf("%d %s", response.Code, response.Body.String())
			}
			if response.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("missing no-store")
			}
			var got taskstore.Task
			if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if tc.snapshot {
				if got.Result["group_names_by_id"].(map[string]any)["7"] != "historical name" {
					t.Fatal("rewrote snapshot")
				}
				if _, ok := got.Result["grouping_source"]; ok {
					t.Fatal("historical snapshot treated as current")
				}
			} else if tc.group == "99" {
				if got.Result["grouping_source"] != "unavailable" {
					t.Fatal("removed group hidden")
				}
			} else {
				if got.Result["grouping_source"] != "current" {
					t.Fatal("current membership passed as historical")
				}
				memberships := got.Result["group_ids_by_account"].(map[string]any)
				if len(memberships) != 1 || len(memberships["41"].([]any)) != 1 {
					t.Fatalf("included new accounts or lost original: %#v", memberships)
				}
				if got.Result["group_names_by_id"].(map[string]any)["7"] != "current name" {
					t.Fatal("lost current group name")
				}
			}
			saved, err := tasks.Get(ctx, task.ID)
			if err != nil {
				t.Fatal(err)
			}
			if !tc.snapshot {
				if _, ok := saved.Result["group_ids_by_account"]; ok {
					t.Fatal("response enrichment rewrote historical record")
				}
			}
		})
	}
}
