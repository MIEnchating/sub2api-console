package inspection_test

import (
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/evidence"
	"github.com/MIEnchating/sub2api-console/backend/internal/inspection"
	"github.com/MIEnchating/sub2api-console/backend/internal/routing"
	"github.com/MIEnchating/sub2api-console/backend/internal/runtimepolicy"
	"path/filepath"
	"testing"
	"time"
)

func TestBindingCleanupSchedulesIdleHeartbeatOnlyWhenEnabledInFullMode(t *testing.T) {
	for _, scenario := range []struct {
		name, mode    string
		enabled, want bool
	}{
		{"full enabled", runtimepolicy.Full, true, true},
		{"full disabled", runtimepolicy.Full, false, false},
		{"monitoring enabled", runtimepolicy.Monitoring, true, false},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			store, err := business.Open(filepath.Join(t.TempDir(), "inspection.db"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = store.Close() })
			if err := store.Bootstrap(t.Context()); err != nil {
				t.Fatal(err)
			}
			if _, err := store.SetMode(t.Context(), scenario.mode); err != nil {
				t.Fatal(err)
			}
			if _, err := store.UpdatePolicy(t.Context(), map[string]any{"advanced_policy": map[string]any{"binding_cleanup": map[string]any{"enabled": scenario.enabled}, "probe": map[string]any{"enabled": false}, "recovery": map[string]any{"enabled": false}}}, "test"); err != nil {
				t.Fatal(err)
			}
			if _, err := routing.NewService(store).Calculate(t.Context(), routing.Scope{}, true); err != nil {
				t.Fatal(err)
			}
			now := time.Now().UTC()
			for _, name := range []string{"traffic", "upstream-sync"} {
				if err := store.MarkInspectionTask(t.Context(), name, now); err != nil {
					t.Fatal(err)
				}
			}
			runner := inspection.NewRunner(store, nil, evidence.New(store, nil), nil, nil, nil, nil, nil)
			preview, err := runner.Preview(t.Context(), now)
			if err != nil {
				t.Fatal(err)
			}
			for _, operation := range preview.Operations {
				if operation.Operation == "routing_calculation" {
					if operation.Due != scenario.want {
						t.Fatalf("unexpected cleanup scheduling: %+v", preview)
					}
					return
				}
			}
			t.Fatal("missing routing calculation preview")
		})
	}
}
