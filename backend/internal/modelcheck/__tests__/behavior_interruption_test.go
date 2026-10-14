package modelcheck_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskcontext"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskrunner"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

type interruptAfterBehaviorResult struct {
	modelcheck.TaskStore
	once      sync.Once
	interrupt func(taskstore.Task)
}

func (s *interruptAfterBehaviorResult) Save(ctx context.Context, task taskstore.Task) error {
	if err := s.TaskStore.Save(ctx, task); err != nil {
		return err
	}
	if task.Status == "running" && task.Result["completed"] == 1 {
		s.once.Do(func() { s.interrupt(task) })
	}
	return nil
}

func TestInterruptedBehaviorCheckPreservesCompletedResults(t *testing.T) {
	for _, tc := range []struct {
		name   string
		cause  error
		status string
	}{
		{"manual cancellation", context.Canceled, "cancelled"},
		{"execution failure", errors.New("isolated lease lost"), "failed"},
		{"service shutdown", taskcontext.ErrInterrupted, "failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := setup(t, 4, "openai", func(w http.ResponseWriter, request *http.Request) {
				_, _ = io.Copy(io.Discard, request.Body)
				if strings.HasPrefix(request.URL.Path, "/blocked/") {
					<-request.Context().Done()
					return
				}
				_, _ = io.WriteString(w, `{"output_text":"21"}`)
			})
			for index := 1; index < len(f.catalog.rows); index++ {
				row := &f.catalog.rows[index]
				baseURL := *row.BaseURL + "/blocked"
				row.BaseURL = &baseURL
				f.catalog.details[row.ID].BaseURL = &baseURL
			}
			parent, cancel := context.WithCancelCause(t.Context())
			defer cancel(context.Canceled)
			runner := taskrunner.New(parent)
			t.Cleanup(func() {
				ctx, stop := context.WithTimeout(context.Background(), 5*time.Second)
				defer stop()
				if err := runner.Shutdown(ctx); err != nil {
					t.Error(err)
				}
			})
			store := &interruptAfterBehaviorResult{TaskStore: f.tasks, interrupt: func(task taskstore.Task) {
				if tc.cause == context.Canceled {
					if !runner.CancelTask(task.ID) {
						t.Error("running behavior check was not cancellable")
					}
					return
				}
				cancel(tc.cause)
			}}
			service, err := modelcheck.New(store, credentials{}, f.catalog, credentials{})
			if err != nil {
				t.Fatal(err)
			}
			service.UseTaskRunner(runner)
			queued, err := service.Enqueue(t.Context(), modelcheck.Request{
				AccountIDs: []string{"1", "2", "3", "4"}, Models: []string{"gpt-6-astra"}, Rounds: 1, TimeoutSeconds: 5,
			})
			if err != nil {
				t.Fatal(err)
			}
			final := finished(t, f)
			if final.ID != queued.ID || final.Status != tc.status {
				t.Fatalf("wrong interruption state: %+v", final)
			}
			rows, ok := final.Result["tests"].([]map[string]any)
			if !ok || len(rows) == 0 || len(rows) >= 4 {
				t.Fatalf("interruption lost partial results: %+v", final.Result)
			}
			if rows[0]["account_id"] != "1" || rows[0]["verdict"] != "ANOMALY" {
				t.Fatalf("completed verdict was replaced: %+v", rows)
			}
			if final.Result["completed"] != len(rows) || final.Result["total"] != 4 || final.Result["credentials_persisted"] != true || final.Result["profile_fingerprint"] == nil {
				t.Fatalf("interruption lost execution metadata: %+v", final.Result)
			}
			if _, ok := final.Result["active"]; ok {
				t.Fatalf("terminal task still exposes active combinations: %+v", final.Result)
			}
			if final.Result["phase"] != tc.status {
				t.Fatalf("terminal task still exposes a running phase: %+v", final.Result)
			}
			persisted, err := f.tasks.Get(t.Context(), final.ID)
			if err != nil {
				t.Fatal(err)
			}
			persistedRows, ok := persisted.Result["tests"].([]any)
			if !ok || len(persistedRows) != len(rows) {
				t.Fatalf("persisted task lost results: %+v", persisted.Result)
			}
			if tc.cause == context.Canceled && final.Result["cancelled"] != true {
				t.Fatal("manual cancellation marker missing")
			}
			if tc.cause == taskcontext.ErrInterrupted && (final.Result["interrupted"] != true || final.Result["cancelled"] != nil) {
				t.Fatal("shutdown was confused with manual cancellation")
			}
			if tc.cause != context.Canceled && tc.cause != taskcontext.ErrInterrupted && final.Result["error"] != tc.cause.Error() {
				t.Fatalf("failure cause was lost: %+v", final.Result)
			}
		})
	}
}
