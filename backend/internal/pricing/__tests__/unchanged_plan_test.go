package pricing_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/mutationguard"
	"github.com/MIEnchating/sub2api-console/backend/internal/pricing"
	"github.com/MIEnchating/sub2api-console/backend/internal/runtimepolicy"
)

func TestUnchangedPlanCompletesWhileAnotherTaskHoldsManagementTarget(t *testing.T) {
	for _, test := range []struct {
		name      string
		accounts  []map[string]any
		unchanged int
		skipped   int
	}{
		{name: "empty catalog", accounts: []map[string]any{}},
		{
			name: "unchanged and incomplete accounts",
			accounts: []map[string]any{
				{"id": json.Number("41"), "name": "unchanged", "platform": "openai", "rate_multiplier": json.Number("0.1"), "group_ids": []any{json.Number("8")}},
				{"id": json.Number("42"), "name": "missing cost", "platform": "openai", "group_ids": []any{json.Number("8")}},
			},
			unchanged: 1,
			skipped:   1,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			store, err := business.Open(filepath.Join(t.TempDir(), "pricing.sqlite3"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = store.Close() })
			if err := store.Bootstrap(ctx); err != nil {
				t.Fatal(err)
			}
			if _, err := store.SetMode(ctx, runtimepolicy.Full); err != nil {
				t.Fatal(err)
			}
			_, err = store.SyncManagementSnapshot(ctx, test.accounts, []map[string]any{
				{"id": json.Number("8"), "name": "standard", "platform": "openai", "rate_multiplier": json.Number("0.2")},
				{"id": json.Number("9"), "name": "premium", "platform": "openai", "rate_multiplier": json.Number("0.3")},
			}, "test")
			if err != nil {
				t.Fatal(err)
			}
			service := pricing.New(store, testTarget{configstore.TargetSettings{
				BaseURL: "http://127.0.0.1:1", AdminKey: "isolated-test-key", TimeoutSeconds: 1,
			}}, nil)
			_, err = service.UpdateConfig(ctx, pricing.Config{
				Enabled: true, ProfitMargin: 0.25, ExchangeGroupSets: [][]string{{"8", "9"}},
				IntervalSeconds: 120, WriteConcurrency: 1,
			}, "test")
			if err != nil {
				t.Fatal(err)
			}
			_, release, err := mutationguard.Acquire(ctx, store, mutationguard.ManagementTarget())
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := release(); err != nil {
					t.Error(err)
				}
			}()
			applyCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			defer cancel()

			result, err := service.ApplyNow(applyCtx, "test")

			if err != nil {
				t.Fatalf("unchanged plan waited for an unrelated management mutation: %v", err)
			}
			if result.Requested != len(test.accounts) || result.Unchanged != test.unchanged || result.Skipped != test.skipped {
				t.Fatalf("unexpected unchanged plan summary: %#v", result)
			}
			if result.Changed != 0 || result.Failed != 0 || result.RemoteWrite || len(result.Items) != 0 {
				t.Fatalf("unchanged plan reported account writes: %#v", result)
			}
		})
	}
}
