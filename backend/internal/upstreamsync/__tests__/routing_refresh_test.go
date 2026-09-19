package upstreamsync_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/routing"
	"github.com/MIEnchating/sub2api-console/backend/internal/runtimepolicy"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskrunner"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/upstreamsync"
)

type routingRefreshTasks struct{ done chan taskstore.Task }

func (s routingRefreshTasks) Save(_ context.Context, task taskstore.Task) error {
	if task.Status == "succeeded" || task.Status == "failed" || task.Status == "cancelled" {
		s.done <- task
	}
	return nil
}

func TestBalanceSyncRefreshesRoutingBeforeTaskCompletes(t *testing.T) {
	for _, mode := range []string{runtimepolicy.Full, runtimepolicy.Monitoring} {
		t.Run(mode, func(t *testing.T) {
			service, store, db, tasks := routingRefreshFixture(t, mode)
			if _, err := service.EnqueueAll(t.Context(), upstreamsync.Scope{Balance: true}, "test", "balance-sync"); err != nil {
				t.Fatal(err)
			}
			task := awaitRoutingRefreshTask(t, tasks)
			if task.Status != "succeeded" {
				t.Fatalf("sync failed: %+v", task)
			}
			result, ok := task.Result["routing"].(routing.Result)
			if !ok || result.Accounts != 1 {
				t.Fatalf("sync completed without calculating the waiting account: %+v", task.Result)
			}
			decision := result.AccountDecisions["41"]
			if mode == runtimepolicy.Full && (decision.RoutingState == "concurrency_limited" || !decision.Schedulable) {
				t.Fatalf("sync must calculate recovery using freshly confirmed capacity: %+v", decision)
			}
			rows, err := store.PreviousRoutingDecisions(t.Context(), nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			if mode == runtimepolicy.Full && len(rows) != 1 || mode == runtimepolicy.Monitoring && len(rows) != 0 {
				t.Fatalf("persisted decisions violate runtime mode %s: %+v", mode, rows)
			}
			var schedulable, concurrency int
			if err := db.QueryRow(`SELECT schedulable,concurrency FROM accounts WHERE id='41'`).Scan(&schedulable, &concurrency); err != nil {
				t.Fatal(err)
			}
			if schedulable != 0 || concurrency != 3 || task.Result["remote_write"] != false {
				t.Fatal("a sync calculation must not bypass recovery policy or perform remote writes")
			}
		})
	}
}

func TestBalanceSyncUsesNewCapacityToCalculateRecovery(t *testing.T) {
	service, store, _, tasks := routingRefreshFixture(t, runtimepolicy.Full)
	if _, err := store.UpdatePolicy(t.Context(), map[string]any{
		"advanced_policy": map[string]any{
			"scaling": map[string]any{"enabled": true, "global_max_concurrency": 100, "min_per_account": 1, "max_per_account": 10},
		},
		"auto_apply": map[string]any{"concurrency": true, "schedulable": true},
	}, "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.EnqueueHost(t.Context(), "capacity.example.test", upstreamsync.Scope{Balance: true}, "test", "balance-sync"); err != nil {
		t.Fatal(err)
	}
	task := awaitRoutingRefreshTask(t, tasks)
	result, ok := task.Result["routing"].(routing.Result)
	if task.Status != "succeeded" || !ok {
		t.Fatalf("sync did not finish calculation: %+v", task)
	}
	target := result.AccountTargets["41"]
	if target.Schedulable == nil || !*target.Schedulable || target.DesiredHealth == "concurrency_limited" {
		t.Fatalf("fresh capacity must produce a recovery target: %+v", target)
	}
}

func TestBalanceSyncReportsCalculationFailureWithoutDiscardingSyncedCapacity(t *testing.T) {
	service, store, db, tasks := routingRefreshFixture(t, runtimepolicy.Full)
	if _, err := db.Exec(`DROP TABLE health_samples`); err != nil {
		t.Fatal(err)
	}
	if _, err := service.EnqueueAll(t.Context(), upstreamsync.Scope{Balance: true}, "test", "balance-sync"); err != nil {
		t.Fatal(err)
	}
	task := awaitRoutingRefreshTask(t, tasks)
	if task.Status != "failed" || task.Result["routing_error"] == nil || !strings.Contains(task.Message, "调度重新计算失败") {
		t.Fatalf("calculation failure must be visible in the task: %+v", task)
	}
	summary, err := store.Upstreams(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(summary.Hosts) != 1 || summary.Hosts[0].ConcurrencyLimit == nil || *summary.Hosts[0].ConcurrencyLimit != 10 {
		t.Fatalf("calculation failure discarded the synced capacity: %+v", summary)
	}
}

func TestCatalogOnlySyncDoesNotRecalculateCapacity(t *testing.T) {
	service, store, _, tasks := routingRefreshFixture(t, runtimepolicy.Full)
	if _, err := service.EnqueueAll(t.Context(), upstreamsync.Scope{Catalog: true}, "test", "groups-sync"); err != nil {
		t.Fatal(err)
	}
	task := awaitRoutingRefreshTask(t, tasks)
	if task.Status != "succeeded" || task.Result["routing"] != nil {
		t.Fatalf("catalog sync must not trigger another capacity calculation: %+v", task)
	}
	rows, err := store.PreviousRoutingDecisions(t.Context(), nil, nil)
	if err != nil || len(rows) != 0 {
		t.Fatalf("catalog-only sync changed routing decisions: %+v, %v", rows, err)
	}
}

func routingRefreshFixture(t *testing.T, mode string) (*upstreamsync.Service, *business.Store, *sql.DB, routingRefreshTasks) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "routing-refresh.sqlite3")
	store, err := business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Bootstrap(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetMode(t.Context(), mode); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdatePolicy(t.Context(), map[string]any{"advanced_policy": map[string]any{
		"scaling":              map[string]any{"enabled": false},
		"upstream_concurrency": map[string]any{"enabled": true},
	}}, "test"); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(10000)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	for _, statement := range []string{
		`INSERT INTO upstreams(host,base_url,upstream_type,auth_status,metadata_json,updated_at) VALUES('capacity.example.test','https://capacity.example.test','sub2api','已鉴权','{}','before')`,
		`INSERT INTO upstream_identities(upstream_id,created_at,updated_at) VALUES('upstream-1','before','before')`,
		`INSERT INTO upstream_identity_hosts(host,upstream_id,is_primary,updated_at) VALUES('capacity.example.test','upstream-1',1,'before')`,
		`INSERT INTO accounts(id,name,upstream_host,upstream_type,schedulable,concurrency,multiplier,routing_state,updated_at) VALUES('41','waiting-account','capacity.example.test','sub2api',0,3,'1','concurrency_limited','before')`,
		`INSERT INTO account_groups(account_id,group_name,group_id) VALUES('41','codex','7')`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	tasks := routingRefreshTasks{done: make(chan taskstore.Task, 1)}
	reader := balanceReader{read: func(context.Context, string) (business.UpstreamBalanceObservation, error) {
		limit := int64(10)
		return business.UpstreamBalanceObservation{ConcurrencyLimit: &limit, ProfileUserID: "17"}, nil
	}}
	service := upstreamsync.New(store, privateRecords{}, reader, nil, tasks)
	runner := taskrunner.New(t.Context())
	t.Cleanup(func() { _ = runner.Shutdown(context.Background()) })
	service.UseTaskRunner(runner)
	return service, store, db, tasks
}

func awaitRoutingRefreshTask(t *testing.T, tasks routingRefreshTasks) taskstore.Task {
	t.Helper()
	select {
	case task := <-tasks.done:
		return task
	case <-time.After(10 * time.Second):
		t.Fatal("sync task did not finish")
		return taskstore.Task{}
	}
}
