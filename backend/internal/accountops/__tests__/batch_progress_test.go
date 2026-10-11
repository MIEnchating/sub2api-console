package accountops_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountops"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

type batchProgressTarget struct{ settingsTarget }

func (batchProgressTarget) AccountCreationSettings(context.Context) (configstore.AccountCreationSettings, error) {
	return configstore.AccountCreationSettings{
		Default: configstore.AccountCreationPolicy{PoolModeRetryCount: 3, PoolModeRetryStatusCodes: []int{429}},
	}, nil
}

type batchProgressStore struct {
	*taskstore.Store
	updates chan taskstore.Task
}

func (s *batchProgressStore) Save(ctx context.Context, task taskstore.Task) error {
	if err := s.Store.Save(ctx, task); err != nil {
		return err
	}
	persisted, err := s.Store.Get(ctx, task.ID)
	if err != nil {
		return err
	}
	s.updates <- persisted
	return nil
}

func newBatchProgressService(t *testing.T, handler http.HandlerFunc) (*accountops.Service, *settingsRunner, *batchProgressStore) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "accounts.db")
	repository, err := business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repository.Close() })
	if err := repository.Bootstrap(t.Context()); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO accounts(id,name,schedulable,priority,load_factor,concurrency,metadata_json,updated_at)
		VALUES('41','first',1,20,'1',3,'{}','2026-10-10T00:00:00Z'),
		('42','second',1,20,'1',3,'{}','2026-10-10T00:00:00Z')`)
	_ = db.Close()
	if err != nil {
		t.Fatal(err)
	}
	store, err := taskstore.Open(filepath.Join(t.TempDir(), "tasks.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	tasks := &batchProgressStore{Store: store, updates: make(chan taskstore.Task, 10)}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	runner := &settingsRunner{}
	service := accountops.New(batchProgressTarget{settingsTarget{endpoint: server.URL}}, repository, tasks)
	service.UseTaskRunner(runner)
	return service, runner, tasks
}

func batchProgressResponse(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if strings.HasSuffix(r.URL.Path, "/models/sync-upstream") {
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []string{"model-a"}})
		return
	}
	id := 41
	if strings.HasSuffix(r.URL.Path, "/42") {
		id = 42
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{
		"id": id, "group_ids": []int{}, "credentials": map[string]any{
			"model_mapping": map[string]string{}, "pool_mode": false,
			"pool_mode_retry_count": 3, "pool_mode_retry_status_codes": []int{429},
		},
	}})
}

func TestAccountBatchProgressBeforeFirstOutcomeRemainsZero(t *testing.T) {
	for _, operation := range []string{"discovery", "pool-mode"} {
		t.Run(operation, func(t *testing.T) {
			entered := make(chan struct{}, 1)
			release := make(chan struct{})
			service, runner, tasks := newBatchProgressService(t, func(w http.ResponseWriter, r *http.Request) {
				entered <- struct{}{}
				select {
				case <-release:
					batchProgressResponse(w, r)
				case <-r.Context().Done():
				}
			})
			var queued taskstore.Task
			var err error
			if operation == "discovery" {
				queued, err = service.EnqueueModelDiscovery(t.Context(), []string{"41"}, "test")
			} else {
				queued, err = service.EnqueuePoolModeSync(t.Context(), []string{"41"}, "test")
			}
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(t.Context())
			done := make(chan struct{})
			go func() { defer close(done); runner.run(ctx) }()
			t.Cleanup(func() { close(release); cancel(); <-done })
			select {
			case <-entered:
			case <-time.After(3 * time.Second):
				t.Fatal("task did not reach the remote request")
			}
			active, err := tasks.Get(t.Context(), queued.ID)
			if err != nil {
				t.Fatal(err)
			}
			if active.Status != "running" || active.Progress != 0 || active.Result["completed"] != json.Number("0") {
				t.Fatalf("task before any outcome = %+v", active)
			}
		})
	}
}

func TestAccountBatchProgressAfterOneOfTwoOutcomesIsHalf(t *testing.T) {
	for _, operation := range []string{"discovery", "pool-mode"} {
		t.Run(operation, func(t *testing.T) {
			release := make(chan struct{})
			var first sync.Once
			var firstAccountPath string
			service, runner, tasks := newBatchProgressService(t, func(w http.ResponseWriter, r *http.Request) {
				first.Do(func() { firstAccountPath = r.URL.Path })
				if r.URL.Path != firstAccountPath && !strings.HasPrefix(r.URL.Path, firstAccountPath+"/") {
					select {
					case <-release:
					case <-r.Context().Done():
						return
					}
				}
				batchProgressResponse(w, r)
			})
			var err error
			if operation == "discovery" {
				_, err = service.EnqueueModelDiscovery(t.Context(), []string{"41", "42"}, "test")
			} else {
				_, err = service.EnqueuePoolModeSync(t.Context(), []string{"41", "42"}, "test")
			}
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(t.Context())
			done := make(chan struct{})
			go func() { defer close(done); runner.run(ctx) }()
			t.Cleanup(func() { cancel(); <-done })
			t.Cleanup(func() { close(release) })
			var observed []taskstore.Task
			deadline := time.After(3 * time.Second)
			for {
				select {
				case update := <-tasks.updates:
					observed = append(observed, update)
					if update.Status != "running" || update.Result["completed"] != json.Number("1") {
						continue
					}
					if update.Progress != 50 {
						t.Fatalf("one of two outcomes progress = %d, want 50", update.Progress)
					}
					return
				case <-deadline:
					t.Fatalf("first outcome was not persisted: %+v", observed)
				}
			}
		})
	}
}

func TestAccountBatchProgressAfterAllOutcomesReachesCompletion(t *testing.T) {
	for _, operation := range []string{"discovery", "pool-mode"} {
		for _, outcome := range []string{"succeeded", "partial"} {
			t.Run(operation+"/"+outcome, func(t *testing.T) {
				service, runner, tasks := newBatchProgressService(t, func(w http.ResponseWriter, r *http.Request) {
					if outcome == "partial" && strings.HasSuffix(r.URL.Path, "/42") {
						http.Error(w, `{"message":"account unavailable"}`, http.StatusBadRequest)
						return
					}
					batchProgressResponse(w, r)
				})
				var queued taskstore.Task
				var err error
				if operation == "discovery" {
					queued, err = service.EnqueueModelDiscovery(t.Context(), []string{"41", "42"}, "test")
				} else {
					queued, err = service.EnqueuePoolModeSync(t.Context(), []string{"41", "42"}, "test")
				}
				if err != nil {
					t.Fatal(err)
				}
				runner.run(t.Context())
				finished, err := tasks.Get(t.Context(), queued.ID)
				if err != nil {
					t.Fatal(err)
				}
				if finished.Status != outcome || finished.Progress != 100 || finished.Result["completed"] != json.Number("2") {
					t.Fatalf("task after all outcomes = %+v", finished)
				}
				wantFailed := json.Number("0")
				if outcome == "partial" {
					wantFailed = json.Number("1")
				}
				if finished.Result["failed"] != wantFailed {
					t.Fatalf("failed accounts = %v, want %s", finished.Result["failed"], wantFailed)
				}
			})
		}
	}
}
