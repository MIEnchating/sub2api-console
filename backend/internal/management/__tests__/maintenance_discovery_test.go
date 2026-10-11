package management_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/management"
	"github.com/MIEnchating/sub2api-console/backend/internal/runtimepolicy"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

func maintenanceDiscoveryFixture(t testing.TB) (*business.Store, *sql.DB, *taskstore.Store, *management.Service) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "business.db")
	store, err := business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Bootstrap(t.Context()); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	for _, statement := range []string{
		`INSERT INTO upstreams(host,base_url,upstream_type,auth_status,metadata_json,updated_at) VALUES
		('selected.example','https://selected.example','sub2api','ready','{}','now'),
		('unrelated.example','https://unrelated.example','sub2api','ready','{}','now')`,
		`INSERT INTO upstream_identities(upstream_id,created_at,updated_at) VALUES('selected','now','now'),('unrelated','now','now')`,
		`INSERT INTO upstream_identity_hosts(host,upstream_id,is_primary,updated_at) VALUES
		('selected.example','selected',1,'now'),('unrelated.example','unrelated',1,'now')`,
		`INSERT INTO accounts(id,name,metadata_json,updated_at) VALUES('2','same','{}','now'),('10','same','{}','now'),('30','other','{}','now')`,
		`INSERT INTO bindings(local_account_id,upstream_host,upstream_key_id,upstream_key_name,local_group,source_auth_host,updated_at) VALUES
		('2','selected.example','a','fixture','default',NULL,'now'),
		('2','selected.example','b','fixture','default',NULL,'now'),
		('10','unrelated.example','c','fixture','default',' HTTPS://SELECTED.EXAMPLE/v1/ ','now'),
		('30','unrelated.example','d','fixture','default',NULL,'now')`,
		`INSERT INTO binding_identities(binding_id,upstream_id,upstream_key_id,updated_at)
		SELECT id,CASE WHEN upstream_host='selected.example' THEN 'selected' ELSE 'unrelated' END,upstream_key_id,'now' FROM bindings`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	tasks, err := taskstore.Open(filepath.Join(t.TempDir(), "tasks.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tasks.Close() })
	service := management.New(snapshotTarget{"http://127.0.0.1:1"}, store, tasks)
	service.UseTaskRunner(&recoveryRunner{})
	return store, db, tasks, service
}

func TestHostMaintenanceDiscoveryIgnoresUnrelatedCorruptAccountMetadata(t *testing.T) {
	for _, operation := range []string{"rate", "base-url"} {
		t.Run(operation, func(t *testing.T) {
			store, db, tasks, service := maintenanceDiscoveryFixture(t)
			if _, err := db.Exec(`UPDATE accounts SET metadata_json='invalid' WHERE id='30'`); err != nil {
				t.Fatal(err)
			}
			var taskID string
			var err error
			if operation == "rate" {
				taskID, err = service.EnqueueHostAccountRateSync(t.Context(), "https://SELECTED.EXAMPLE/", "test")
			} else {
				taskID, err = service.EnqueueHostAccountBaseURLSync(t.Context(), "https://SELECTED.EXAMPLE/", "test")
			}
			if err != nil || taskID == "" {
				t.Fatalf("host discovery failed: task=%q err=%v", taskID, err)
			}
			task, err := tasks.Get(t.Context(), taskID)
			if err != nil || task.Status != "queued" || task.Result["requested"] != json.Number("2") {
				t.Fatalf("queued scope=%+v err=%v", task, err)
			}
			if operation == "rate" {
				recovery, err := tasks.RecoveryTask(t.Context(), taskID)
				if err != nil {
					t.Fatal(err)
				}
				var request struct {
					AccountIDs []string `json:"account_ids"`
				}
				if err := taskstore.RecoveryInput(recovery, &request); err != nil || !reflect.DeepEqual(request.AccountIDs, []string{"2", "10"}) {
					t.Fatalf("recovery scope=%v err=%v", request.AccountIDs, err)
				}
			}
			if _, err := store.BoundAccountsForMaintenance(t.Context(), []string{"30"}); err == nil {
				t.Fatal("execution read accepted corrupt selected metadata")
			}
		})
	}
}

func TestHostMaintenanceDiscoveryWithNoMatchDoesNotCreateTask(t *testing.T) {
	_, _, tasks, service := maintenanceDiscoveryFixture(t)
	for _, host := range []string{"missing.example", " ", "///"} {
		for _, enqueue := range []func(context.Context, string, string) (string, error){service.EnqueueHostAccountRateSync, service.EnqueueHostAccountBaseURLSync} {
			id, err := enqueue(t.Context(), host, "test")
			if err != nil || id != "" {
				t.Fatalf("host=%q task=%q err=%v", host, id, err)
			}
		}
	}
	rows, err := tasks.List(t.Context(), nil)
	if err != nil || len(rows) != 0 {
		t.Fatalf("unexpected tasks=%v err=%v", rows, err)
	}
}

func TestMaintenanceDiscoveryAllAndRotatingBatchesPersistExactScope(t *testing.T) {
	_, db, tasks, service := maintenanceDiscoveryFixture(t)
	if _, err := db.Exec(`UPDATE accounts SET metadata_json='invalid' WHERE id='30'`); err != nil {
		t.Fatal(err)
	}
	for _, fixture := range []struct {
		all  bool
		want []string
	}{
		{true, []string{"2", "10", "30"}},
		{false, []string{"2", "10"}},
		{false, []string{"30", "2"}},
		{false, []string{"10", "30"}},
	} {
		var id string
		var err error
		if fixture.all {
			id, err = service.EnqueueAllAccountRateSync(t.Context(), "test")
		} else {
			id, err = service.EnqueueAccountRateSyncBatch(t.Context(), 2, 0, "test")
		}
		if err != nil || id == "" {
			t.Fatalf("enqueue task=%q err=%v", id, err)
		}
		task, err := tasks.RecoveryTask(t.Context(), id)
		if err != nil {
			t.Fatal(err)
		}
		var request struct {
			AccountIDs []string `json:"account_ids"`
		}
		if err := taskstore.RecoveryInput(task, &request); err != nil || !reflect.DeepEqual(request.AccountIDs, fixture.want) {
			t.Fatalf("scope=%v err=%v want=%v", request.AccountIDs, err, fixture.want)
		}
		task.Status = "succeeded"
		if err := tasks.Save(t.Context(), task); err != nil {
			t.Fatal(err)
		}
	}
}

func TestMaintenanceDiscoveryOutsideFullModeDoesNotReadBindingsOrQueue(t *testing.T) {
	store, db, tasks, service := maintenanceDiscoveryFixture(t)
	if _, err := store.SetMode(t.Context(), runtimepolicy.Monitoring); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DROP TABLE bindings`); err != nil {
		t.Fatal(err)
	}
	for _, enqueue := range []func() (string, error){
		func() (string, error) {
			return service.EnqueueHostAccountRateSync(t.Context(), "selected.example", "test")
		},
		func() (string, error) {
			return service.EnqueueHostAccountBaseURLSync(t.Context(), "selected.example", "test")
		},
		func() (string, error) { return service.EnqueueAllAccountRateSync(t.Context(), "test") },
		func() (string, error) { return service.EnqueueAccountRateSyncBatch(t.Context(), 1, 0, "test") },
	} {
		id, err := enqueue()
		if err != nil || id != "" {
			t.Fatalf("task=%q err=%v", id, err)
		}
	}
	rows, err := tasks.List(t.Context(), nil)
	if err != nil || len(rows) != 0 {
		t.Fatalf("unexpected tasks=%v err=%v", rows, err)
	}
}

func TestMaintenanceDiscoveryReadFailureDoesNotCreateTask(t *testing.T) {
	_, db, tasks, service := maintenanceDiscoveryFixture(t)
	if _, err := db.Exec(`DROP TABLE bindings`); err != nil {
		t.Fatal(err)
	}
	for _, enqueue := range []func() (string, error){
		func() (string, error) {
			return service.EnqueueHostAccountRateSync(t.Context(), "selected.example", "test")
		},
		func() (string, error) {
			return service.EnqueueHostAccountBaseURLSync(t.Context(), "selected.example", "test")
		},
		func() (string, error) { return service.EnqueueAllAccountRateSync(t.Context(), "test") },
		func() (string, error) { return service.EnqueueAccountRateSyncBatch(t.Context(), 1, 0, "test") },
	} {
		id, err := enqueue()
		if id != "" || err == nil || !strings.Contains(err.Error(), "读取上游绑定账号失败") {
			t.Fatalf("task=%q err=%v", id, err)
		}
	}
	rows, err := tasks.List(t.Context(), nil)
	if err != nil || len(rows) != 0 {
		t.Fatalf("unexpected tasks=%v err=%v", rows, err)
	}
}
