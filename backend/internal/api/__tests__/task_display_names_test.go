package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/api"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/config"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

func taskNameStores(t testing.TB, rows []map[string]any) (*business.Store, *configstore.Store, *taskstore.Store) {
	t.Helper()
	ctx := context.Background()
	directory := t.TempDir()
	accounts, err := business.Open(filepath.Join(directory, "business.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = accounts.Close() })
	if err := accounts.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	_, err = accounts.SyncManagementSnapshot(ctx, rows, nil, "test")
	if err != nil {
		t.Fatal(err)
	}
	private, err := configstore.Open(filepath.Join(directory, "private.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = private.Close() })
	tasks, err := taskstore.Open(filepath.Join(directory, "tasks.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tasks.Close() })
	return accounts, private, tasks
}

func TestTaskDetailDisplayNamesPreserveLargeScopeAndIgnoreUnrelatedOperations(t *testing.T) {
	rows := make([]map[string]any, 206)
	results := make([]any, 205)
	want := make(map[string]string)
	for index := range rows {
		id := strconv.Itoa(index + 1)
		name := "账号 " + id
		rows[index] = map[string]any{"id": json.Number(id), "name": name}
		if index < len(results) {
			results[index] = map[string]any{"account_id": id, "result": "通过"}
			want[id] = name
		}
	}
	accounts, private, tasks := taskNameStores(t, rows)
	router := api.New(config.Config{AdminToken: "isolated-token"}, private, accounts, api.Dependencies{Tasks: tasks})
	for index, scenario := range []struct {
		name, operation string
		results         any
		names           map[string]string
	}{
		{"large legacy task", "active-probe", results, want},
		{"unrelated operation", "account-model-automation", results, nil},
		{"invalid identities", "active-probe", []any{map[string]any{"account_id": " 1 "}, map[string]any{"account_id": 2}, map[string]any{"account_id": ""}}, nil},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			task := taskstore.Task{ID: "scope-task-" + strconv.Itoa(index), Skill: "console", Operation: scenario.operation, Status: "succeeded", Progress: 100, Message: "完成", CreatedAt: "2026-10-10T00:00:00Z", UpdatedAt: "2026-10-10T00:00:00Z", Result: map[string]any{"results": scenario.results}}
			if err := tasks.Save(context.Background(), task); err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest(http.MethodGet, "/api/tasks/"+task.ID, nil)
			request.Header.Set("Authorization", "Bearer isolated-token")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != http.StatusOK {
				t.Fatalf("status %d: %s", response.Code, response.Body.String())
			}
			var payload struct {
				Names map[string]string `json:"display_account_names"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(payload.Names, scenario.names) {
				t.Fatalf("names=%#v; want %#v", payload.Names, scenario.names)
			}
		})
	}
}

func TestTaskDetailDisplayNamesOnlyIncludeMissingProbeNamesAndPreserveHistory(t *testing.T) {
	ctx := context.Background()
	accounts, private, tasks := taskNameStores(t, []map[string]any{
		{"id": json.Number("41"), "name": "当前目录名称"},
		{"id": json.Number("42"), "name": "已改名账号"},
		{"id": json.Number("43"), "name": "无关账号"},
	})
	historical := map[string]any{"results": []any{
		map[string]any{"account_id": "41", "result": "通过"},
		map[string]any{"account_id": "41", "result": "超时"},
		map[string]any{"account_id": "42", "account_name": "历史名称", "result": "通过"},
		map[string]any{"account_id": "99", "result": "失败"},
		map[string]any{"account_id": json.Number("43")},
		"invalid",
	}}
	task := taskstore.Task{ID: "probe-legacy", Skill: "console", Operation: "active-probe", Status: "succeeded", Progress: 100, Message: "完成", Result: historical, CreatedAt: "2026-10-10T00:00:00Z", UpdatedAt: "2026-10-10T00:00:00Z"}
	if err := tasks.Save(ctx, task); err != nil {
		t.Fatal(err)
	}
	router := api.New(config.Config{AdminToken: "isolated-token"}, private, accounts, api.Dependencies{Tasks: tasks})
	read := func(auth bool) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodGet, "/api/tasks/probe-legacy", nil)
		if auth {
			request.Header.Set("Authorization", "Bearer isolated-token")
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		return response
	}
	t.Run("authorized display projection", func(t *testing.T) {
		response := read(true)
		if response.Code != http.StatusOK {
			t.Fatalf("status %d: %s", response.Code, response.Body.String())
		}
		var payload struct {
			taskstore.Task
			Names map[string]string `json:"display_account_names"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(payload.Names, map[string]string{"41": "当前目录名称"}) {
			t.Fatalf("names = %#v", payload.Names)
		}
		if response.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("task response must not be stored in browser caches")
		}
		saved, err := tasks.Get(ctx, task.ID)
		if err != nil {
			t.Fatal(err)
		}
		want, _ := json.Marshal(historical)
		got, _ := json.Marshal(saved.Result)
		returned, _ := json.Marshal(payload.Result)
		if string(got) != string(want) || string(returned) != string(want) {
			t.Fatal("display projection rewrote historical result")
		}
	})
	t.Run("authentication required", func(t *testing.T) {
		if response := read(false); response.Code != http.StatusUnauthorized {
			t.Fatalf("anonymous status %d", response.Code)
		}
	})
	t.Run("directory failure preserves task", func(t *testing.T) {
		if err := accounts.Close(); err != nil {
			t.Fatal(err)
		}
		response := read(true)
		if response.Code != http.StatusOK {
			t.Fatalf("optional directory failure blocked task: %d", response.Code)
		}
		var payload map[string]any
		if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
			t.Fatal(err)
		}
		if payload["id"] != task.ID || payload["display_account_names"] != nil {
			t.Fatalf("unexpected fallback %#v", payload)
		}
	})
}
