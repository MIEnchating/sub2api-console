package taskstore_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/sqliteutil"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

func TestModelCheckHistoryIgnoresMalformedTestEntriesAndKeepsValidVerdicts(t *testing.T) {
	store, err := taskstore.Open(filepath.Join(t.TempDir(), "history.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	for _, task := range []taskstore.Task{
		{ID: "valid", Skill: "sub2api-model-check", Operation: "account-model-behavior-check", Status: "succeeded", Progress: 100, Message: "检测完成",
			CreatedAt: "2026-10-01T00:00:00Z", UpdatedAt: "2026-10-01T00:00:00Z",
			Result: map[string]any{"account_ids": []any{"41", nil, 42, "", map[string]any{"id": "43"}}, "tests": []any{
				map[string]any{"account_id": "41", "claimed_model": "model-a", "verdict": "MATCH", "requests": map[string]any{"total": 2, "successful": 2}, "secret_detail": "不应公开"},
				"broken entry", 7, nil, []any{"nested"}, map[string]any{"account_id": 41}, map[string]any{"account_id": ""},
			}}},
		{ID: "wrong-container", Skill: "sub2api-model-check", Operation: "account-model-behavior-check", Status: "failed", Progress: 100, Message: "旧结果格式异常",
			CreatedAt: "2026-10-02T00:00:00Z", UpdatedAt: "2026-10-02T00:00:00Z",
			Result: map[string]any{"tests": "not an array", "account_ids": "42"}},
	} {
		if err := store.Save(ctx, task); err != nil {
			t.Fatal(err)
		}
	}
	history, err := store.ModelCheckHistory(ctx, time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("malformed entry hid valid model-check history: %v", err)
	}
	if len(history) != 2 {
		t.Fatalf("history count=%d; want both task records", len(history))
	}
	valid := history[1]
	tests, ok := valid.Result["tests"].([]any)
	if valid.ID != "valid" || !ok || len(tests) != 1 {
		t.Fatalf("valid verdict projection missing: %#v", valid)
	}
	verdict := tests[0].(map[string]any)
	if verdict["account_id"] != "41" || verdict["claimed_model"] != "model-a" || verdict["verdict"] != "MATCH" || verdict["secret_detail"] != nil {
		t.Fatalf("public verdict projection changed: %#v", verdict)
	}
	requests, ok := verdict["requests"].(map[string]any)
	if !ok || requests["total"] != json.Number("2") || requests["successful"] != json.Number("2") {
		t.Fatalf("request counts were not preserved: %#v", verdict["requests"])
	}
	ids := valid.Result["account_ids"].([]any)
	if len(ids) != 1 || ids[0] != "41" {
		t.Fatalf("malformed account IDs were projected: %#v", ids)
	}
	if malformed := history[0].Result; len(malformed["tests"].([]any)) != 0 || len(malformed["account_ids"].([]any)) != 0 {
		t.Fatalf("wrong containers inferred account identity: %#v", malformed)
	}
}

func TestModelCheckHistoryKeepsLatestOlderCheckOnlyForValidAccountIdentities(t *testing.T) {
	store, err := taskstore.Open(filepath.Join(t.TempDir(), "history.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	for _, fixture := range []struct {
		id, updated string
		result      map[string]any
	}{
		{"older", "2026-08-01T00:00:00Z", map[string]any{"account_ids": []any{"41"}}},
		{"latest", "2026-08-02T00:00:00Z", map[string]any{"tests": []any{map[string]any{"account_id": "41", "verdict": "match"}}}},
		{"invalid-identity", "2026-08-03T00:00:00Z", map[string]any{"account_ids": []any{nil, "", 41, map[string]any{"id": "42"}}, "tests": []any{map[string]any{"account_id": 41}}}},
	} {
		task := taskstore.Task{ID: fixture.id, Skill: "sub2api-model-check", Operation: "account-model-behavior-check", Status: "succeeded", Progress: 100, Message: "旧检查", Result: fixture.result, CreatedAt: fixture.updated, UpdatedAt: fixture.updated}
		if err := store.Save(ctx, task); err != nil {
			t.Fatal(err)
		}
	}
	history, err := store.ModelCheckHistory(ctx, time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC))
	if err != nil || len(history) != 1 || history[0].ID != "latest" {
		t.Fatalf("invalid account IDs displaced older valid check: %#v, %v", history, err)
	}
}

func TestModelCheckHistorySkipsCorruptJSONAndKeepsLatestForEachAccount(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.sqlite3")
	store, err := taskstore.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	for _, fixture := range []struct {
		id, updated string
		accounts    []string
	}{
		{"shared", "2026-08-01T00:00:00Z", []string{"41", "42"}},
		{"latest-41", "2026-08-02T00:00:00Z", []string{"41"}},
	} {
		if err := store.Save(ctx, taskstore.Task{ID: fixture.id, Skill: "sub2api-model-check", Operation: "account-model-behavior-check", Status: "succeeded", Progress: 100, Message: "旧检查", Result: map[string]any{"account_ids": fixture.accounts}, CreatedAt: fixture.updated, UpdatedAt: fixture.updated}); err != nil {
			t.Fatal(err)
		}
	}
	db, err := sql.Open("sqlite", sqliteutil.DSN(path, ""))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.ExecContext(ctx, `INSERT INTO tasks VALUES('corrupt','sub2api-model-check','account-model-behavior-check','succeeded',100,'完成','{broken','2026-10-02T00:00:00.000000000Z','2026-10-02T00:00:00.000000000Z')`); err != nil {
		t.Fatal(err)
	}
	history, err := store.ModelCheckHistory(ctx, time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC))
	if err != nil || len(history) != 2 || history[0].ID != "latest-41" || history[1].ID != "shared" {
		t.Fatalf("latest per-account history changed: %#v, %v", history, err)
	}
}
