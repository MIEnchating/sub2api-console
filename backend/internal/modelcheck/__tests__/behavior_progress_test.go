package modelcheck_test

import (
	"context"
	"io"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

type behaviorProgressStore struct {
	modelcheck.TaskStore
	mu        sync.Mutex
	snapshots []taskstore.Task
	started   chan taskstore.Task
	once      sync.Once
}

func (store *behaviorProgressStore) Save(ctx context.Context, task taskstore.Task) error {
	if err := store.TaskStore.Save(ctx, task); err != nil {
		return err
	}
	if task.Status != "running" || task.Result["phase"] != "testing" {
		return nil
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	store.snapshots = append(store.snapshots, task)
	if active, ok := task.Result["active"].(map[int]map[string]any); ok && len(active) > 0 && task.Result["completed"] == 0 {
		store.once.Do(func() { store.started <- task })
	}
	return nil
}

func TestBehaviorProgressPublishesActiveAccountsBeforeAnyRequestCompletes(t *testing.T) {
	t.Parallel()
	release := make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	f := setup(t, 3, "openai", func(w http.ResponseWriter, request *http.Request) {
		_, _ = io.Copy(io.Discard, request.Body)
		select {
		case <-release:
			_, _ = io.WriteString(w, `{"output_text":"21"}`)
		case <-request.Context().Done():
		}
	})
	t.Cleanup(unblock)
	store := &behaviorProgressStore{TaskStore: f.tasks, started: make(chan taskstore.Task, 1)}
	service, err := modelcheck.New(store, credentials{}, f.catalog, credentials{})
	if err != nil {
		t.Fatal(err)
	}
	service.UseTaskRunner(f.runner)
	queued, err := service.Enqueue(t.Context(), modelcheck.Request{
		AccountIDs: []string{"1", "2", "3"}, Models: []string{"gpt-6-astra"}, Rounds: 1, TimeoutSeconds: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case first := <-store.started:
		persisted, err := f.tasks.Get(t.Context(), queued.ID)
		if err != nil || persisted.Status != "running" || persisted.Result["active"] == nil {
			t.Fatalf("active account was not persisted while upstream was blocked: %+v %v", persisted, err)
		}
		if first.Result["completed"] != 0 || first.Progress != 5 || len(first.Result["tests"].([]map[string]any)) != 0 {
			t.Fatalf("request start invented completed results: %+v", first)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("active accounts were not published before upstream completion")
	}
	unblock()
	final := finished(t, f)
	if final.Status != "succeeded" || final.Result["completed"] != 3 {
		t.Fatalf("completion lost results: %+v", final)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	seenCompleted := map[string]bool{}
	for _, snapshot := range store.snapshots {
		rows := snapshot.Result["tests"].([]map[string]any)
		if len(rows) != snapshot.Result["completed"] {
			t.Fatalf("previous result snapshot changed: %+v", snapshot.Result)
		}
		for _, row := range rows {
			seenCompleted[row["account_id"].(string)] = true
		}
		activeRows, _ := snapshot.Result["active"].(map[int]map[string]any)
		for _, active := range activeRows {
			if seenCompleted[active["account_id"].(string)] {
				t.Fatalf("completed account reappeared as active: %+v", snapshot.Result)
			}
		}
	}
}
