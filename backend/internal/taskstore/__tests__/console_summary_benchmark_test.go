package taskstore_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

func BenchmarkConsoleSummariesWithUnrelatedHistory(b *testing.B) {
	path := filepath.Join(b.TempDir(), "tasks.sqlite3")
	store, err := taskstore.Open(path)
	if err != nil {
		b.Fatal(err)
	}
	defer store.Close()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		b.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`WITH RECURSIVE sequence(n) AS (VALUES(1) UNION ALL SELECT n+1 FROM sequence WHERE n<2000)
		INSERT INTO tasks(id,skill,operation,status,progress,message,result_json,created_at,updated_at)
		SELECT 'hidden-'||n,'console','automatic-inspection','succeeded',100,'完成',?,'now','2026-10-02T00:00:00.000000000Z' FROM sequence`,
		`{"details":"`+strings.Repeat("x", 16384)+`"}`); err != nil {
		b.Fatal(err)
	}
	if _, err := db.Exec(`WITH RECURSIVE sequence(n) AS (VALUES(1) UNION ALL SELECT n+1 FROM sequence WHERE n<20)
		INSERT INTO tasks(id,skill,operation,status,progress,message,result_json,created_at,updated_at)
		SELECT 'visible-'||n,'console','export','succeeded',100,'导出完成',?,'now','2026-10-01T00:00:00.000000000Z' FROM sequence`,
		`{"system_info":true,"run_key":"run","object_label":"导出","details":"`+strings.Repeat("y", 16384)+`"}`); err != nil {
		b.Fatal(err)
	}
	limit := 20
	b.ReportAllocs()
	for b.Loop() {
		rows, err := store.ListConsoleSummaries(context.Background(), &limit)
		if err != nil || len(rows) != 20 || rows[0].Result["object_label"] != "导出" {
			b.Fatalf("summaries = %#v, %v", rows, err)
		}
	}
}

func BenchmarkSaveTaskProgressWithConsoleIndex(b *testing.B) {
	for _, visible := range []bool{false, true} {
		name := "hidden"
		if visible {
			name = "visible"
		}
		b.Run(name, func(b *testing.B) {
			store, err := taskstore.Open(filepath.Join(b.TempDir(), "tasks.sqlite3"))
			if err != nil {
				b.Fatal(err)
			}
			defer store.Close()
			now := time.Now().UTC().Format(time.RFC3339Nano)
			task := taskstore.Task{
				ID: "progress", Skill: "console", Operation: "export", Status: "running", Progress: 50,
				Message: "正在导出", CreatedAt: now, UpdatedAt: now,
				Result: map[string]any{"system_info": visible, "details": strings.Repeat("x", 16384)},
			}
			b.ReportAllocs()
			for b.Loop() {
				task.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
				if err := store.Save(context.Background(), task); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
