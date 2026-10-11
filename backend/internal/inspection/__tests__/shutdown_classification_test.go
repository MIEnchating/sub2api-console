package inspection_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/evidence"
	"github.com/MIEnchating/sub2api-console/backend/internal/inspection"
	"github.com/MIEnchating/sub2api-console/backend/internal/management"
	"github.com/MIEnchating/sub2api-console/backend/internal/routing"
	"github.com/MIEnchating/sub2api-console/backend/internal/runtimepolicy"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskcontext"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

func TestSchedulerShutdownRetainsInterruptedTaskForRecovery(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	store, err := business.Open(filepath.Join(t.TempDir(), "business.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetMode(ctx, runtimepolicy.Monitoring); err != nil {
		t.Fatal(err)
	}
	config := business.DefaultAutoInspectionConfig()
	config.Enabled = true
	if _, err := store.UpdateAutoInspectionConfig(ctx, config); err != nil {
		t.Fatal(err)
	}
	tasks, err := taskstore.Open(filepath.Join(t.TempDir(), "tasks.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer tasks.Close()
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case entered <- struct{}{}:
		default:
		}
		select {
		case <-r.Context().Done():
			return
		case <-release:
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"items": []any{}, "total": 0}})
	}))
	defer server.Close()
	defer close(release)
	target := managementTarget{collectionTarget{server.URL}}
	runner := inspection.NewRunner(store, target, evidence.New(store, nil), routing.NewService(store), nil, nil, nil, tasks, management.New(target, store, tasks))
	scheduler, err := inspection.NewScheduler(store, runner)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, err := scheduler.RunDue(ctx, time.Now().UTC(), true); done <- err }()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("inspection did not begin")
	}
	status, err := scheduler.Status(ctx)
	if err != nil || status.LastTaskID == nil {
		t.Fatalf("running status=%+v err=%v", status, err)
	}
	if err := scheduler.StopContext(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("inspection did not stop")
	}
	task, err := tasks.Get(ctx, *status.LastTaskID)
	if err != nil {
		t.Fatal(err)
	}
	if task.Status != "queued" || task.Result["interrupted"] != true || task.Result["cancelled"] == true || task.Progress == 100 {
		t.Fatalf("shutdown was treated as manual cancellation: %+v", task)
	}
	if _, err := tasks.RecoveryTask(ctx, task.ID); err != nil {
		t.Fatalf("interrupted task lost recovery parameters: %v", err)
	}
	history, err := store.InspectionHeartbeats(ctx, 20)
	if err != nil || len(history) != 1 || history[0].Status != "failed" || history[0].Error == nil || !strings.Contains(*history[0].Error, taskcontext.ErrInterrupted.Error()) {
		t.Fatalf("shutdown heartbeat=%+v err=%v", history, err)
	}
}
