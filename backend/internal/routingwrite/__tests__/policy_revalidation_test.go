package routingwrite_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/routingwrite"
	"github.com/MIEnchating/sub2api-console/backend/internal/runtimepolicy"
)

type policyChangingLeaseStore struct {
	*business.Store
	patch map[string]any
}

func (store *policyChangingLeaseStore) AcquireMutationLease(ctx context.Context, ownerID string, resources []string, now time.Time, ttl time.Duration) (bool, error) {
	if store.patch != nil {
		patch := store.patch
		store.patch = nil
		if _, err := store.UpdatePolicy(ctx, patch, "operator"); err != nil {
			return false, err
		}
	}
	return store.Store.AcquireMutationLease(ctx, ownerID, resources, now, ttl)
}

func TestRegularWriteSkipsWithoutFailureWhenPolicyChangesWhileAcquiringLease(t *testing.T) {
	for _, scenario := range []struct {
		name  string
		patch map[string]any
	}{
		{name: "automatic concurrency disabled", patch: map[string]any{"auto_apply": map[string]any{"concurrency": false}}},
		{name: "group excluded from scheduling", patch: map[string]any{"excluded_group_ids": []any{"7"}}},
		{name: "monitoring selected", patch: map[string]any{"mode": runtimepolicy.Monitoring}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			_, fixture := newUpstreamCapacityWriteFixture(t, writeCapacityPointer(10), 4, 4, true)
			repository := &policyChangingLeaseStore{Store: fixture.store, patch: scenario.patch}
			desired := int64(6)
			result, err := routingwrite.New(routingTarget{url: fixture.serverURL}, repository).Apply(t.Context(), map[string]business.AccountRoutingTarget{
				"41": {AccountID: "41", GroupNames: []string{"capacity-group"}, Concurrency: &desired},
			}, "scheduler")
			fixture.mu.Lock()
			defer fixture.mu.Unlock()
			if fixture.states["41"]["concurrency"] != 4 || result.RemoteWrite {
				t.Fatalf("old policy still authorized a write: state=%v result=%+v err=%v", fixture.states["41"], result, err)
			}
			if err != nil || result.Failed != 0 || len(result.Results) != 1 {
				t.Fatalf("policy change must defer the stale target without failing: result=%+v err=%v", result, err)
			}
			item := result.Results[0]
			if !item.Skipped || item.Error != nil || item.Reason == nil || !strings.Contains(*item.Reason, "重新计算调度") {
				t.Fatalf("policy change must explain why a fresh calculation is needed: %+v", item)
			}
			audit, err := fixture.store.AuditEvents(t.Context(), nil, false)
			if err != nil || len(audit) != 1 || audit[0].State != "skipped" || audit[0].Writeback || audit[0].Error != nil {
				t.Fatalf("policy deferral must be audited without claiming a write: %+v err=%v", audit, err)
			}
		})
	}
}
