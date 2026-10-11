package business_test

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/sqliteutil"
)

func BenchmarkCommitAccountGroupsCounts(b *testing.B) {
	path := filepath.Join(b.TempDir(), "memberships.db")
	store, err := business.Open(path)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = store.Close() })
	if err := store.Bootstrap(b.Context()); err != nil {
		b.Fatal(err)
	}
	db, err := sql.Open("sqlite", sqliteutil.DSN(path, ""))
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec(`WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<200)
		INSERT INTO local_groups(name,remote_id,account_count,updated_at) SELECT printf('group-%d',x),CAST(x AS TEXT),10,'before' FROM n;
		WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<2000)
		INSERT INTO accounts(id,name,multiplier,updated_at) SELECT CAST(x AS TEXT),printf('account-%d',x),'0.1','before' FROM n;
		INSERT INTO account_groups(account_id,group_name,group_id)
		SELECT id,printf('group-%d',((CAST(id AS INTEGER)-1)%200)+1),CAST(((CAST(id AS INTEGER)-1)%200)+1 AS TEXT) FROM accounts`)
	if err != nil {
		b.Fatal(err)
	}
	operation := business.AccountOperation{OperationID: "benchmark-move", OperationType: "account.groups", ObjectID: "1", State: "succeeded", Phase: "readback", Actor: "benchmark"}
	before, after := []string{"1"}, []string{"2"}
	b.ReportAllocs()
	for b.Loop() {
		if err := store.CommitAccountGroups(b.Context(), "1", before, after, operation); err != nil {
			b.Fatal(err)
		}
		before, after = after, before
	}
	var memberships int
	if err := db.QueryRow(`SELECT SUM(account_count) FROM local_groups`).Scan(&memberships); err != nil || memberships != 2000 {
		b.Fatalf("memberships=%d error=%v", memberships, err)
	}
}
