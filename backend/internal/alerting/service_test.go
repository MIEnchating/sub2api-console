package alerting

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskrunner"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
	_ "modernc.org/sqlite"
)

type fakeDeliverer struct{ result business.AlertDeliveryResult }

type recordingTaskStore struct {
	tasks []taskstore.Task
}

func (store *recordingTaskStore) Save(_ context.Context, task taskstore.Task) error {
	store.tasks = append(store.tasks, task)
	return nil
}

func (f fakeDeliverer) Deliver(context.Context, bool) (business.AlertDeliveryResult, error) {
	return f.result, nil
}

func TestEnqueuePersistsTerminalTaskWhenRunnerIsStopping(t *testing.T) {
	store := &recordingTaskStore{}
	runner := taskrunner.New(context.Background())
	runner.Cancel()
	service := NewTaskService(nil, store)
	service.UseTaskRunner(runner)

	if _, err := service.Enqueue(context.Background()); !errors.Is(err, taskrunner.ErrStopped) {
		t.Fatalf("Enqueue error = %v, want stopped runner", err)
	}
	if len(store.tasks) != 2 {
		t.Fatalf("saved tasks = %d, want queued and terminal writes", len(store.tasks))
	}
	queued, terminal := store.tasks[0], store.tasks[1]
	if queued.Status != "queued" {
		t.Fatalf("initial task status = %q, want queued", queued.Status)
	}
	if terminal.ID != queued.ID || terminal.Status != "cancelled" || terminal.Progress != 100 {
		t.Fatalf("terminal task = %#v", terminal)
	}
	if terminal.Result["cancelled"] != true || terminal.Result["error"] != taskrunner.ErrStopped.Error() {
		t.Fatalf("terminal result = %#v", terminal.Result)
	}
}

func TestEvaluationCreatesAndRecoversAuthIncident(t *testing.T) {
	path := filepath.Join(t.TempDir(), "alerts.sqlite3")
	repository, err := business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repository.Close() })
	if err := repository.Bootstrap(context.Background()); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO upstreams(host,base_url,upstream_type,auth_status,metadata_json,updated_at)
		VALUES('api.example','https://api.example','sub2api','认证过期','{}','now')`); err != nil {
		t.Fatal(err)
	}
	service := New(repository, fakeDeliverer{result: business.AlertDeliveryResult{Skipped: 1, Configured: true, MessageIDs: []string{}}})
	result, err := service.Evaluate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Findings != 1 || result.RunKey == "" || result.EventID >= 0 || result.Status != "succeeded" {
		t.Fatalf("unexpected evaluation: %#v", result)
	}
	var status string
	if err := db.QueryRow(`SELECT status FROM alert_incidents WHERE incident_key='console:auth:api.example'`).Scan(&status); err != nil || status != "firing" {
		t.Fatalf("auth incident not created: status=%q err=%v", status, err)
	}
	if _, err := db.Exec(`UPDATE upstreams SET auth_status='已鉴权' WHERE host='api.example'`); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Evaluate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT status FROM alert_incidents WHERE incident_key='console:auth:api.example'`).Scan(&status); err != nil || status != "recovered" {
		t.Fatalf("auth incident not recovered: status=%q err=%v", status, err)
	}
}
