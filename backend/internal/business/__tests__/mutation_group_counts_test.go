package business_test

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

type groupCountMutation struct {
	name       string
	run        func(*testing.T, *business.Store) error
	newCount   int
	auditTable string
}

func groupCountMutations() []groupCountMutation {
	operation := business.AccountOperation{
		OperationID: "group-count-mutation", OperationType: "account.groups.sync",
		State: "succeeded", Phase: "readback", ObjectID: "41", Actor: "test",
		RemoteConfirmed: true, ReadbackConfirmed: true,
	}
	return []groupCountMutation{
		{name: "group-readback", newCount: 2, auditTable: "operation_audit", run: func(t *testing.T, store *business.Store) error {
			return store.CommitAccountGroupsReadback(t.Context(), "41", []business.LocalOnboardingGroup{{ID: "8", Name: "new"}}, nil, operation)
		}},
		{name: "account-delete", newCount: 1, auditTable: "operation_audit", run: func(t *testing.T, store *business.Store) error {
			deleteOperation := operation
			deleteOperation.OperationType = "account.delete"
			return store.DeleteAccountProjection(t.Context(), "41", deleteOperation)
		}},
		{name: "upstream-delete", newCount: 1, auditTable: "runtime_events", run: func(t *testing.T, store *business.Store) error {
			_, err := store.DeleteUpstreamProjection(t.Context(), "removed.example", []string{"41"}, business.UpstreamDeleteAudit{RemoteDeletedAccounts: 1, ReadbackConfirmed: true})
			return err
		}},
	}
}

func mutationGroupCountFixture(t *testing.T) (*business.Store, *sql.DB) {
	t.Helper()
	store, db := pricingProjectionFixture(t)
	for _, statement := range []string{
		`INSERT INTO upstreams(host,base_url,upstream_type,auth_status,updated_at)
		VALUES('removed.example','https://removed.example','sub2api','ready','before')`,
		`INSERT INTO accounts(id,name,upstream_host,multiplier,updated_at) VALUES
		('41','moving','removed.example','0.1','before'),('42','stable-peer',NULL,'0.2','before'),('43','legacy-peer',NULL,'0.1','before')`,
		`INSERT INTO local_groups(name,remote_id,account_count,updated_at) VALUES
		('old','7',99,'before'),('new','8',99,'before'),('legacy',NULL,99,'before'),('empty','9',99,'before')`,
		`INSERT INTO account_groups(account_id,group_name,group_id) VALUES
		('41','old','7'),('42','previous-new-name','8'),('43','old',NULL),('43','legacy',NULL),('42','empty','888')`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	return store, db
}

func TestConfirmedMutationsCountStableGroupsAfterRenameAndKeepLegacyFallback(t *testing.T) {
	for _, mutation := range groupCountMutations() {
		t.Run(mutation.name, func(t *testing.T) {
			store, db := mutationGroupCountFixture(t)
			if err := mutation.run(t, store); err != nil {
				t.Fatal(err)
			}
			for name, want := range map[string]int{"old": 1, "new": mutation.newCount, "legacy": 1, "empty": 0} {
				var count int
				var updatedAt string
				if err := db.QueryRow(`SELECT account_count,updated_at FROM local_groups WHERE name=?`, name).Scan(&count, &updatedAt); err != nil {
					t.Fatal(err)
				}
				if count != want || updatedAt == "before" {
					t.Errorf("group %s count=%d want=%d updatedAt=%s", name, count, want, updatedAt)
				}
			}
		})
	}
}

func TestConfirmedMutationsAuditFailureRollsBackMembershipAndCounts(t *testing.T) {
	for _, mutation := range groupCountMutations() {
		t.Run(mutation.name, func(t *testing.T) {
			store, db := mutationGroupCountFixture(t)
			if _, err := db.Exec(`CREATE TRIGGER reject_mutation_audit BEFORE INSERT ON ` + mutation.auditTable + `
			BEGIN SELECT RAISE(ABORT,'mutation audit unavailable'); END`); err != nil {
				t.Fatal(err)
			}
			if err := mutation.run(t, store); err == nil || !strings.Contains(err.Error(), "mutation audit unavailable") {
				t.Fatalf("audit failure not reported: %v", err)
			}
			assertGroupCountMutationRolledBack(t, db)
		})
	}
}

func TestConfirmedMutationsCountFailureRollsBackMembershipAndAudit(t *testing.T) {
	for _, mutation := range groupCountMutations() {
		t.Run(mutation.name, func(t *testing.T) {
			store, db := mutationGroupCountFixture(t)
			if _, err := db.Exec(`CREATE TRIGGER reject_count_refresh BEFORE UPDATE OF account_count ON local_groups
			BEGIN SELECT RAISE(ABORT,'group counts unavailable'); END`); err != nil {
				t.Fatal(err)
			}
			if err := mutation.run(t, store); err == nil || !strings.Contains(err.Error(), "group counts unavailable") {
				t.Fatalf("count failure not reported: %v", err)
			}
			assertGroupCountMutationRolledBack(t, db)
		})
	}
}

func assertGroupCountMutationRolledBack(t *testing.T, db *sql.DB) {
	t.Helper()
	var groupID, accountUpdatedAt string
	if err := db.QueryRow(`SELECT ag.group_id,a.updated_at FROM account_groups ag JOIN accounts a ON a.id=ag.account_id
	WHERE a.id='41'`).Scan(&groupID, &accountUpdatedAt); err != nil || groupID != "7" || accountUpdatedAt != "before" {
		t.Fatalf("account or membership escaped rollback: group=%s updatedAt=%s error=%v", groupID, accountUpdatedAt, err)
	}
	var groups, upstreams, audits int
	if err := db.QueryRow(`SELECT COUNT(*) FROM local_groups WHERE account_count=99 AND updated_at='before'`).Scan(&groups); err != nil || groups != 4 {
		t.Fatalf("counts escaped rollback: groups=%d error=%v", groups, err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM upstreams WHERE host='removed.example'`).Scan(&upstreams); err != nil || upstreams != 1 {
		t.Fatalf("upstream escaped rollback: count=%d error=%v", upstreams, err)
	}
	if err := db.QueryRow(`SELECT (SELECT COUNT(*) FROM operation_audit)+(SELECT COUNT(*) FROM runtime_events
	WHERE event_type='upstream.deleted')`).Scan(&audits); err != nil || audits != 0 {
		t.Fatalf("failed mutation left audit records: count=%d error=%v", audits, err)
	}
}
