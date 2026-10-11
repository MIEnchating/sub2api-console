package business_test

import (
	"context"
	"testing"
	"time"
)

func BenchmarkAccountStabilityRead(b *testing.B) {
	for _, snapshot := range []bool{false, true} {
		name := "cached-independent-read"
		if snapshot {
			name = "consistent-snapshot"
		}
		b.Run(name, func(b *testing.B) {
			store, db := accountOwnershipFixture(b, 20)
			_, err := db.Exec(`WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<20000)
INSERT INTO account_stability_samples(account_id,source,evidence_key,observed_at,outcome,usage_known)
SELECT CAST(10001+(x%20) AS TEXT),'traffic',CAST(x AS TEXT),?,'passed',1 FROM n`,
				time.Now().UTC().Add(-time.Minute).Format("2006-01-02T15:04:05.000000000Z"))
			if err != nil {
				b.Fatal(err)
			}
			read := func(ctx context.Context) error {
				accounts, err := store.Accounts(ctx)
				if err == nil && (len(accounts) != 21 || accounts[1].Stability.Long.Samples != 1000) {
					b.Fatal("benchmark lost account stability data")
				}
				return err
			}
			if err := read(b.Context()); err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			for b.Loop() {
				if snapshot {
					err = store.WithReadSnapshot(b.Context(), read)
				} else {
					err = read(b.Context())
				}
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
