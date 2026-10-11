package management_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

type discoveryBenchmarkRunner struct{}

func (discoveryBenchmarkRunner) Go(func(context.Context)) error             { return nil }
func (discoveryBenchmarkRunner) GoTask(string, func(context.Context)) error { return nil }
func (discoveryBenchmarkRunner) CancelTask(string) bool                     { return false }

func BenchmarkHostMaintenanceEnqueueWithLargeMetadata(b *testing.B) {
	_, db, tasks, service := maintenanceDiscoveryFixture(b)
	for _, statement := range []string{
		`WITH RECURSIVE n(x) AS (VALUES(100) UNION ALL SELECT x+1 FROM n WHERE x<1096)
		INSERT INTO accounts(id,name,metadata_json,updated_at) SELECT CAST(x AS TEXT),'bulk','{}','now' FROM n`,
		`INSERT INTO bindings(local_account_id,upstream_host,upstream_key_id,upstream_key_name,local_group,updated_at)
		SELECT id,'unrelated.example','key-'||id,'fixture','default','now' FROM accounts WHERE CAST(id AS INTEGER)>=100`,
		`INSERT INTO binding_identities(binding_id,upstream_id,upstream_key_id,updated_at)
		SELECT id,'unrelated',upstream_key_id,'now' FROM bindings WHERE CAST(local_account_id AS INTEGER)>=100`,
	} {
		if _, err := db.Exec(statement); err != nil {
			b.Fatal(err)
		}
	}
	metadata := `{"platform":"openai","notes":"` + strings.Repeat("fixture ", 1024) + `"}`
	if _, err := db.Exec(`UPDATE accounts SET metadata_json=?`, metadata); err != nil {
		b.Fatal(err)
	}
	service.UseTaskRunner(discoveryBenchmarkRunner{})
	b.ReportAllocs()
	for b.Loop() {
		id, err := service.EnqueueHostAccountRateSync(b.Context(), "selected.example", "benchmark")
		if err != nil || id == "" {
			b.Fatalf("task=%q err=%v", id, err)
		}
		b.StopTimer()
		task, err := tasks.Get(b.Context(), id)
		if err != nil || task.Result["requested"] != json.Number("2") {
			b.Fatalf("queued scope=%+v err=%v", task, err)
		}
		task.Status = "succeeded"
		if err := tasks.Save(b.Context(), task); err != nil {
			b.Fatal(err)
		}
		b.StartTimer()
	}
}
