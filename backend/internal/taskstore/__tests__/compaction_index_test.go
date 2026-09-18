package taskstore_test

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

func TestCompactionSelectsOnlyUncompactedTerminalTasksThroughIndex(t *testing.T) {
	path := filepath.Join(t.TempDir(), "compaction.db")
	store, err := taskstore.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rows, err := db.Query(`EXPLAIN QUERY PLAN SELECT id FROM tasks WHERE operation='automatic-inspection'
		AND status NOT IN ('queued','running','waiting_input') AND json_valid(result_json)
		AND COALESCE(json_extract(result_json,'$.compacted'),0)<>1 ORDER BY updated_at,id LIMIT 1`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var plan strings.Builder
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		plan.WriteString(detail)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plan.String(), "ix_tasks_pending_compaction") || strings.Contains(plan.String(), "TEMP B-TREE") {
		t.Fatalf("compaction scans historical JSON or sorts candidates: %s", plan.String())
	}
}
