package routingwrite

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/runtimepolicy"
)

type modeSwitchRoutingRepository struct {
	*business.Store
}

func (repository *modeSwitchRoutingRepository) AcquireMutationLease(ctx context.Context, ownerID string, resources []string, now time.Time, ttl time.Duration) (bool, error) {
	if _, err := repository.Store.SetMode(ctx, runtimepolicy.Monitoring); err != nil {
		return false, err
	}
	return repository.Store.AcquireMutationLease(ctx, ownerID, resources, now, ttl)
}

func TestRoutingModeChangeDuringLeaseAcquisitionPreventsRemoteOperations(t *testing.T) {
	for _, operation := range []string{"apply", "restore"} {
		t.Run(operation, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "routing-mode.sqlite3")
			store, err := business.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = store.Close() })
			ctx := context.Background()
			if err := store.Bootstrap(ctx); err != nil {
				t.Fatal(err)
			}
			if _, err := store.SetMode(ctx, runtimepolicy.Full); err != nil {
				t.Fatal(err)
			}
			if _, err := store.SyncManagementSnapshot(ctx, []map[string]any{{
				"id": json.Number("41"), "name": "mode-switch", "priority": json.Number("10"),
				"groups": []any{json.Number("7")},
			}}, []map[string]any{{"id": json.Number("7"), "name": "codex"}}, "test"); err != nil {
				t.Fatal(err)
			}
			priority := int64(20)
			if err := store.CaptureRoutingBaseline(ctx, business.RoutingBaseline{
				AccountID: "41", Priority: &priority, TargetFingerprint: routingTargetFingerprint(testRoutingTarget),
			}); err != nil {
				t.Fatal(err)
			}
			admin := &concurrentWriteAdmin{dbPath: path, state: map[string]any{"id": json.Number("41"), "priority": int64(10)}}
			service := newTestService(&modeSwitchRoutingRepository{Store: store}, admin)
			var result Result
			if operation == "restore" {
				result, err = service.RestoreControl(ctx, "operator")
				if err == nil || !strings.Contains(err.Error(), "完全模式") {
					t.Errorf("restore must reject restricted mode: result=%#v err=%v", result, err)
				}
			} else {
				result, err = service.Apply(ctx, map[string]business.AccountRoutingTarget{
					"41": {AccountID: "41", Priority: &priority, GroupNames: []string{"codex"}},
				}, "scheduler")
				if err != nil || !result.CalculationOnly || result.Mode != runtimepolicy.Monitoring {
					t.Errorf("apply must fall back to calculation-only mode: result=%#v err=%v", result, err)
				}
			}
			admin.mu.Lock()
			defer admin.mu.Unlock()
			if admin.accounts != 0 || admin.mutates != 0 || admin.deletes != 0 || result.RemoteWrite {
				t.Errorf("restricted mode reached remote operations: accounts=%d mutates=%d deletes=%d result=%#v", admin.accounts, admin.mutates, admin.deletes, result)
			}
		})
	}
}
