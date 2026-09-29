package accountdelete_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountdelete"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskrunner"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

type deletionTasks struct {
	*taskstore.Store
	finished chan taskstore.Task
}

func (tasks *deletionTasks) Save(ctx context.Context, task taskstore.Task) error {
	if err := tasks.Store.Save(ctx, task); err != nil {
		return err
	}
	if task.Status == "succeeded" || task.Status == "failed" || task.Status == "partial" {
		tasks.finished <- task
	}
	return nil
}

func manualDeletionFixture(t *testing.T, failDeletion bool) (*accountdelete.Service, *business.Store, <-chan taskstore.Task) {
	t.Helper()
	var mu sync.Mutex
	deleted := map[string]bool{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		id := filepath.Base(r.URL.Path)
		if r.URL.Path != "/api/v1/admin/accounts/"+id || (id != "41" && id != "42") {
			t.Errorf("unexpected deletion target: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		switch r.Method {
		case http.MethodDelete:
			if failDeletion {
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"message":"deletion denied"}`))
				return
			}
			deleted[id] = true
			_, _ = w.Write([]byte(`{"success":true}`))
		case http.MethodGet:
			if deleted[id] {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"message":"not found"}`))
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"id": id}})
		default:
			t.Errorf("deletion must not restore manual settings: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	t.Cleanup(server.Close)
	dir := t.TempDir()
	repository, err := business.Open(filepath.Join(dir, "business.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repository.Close() })
	if err := repository.Bootstrap(t.Context()); err != nil {
		t.Fatal(err)
	}
	_, err = repository.SyncCompleteManagementSnapshot(t.Context(), []map[string]any{
		{"id": "41", "name": "manual-first", "priority": 1, "schedulable": true, "concurrency": 100, "group_ids": []any{1}},
		{"id": "42", "name": "manual-second", "priority": 2, "schedulable": true, "concurrency": 100, "group_ids": []any{1}},
		{"id": "43", "name": "manual-peer", "priority": 3, "schedulable": true, "concurrency": 100, "group_ids": []any{1}},
	}, []map[string]any{{"id": 1, "name": "manual-group"}}, "test")
	if err != nil {
		t.Fatal(err)
	}
	for i, id := range []string{"41", "42", "43"} {
		if _, err := repository.AssignManualPriority(t.Context(), id, int64(i+1), "100", 100, false, "test"); err != nil {
			t.Fatal(err)
		}
	}
	private, err := configstore.Open(filepath.Join(dir, "private.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = private.Close() })
	if err := private.ConfigureTarget(t.Context(), server.URL, "test-admin-key", 2); err != nil {
		t.Fatal(err)
	}
	store, err := taskstore.Open(filepath.Join(dir, "tasks.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	tasks := &deletionTasks{Store: store, finished: make(chan taskstore.Task, 1)}
	runner := taskrunner.New(t.Context())
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := runner.Shutdown(ctx); err != nil {
			t.Error(err)
		}
	})
	service := accountdelete.New(repository, private, nil, tasks)
	service.UseTaskRunner(runner)
	return service, repository, tasks.finished
}

func TestConfirmedDeletionRemovesManualAccountsWithoutReleasingControlFirst(t *testing.T) {
	for _, batch := range []bool{false, true} {
		name := "single"
		if batch {
			name = "batch"
		}
		t.Run(name, func(t *testing.T) {
			service, repository, finished := manualDeletionFixture(t, false)
			ids := []string{"41"}
			if batch {
				ids = append(ids, "42")
			}
			preview, err := service.PreviewBatch(t.Context(), ids)
			if err != nil {
				t.Fatal(err)
			}
			if batch {
				confirmations := make([]accountdelete.Confirmation, 0, len(preview.Accounts))
				for _, item := range preview.Accounts {
					confirmations = append(confirmations, accountdelete.Confirmation{AccountID: item.AccountID, ManagementBaseURL: item.ManagementBaseURL, Binding: item.Binding})
				}
				_, err = service.EnqueueBatch(t.Context(), confirmations, "test")
			} else {
				item := preview.Accounts[0]
				_, err = service.Enqueue(t.Context(), item.AccountID, item.Binding, item.ManagementBaseURL, "test")
			}
			if err != nil {
				t.Fatal(err)
			}
			select {
			case task := <-finished:
				if task.Status != "succeeded" {
					t.Fatalf("manual deletion failed: %+v", task)
				}
			case <-time.After(10 * time.Second):
				t.Fatal("deletion task did not finish")
			}
			for _, id := range ids {
				if _, err := repository.Account(t.Context(), id); !errors.Is(err, sql.ErrNoRows) {
					t.Fatalf("deleted account %s remains: %v", id, err)
				}
			}
			controls, err := repository.ManualPriorityControls(t.Context(), nil)
			if err != nil {
				t.Fatal(err)
			}
			for _, id := range ids {
				if _, found := controls[id]; found {
					t.Fatalf("deleted account %s still reserves a manual position", id)
				}
			}
			if _, found := controls["43"]; !found {
				t.Fatal("unselected account lost manual control")
			}
		})
	}
}

func TestRejectedRemoteDeletionPreservesManualControl(t *testing.T) {
	service, repository, _ := manualDeletionFixture(t, true)
	preview, err := service.Preview(t.Context(), "41")
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Delete(t.Context(), preview, "test")
	if err == nil || result.LocalProjectionDeleted {
		t.Fatalf("failed remote deletion was accepted: %+v %v", result, err)
	}
	account, err := repository.Account(t.Context(), "41")
	if err != nil || account.ManualPriority == nil || *account.ManualPriority != 1 {
		t.Fatalf("failed deletion lost manual control: %+v %v", account, err)
	}
}
