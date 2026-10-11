package business_test

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

type groupCountWorkflow struct {
	name       string
	run        func(*testing.T, *business.Store) error
	oldCount   int
	newCount   int
	auditTable string
}

func groupCountWorkflows() []groupCountWorkflow {
	return []groupCountWorkflow{
		{name: "missing-bindings", oldCount: 1, newCount: 1, auditTable: "runtime_events", run: func(t *testing.T, store *business.Store) error {
			_, err := store.CleanupMissingBindings(t.Context(), []string{"41"}, "test")
			return err
		}},
		{name: "partial-management-snapshot", oldCount: 2, newCount: 1, auditTable: "runtime_events", run: func(t *testing.T, store *business.Store) error {
			// Omitted groups and memberships retain their last confirmed identities.
			_, err := store.SyncManagementSnapshot(t.Context(), []map[string]any{{"id": "41", "name": "updated"}},
				[]map[string]any{{"id": "9", "name": "empty"}}, "test")
			return err
		}},
		{name: "onboarding", oldCount: 1, newCount: 2, auditTable: "operation_audit", run: func(t *testing.T, store *business.Store) error {
			return store.CommitOnboardingProjection(t.Context(), business.OnboardingProjection{
				OperationID: "workflow-group-counts", AccountID: "41", AccountName: "updated",
				UpstreamHost: "removed.example", UpstreamType: "sub2api", UpstreamKeyID: "91", UpstreamKeyName: "key",
				UpstreamGroupID: "6", UpstreamGroupName: "upstream-group", LocalGroupID: "8", LocalGroupName: "new",
				Multiplier: "0.2", ReadbackConfirmed: true,
			})
		}},
	}
}

func workflowGroupCountFixture(t *testing.T) (*business.Store, *sql.DB) {
	t.Helper()
	store, db := mutationGroupCountFixture(t)
	if _, err := db.Exec(`INSERT INTO upstream_identities(upstream_id,created_at,updated_at) VALUES('workflow-upstream','before','before');
		INSERT INTO upstream_identity_hosts(host,upstream_id,is_primary,updated_at) VALUES('removed.example','workflow-upstream',1,'before');
		INSERT INTO bindings(local_account_id,upstream_host,upstream_key_id,upstream_key_name,local_group,status,updated_at)
		VALUES('41','removed.example','91','key','old','missing','before')`); err != nil {
		t.Fatal(err)
	}
	return store, db
}

func TestWorkflowsRefreshCountsByStableIDAndOnlyFallbackForLegacyMembers(t *testing.T) {
	for _, workflow := range groupCountWorkflows() {
		t.Run(workflow.name, func(t *testing.T) {
			store, db := workflowGroupCountFixture(t)
			if err := workflow.run(t, store); err != nil {
				t.Fatal(err)
			}
			for name, want := range map[string]int{"old": workflow.oldCount, "new": workflow.newCount, "legacy": 1, "empty": 0} {
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

func TestWorkflowsAuditFailureRollsBackAccountBindingAndGroupCounts(t *testing.T) {
	for _, workflow := range groupCountWorkflows() {
		t.Run(workflow.name, func(t *testing.T) {
			store, db := workflowGroupCountFixture(t)
			if _, err := db.Exec(`CREATE TRIGGER reject_workflow_audit BEFORE INSERT ON ` + workflow.auditTable + `
				BEGIN SELECT RAISE(ABORT,'workflow audit unavailable'); END`); err != nil {
				t.Fatal(err)
			}
			if err := workflow.run(t, store); err == nil || !strings.Contains(err.Error(), "workflow audit unavailable") {
				t.Fatalf("audit failure not reported: %v", err)
			}
			assertGroupCountWorkflowRolledBack(t, db)
		})
	}
}

func TestWorkflowsCountFailureRollsBackAccountBindingAndAudit(t *testing.T) {
	for _, workflow := range groupCountWorkflows() {
		t.Run(workflow.name, func(t *testing.T) {
			store, db := workflowGroupCountFixture(t)
			if _, err := db.Exec(`CREATE TRIGGER reject_workflow_counts BEFORE UPDATE OF account_count ON local_groups
				BEGIN SELECT RAISE(ABORT,'workflow counts unavailable'); END`); err != nil {
				t.Fatal(err)
			}
			if err := workflow.run(t, store); err == nil || !strings.Contains(err.Error(), "workflow counts unavailable") {
				t.Fatalf("count failure not reported: %v", err)
			}
			assertGroupCountWorkflowRolledBack(t, db)
		})
	}
}

func assertGroupCountWorkflowRolledBack(t *testing.T, db *sql.DB) {
	t.Helper()
	assertGroupCountMutationRolledBack(t, db)
	var bindings, audits, catalog int
	if err := db.QueryRow(`SELECT COUNT(*) FROM bindings WHERE local_account_id='41' AND status='missing' AND updated_at='before'`).Scan(&bindings); err != nil || bindings != 1 {
		t.Fatalf("binding escaped rollback: count=%d error=%v", bindings, err)
	}
	if err := db.QueryRow(`SELECT (SELECT COUNT(*) FROM operation_audit)+(SELECT COUNT(*) FROM runtime_events
		WHERE event_type IN ('binding.missing.cleaned','management.snapshot.synced'))`).Scan(&audits); err != nil || audits != 0 {
		t.Fatalf("failed workflow left audit records: count=%d error=%v", audits, err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM upstream_catalog_entities`).Scan(&catalog); err != nil || catalog != 0 {
		t.Fatalf("catalog escaped rollback: count=%d error=%v", catalog, err)
	}
}
