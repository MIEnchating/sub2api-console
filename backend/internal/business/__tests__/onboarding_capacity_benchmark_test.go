package business_test

import (
	"strings"
	"testing"
)

func BenchmarkOnboardingCapacityWithUnrelatedAccounts(b *testing.B) {
	store, db := accountOwnershipFixture(b, 2000)
	if _, err := db.Exec(`UPDATE upstreams SET metadata_json=?`, `{"concurrency_limit":12,"notes":"`+strings.Repeat("fixture ", 512)+`"}`); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		capacity, err := store.OnboardingCapacity(b.Context(), "upstream-a")
		if err != nil || len(capacity.Accounts) != 1 || capacity.Accounts[0].ID != "41" {
			b.Fatalf("capacity=%+v error=%v", capacity, err)
		}
	}
}
