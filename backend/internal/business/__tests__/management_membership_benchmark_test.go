package business_test

import (
	"fmt"
	"testing"
)

func BenchmarkManagementSnapshotUnchangedMemberships(b *testing.B) {
	store, _ := pricingProjectionFixture(b)
	groups := make([]map[string]any, 20)
	for index := range groups {
		groups[index] = map[string]any{"id": fmt.Sprint(index + 1), "name": fmt.Sprintf("group-%d", index+1)}
	}
	accounts := make([]map[string]any, 500)
	for index := range accounts {
		accounts[index] = map[string]any{
			"id": fmt.Sprint(index + 1), "name": fmt.Sprintf("account-%d", index+1), "rate_multiplier": "0.1",
			"group_ids": []any{fmt.Sprint(index%20 + 1), fmt.Sprint((index+1)%20 + 1)},
		}
	}
	if _, err := store.SyncManagementSnapshot(b.Context(), accounts, groups, "benchmark"); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		result, err := store.SyncManagementSnapshot(b.Context(), accounts, groups, "benchmark")
		if err != nil {
			b.Fatal(err)
		}
		if result.Accounts != 500 || result.GroupLinks != 1000 {
			b.Fatalf("incomplete snapshot: %+v", result)
		}
	}
}
