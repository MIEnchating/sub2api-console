package business_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

func animationEvidenceStore(t *testing.T) (*business.Store, *sql.DB) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "animation-evidence.sqlite3")
	store, err := business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Bootstrap(context.Background()); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return store, db
}

func TestPersistAnimationEvidenceCopiesToEveryCurrentGroupAndUpdatesIdempotently(t *testing.T) {
	store, db := animationEvidenceStore(t)
	ctx := t.Context()
	if _, err := db.ExecContext(ctx, `INSERT INTO accounts(id,name,multiplier,schedulable,metadata_json,updated_at) VALUES('41','animation','1',1,'{}','now');
		INSERT INTO account_groups(account_id,group_name,group_id) VALUES('41','alpha','7'),('41','beta','8')`); err != nil {
		t.Fatal(err)
	}
	if err := store.PersistAnimationEvidence(ctx, []business.AnimationEvidence{{
		AccountID: "41", TaskID: "task-1", Mode: "animation", Verdict: "not_passed", Error: "动画异常", ObservedAt: "2026-09-29T01:00:00Z",
	}}); err != nil {
		t.Fatal(err)
	}
	if err := store.PersistAnimationEvidence(ctx, []business.AnimationEvidence{{
		AccountID: "41", TaskID: "task-1", Mode: "animation", Verdict: "passed", ObservedAt: "2026-09-29T01:01:00Z",
	}}); err != nil {
		t.Fatal(err)
	}
	rows, err := store.RoutingAnimationEvidence(ctx, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected one latest row per group, got %+v", rows)
	}
	for _, row := range rows {
		if row.AccountID != "41" || (row.GroupName != "alpha" && row.GroupName != "beta") || row.Verdict != "passed" || row.TaskID != "task-1" || row.Mode != "animation" {
			t.Fatalf("unexpected latest evidence: %+v", row)
		}
	}
}

func TestRoutingAnimationEvidencePrefersPrecheckForSameAccountAndGroup(t *testing.T) {
	store, db := animationEvidenceStore(t)
	ctx := t.Context()
	if _, err := db.ExecContext(ctx, `INSERT INTO accounts(id,name,multiplier,schedulable,metadata_json,updated_at) VALUES('41','animation','1',1,'{}','now');
		INSERT INTO account_groups(account_id,group_name,group_id) VALUES('41','alpha','7')`); err != nil {
		t.Fatal(err)
	}
	if err := store.PersistAnimationEvidence(ctx, []business.AnimationEvidence{
		{AccountID: "41", TaskID: "task-animation", Mode: "animation", Verdict: "passed", ObservedAt: "2026-09-29T01:02:00Z"},
		{AccountID: "41", TaskID: "task-precheck", Mode: "precheck", Verdict: "not_passed", ObservedAt: "2026-09-29T00:00:00Z"},
	}); err != nil {
		t.Fatal(err)
	}
	rows, err := store.RoutingAnimationEvidence(ctx, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Mode != "precheck" || rows[0].TaskID != "task-precheck" || rows[0].Verdict != "not_passed" {
		t.Fatalf("precheck evidence did not take precedence: %+v", rows)
	}
}
