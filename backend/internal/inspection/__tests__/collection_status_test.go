package inspection_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/evidence"
	"github.com/MIEnchating/sub2api-console/backend/internal/inspection"
	"github.com/MIEnchating/sub2api-console/backend/internal/routing"
	"github.com/MIEnchating/sub2api-console/backend/internal/runtimepolicy"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

type collectionTarget struct{ baseURL string }

func (target collectionTarget) TargetSettings(context.Context) (configstore.TargetSettings, error) {
	return configstore.TargetSettings{BaseURL: target.baseURL, AdminKey: "isolated-test-key", TimeoutSeconds: 2}, nil
}

func TestInspectionReportsCollectionAvailability(t *testing.T) {
	for _, scenario := range []struct {
		name       string
		failedIDs  map[string]bool
		cached     bool
		probeAge   time.Duration
		wantStatus string
	}{
		{name: "all traffic reads fail without fallback reports failed", failedIDs: map[string]bool{"41": true, "42": true}, wantStatus: "failed"},
		{name: "one traffic read fails reports partial", failedIDs: map[string]bool{"41": true}, wantStatus: "partial"},
		{name: "failed reads with usable cache report partial", failedIDs: map[string]bool{"41": true, "42": true}, cached: true, wantStatus: "partial"},
		{name: "failed reads with fresh cached probe report partial", failedIDs: map[string]bool{"41": true, "42": true}, probeAge: time.Minute, wantStatus: "partial"},
		{name: "cached probe freshness is independent of probe interval", failedIDs: map[string]bool{"41": true, "42": true}, probeAge: 10 * time.Minute, wantStatus: "partial"},
		{name: "expired cached probe cannot cover failed reads", failedIDs: map[string]bool{"41": true, "42": true}, probeAge: 20 * time.Minute, wantStatus: "failed"},
		{name: "empty traffic with probes disabled succeeds", wantStatus: "succeeded"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			ctx := t.Context()
			store, err := business.Open(filepath.Join(t.TempDir(), "business.sqlite3"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = store.Close() })
			if err := store.Bootstrap(ctx); err != nil {
				t.Fatal(err)
			}
			if _, err := store.SetMode(ctx, runtimepolicy.Monitoring); err != nil {
				t.Fatal(err)
			}
			if _, err := store.UpdatePolicy(ctx, map[string]any{
				"advanced_policy": map[string]any{
					"probe": map[string]any{"enabled": false}, "recovery": map[string]any{"enabled": false},
				},
			}, "test"); err != nil {
				t.Fatal(err)
			}
			if _, err := store.SyncManagementSnapshot(ctx,
				[]map[string]any{
					{"id": json.Number("41"), "name": "first", "groups": []any{json.Number("7")}, "schedulable": true, "status": "active"},
					{"id": json.Number("42"), "name": "second", "groups": []any{json.Number("7")}, "schedulable": true, "status": "active"},
				}, []map[string]any{{"id": json.Number("7"), "name": "test-group"}}, "test"); err != nil {
				t.Fatal(err)
			}
			now := time.Now().UTC()
			if scenario.cached {
				if _, err := store.PersistTrafficSamples(ctx, []business.TrafficSample{{
					AccountID: "41", GroupName: "test-group", EvidenceKey: "cached-success", Result: "通过",
					ObservedAt: now.Add(-time.Second).Format(time.RFC3339Nano), Payload: map[string]any{"status_code": 200, "request_id": "cached-success"},
				}}); err != nil {
					t.Fatal(err)
				}
			}
			if err := store.MarkInspectionTask(ctx, "upstream-sync", now); err != nil {
				t.Fatal(err)
			}
			if scenario.probeAge != 0 {
				if _, err := store.PersistProbeSamples(ctx, []business.ProbeSample{{
					AccountID: "41", GroupName: "test-group", Result: "通过", SampleCount: 1, Attempts: 1,
					ObservedAt: now.Add(-scenario.probeAge).Format(time.RFC3339Nano),
				}}); err != nil {
					t.Fatal(err)
				}
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				if scenario.failedIDs[request.URL.Query().Get("account_id")] {
					http.Error(w, "test collection endpoint unavailable", http.StatusServiceUnavailable)
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"items": []any{}, "total": 0}})
			}))
			t.Cleanup(server.Close)
			tasks, err := taskstore.Open(filepath.Join(t.TempDir(), "tasks.sqlite3"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = tasks.Close() })
			runner := inspection.NewRunner(store, collectionTarget{baseURL: server.URL}, evidence.New(store, nil), routing.NewService(store), nil, nil, nil, tasks)
			result, err := runner.Run(ctx, inspection.RunRequest{Automatic: true, Actor: "test"})
			if err != nil {
				t.Fatal(err)
			}
			if result.TaskID == nil {
				t.Fatal("inspection did not run")
			}
			task, err := tasks.Get(ctx, *result.TaskID)
			if err != nil {
				t.Fatal(err)
			}
			if result.Status != scenario.wantStatus || task.Status != scenario.wantStatus {
				t.Fatalf("collection availability not reflected in task: result=%+v task=%+v", result, task)
			}
			if (task.Result["routing"] != nil) != (scenario.wantStatus != "failed") {
				t.Fatalf("routing did not follow evidence availability: %+v", task)
			}
			if len(scenario.failedIDs) > 0 {
				collected, ok := task.Result["evidence"].(map[string]any)
				if !ok || collected["source_errors"] == nil || result.Error == nil || *result.Error == "" {
					t.Fatalf("collection failure lost its details: result=%+v task=%+v", result, task)
				}
			}
		})
	}
}
