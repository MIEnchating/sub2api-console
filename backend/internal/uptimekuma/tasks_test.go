package uptimekuma

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/taskrunner"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

type observedTaskStore struct {
	store    *taskstore.Store
	terminal chan taskstore.Task
}

func (s *observedTaskStore) Save(ctx context.Context, task taskstore.Task) error {
	if err := s.store.Save(ctx, task); err != nil {
		return err
	}
	if task.Status == "succeeded" || task.Status == "failed" {
		s.terminal <- task
	}
	return nil
}
func TestConfigTasksPersistSafeSuccessAndFailureResults(t *testing.T) {
	for _, valid := range []bool{true, false} {
		t.Run(map[bool]string{true: "success", false: "failure"}[valid], func(t *testing.T) {
			f := newFixture(t)
			store, err := taskstore.Open(filepath.Join(t.TempDir(), "test-tasks.sqlite"))
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			runner := taskrunner.NewBounded(context.Background(), 1)
			defer runner.Shutdown(context.Background())
			observed := &observedTaskStore{store: store, terminal: make(chan taskstore.Task, 1)}
			service := NewTasks(f.service, observed, runner)
			key := "test-key"
			if !valid {
				key = "wrong-secret"
			}
			queued, err := service.Save(context.Background(), ConfigInput{BaseURL: f.server.URL, APIKey: key, Username: "admin", Password: "test-password"})
			if err != nil {
				t.Fatal(err)
			}
			if queued.Status != "queued" || queued.ID == "" {
				t.Fatalf("missing queued task")
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			select {
			case <-observed.terminal:
			case <-ctx.Done():
				t.Fatal("task did not complete")
			}
			final, err := store.Get(ctx, queued.ID)
			if err != nil {
				t.Fatal(err)
			}
			want := "succeeded"
			if !valid {
				want = "failed"
			}
			if final.Status != want || final.Progress != 100 || final.Result["request_id"] != queued.ID {
				t.Fatalf("task result: %#v", final)
			}
			data, _ := json.Marshal(final)
			for _, secret := range []string{key, "test-password", "test-jwt"} {
				if strings.Contains(string(data), secret) {
					t.Fatal("task leaked credentials")
				}
			}
		})
	}
}

func TestMonitorTaskPersistsAcknowledgedTargetID(t *testing.T) {
	f := newFixture(t)
	c := f.configure(t, true)
	store, err := taskstore.Open(filepath.Join(t.TempDir(), "tasks.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	runner := taskrunner.NewBounded(context.Background(), 1)
	defer runner.Shutdown(context.Background())
	observed := &observedTaskStore{store: store, terminal: make(chan taskstore.Task, 1)}
	s := NewTasks(f.service, observed, runner)
	queued, err := s.Write(context.Background(), 0, WriteInput{Action: "create", ConfigRevision: c.Revision, Monitor: MonitorInput{Name: "测试服务", Type: "http", URL: "https://monitor.example/health", Interval: 60}})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	select {
	case <-observed.terminal:
	case <-ctx.Done():
		t.Fatal("task did not complete")
	}
	final, err := store.Get(ctx, queued.ID)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(final.Result)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		MonitorID int64  `json:"monitor_id"`
		Action    string `json:"action"`
	}
	if err := json.Unmarshal(encoded, &result); err != nil {
		t.Fatal(err)
	}
	if final.Status != "succeeded" || result.MonitorID != 20 || result.Action != "create" {
		t.Fatalf("result: %#v", final)
	}
}

func TestStoppedRunnerRecordsFailureWithoutCallingRemote(t *testing.T) {
	f := newFixture(t)
	store, err := taskstore.Open(filepath.Join(t.TempDir(), "tasks.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	runner := taskrunner.New(context.Background())
	if err := runner.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	observed := &observedTaskStore{store: store, terminal: make(chan taskstore.Task, 1)}
	s := NewTasks(f.service, observed, runner)
	_, err = s.Save(context.Background(), ConfigInput{BaseURL: f.server.URL, APIKey: "test-key"})
	if PublicError(err).Code != "kuma_task_unavailable" {
		t.Fatalf("error: %v", err)
	}
	select {
	case failed := <-observed.terminal:
		if failed.Status != "failed" {
			t.Fatalf("status: %s", failed.Status)
		}
	default:
		t.Fatal("failed task was not persisted")
	}
	stored, _ := f.store.UptimeKuma(context.Background())
	if stored.APIKey != "" {
		t.Fatal("stopped task changed config")
	}
}
