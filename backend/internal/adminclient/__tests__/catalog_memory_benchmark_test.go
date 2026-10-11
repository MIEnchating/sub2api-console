package adminclient_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/adminclient"
)

// Compare live heap immediately before the last response. Both paths return the
// same minimal public projection; the legacy path first collects raw accounts.
// Forced GC makes this a retention measurement, not a latency benchmark.
func BenchmarkCatalogProjectionRetention(b *testing.B) {
	const total, pageSize = 1000, 50
	payloads := make([]string, total/pageSize)
	for page := range payloads {
		rows := make([]map[string]any, 0, pageSize)
		for index := 0; index < pageSize; index++ {
			id := page*pageSize + index + 1
			rows = append(rows, map[string]any{"id": id, "name": fmt.Sprintf("account-%d", id), "credentials": map[string]any{"access_token": strings.Repeat("test-only-", 410)}})
		}
		raw, err := json.Marshal(map[string]any{"data": map[string]any{"items": rows, "total": total}})
		if err != nil {
			b.Fatal(err)
		}
		payloads[page] = string(raw)
	}
	for _, streaming := range []bool{false, true} {
		name := "collect_then_project"
		if streaming {
			name = "visit_then_project"
		}
		b.Run(name, func(b *testing.B) {
			var startHeap, retainedTotal uint64
			client, err := adminclient.New(adminclient.Config{BaseURL: "https://management.test", AdminKey: "test-only", Attempts: 1}, responseTransport(func(r *http.Request) (*http.Response, error) {
				page, err := strconv.Atoi(r.URL.Query().Get("page"))
				if err != nil || page < 1 || page > len(payloads) {
					b.Fatal("invalid page")
				}
				if page == len(payloads) {
					runtime.GC()
					var stats runtime.MemStats
					runtime.ReadMemStats(&stats)
					if stats.HeapAlloc > startHeap {
						retainedTotal += stats.HeapAlloc - startHeap
					}
				}
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(payloads[page-1]))}, nil
			}))
			if err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				runtime.GC()
				var stats runtime.MemStats
				runtime.ReadMemStats(&stats)
				startHeap = stats.HeapAlloc
				result := make([]string, 0, total)
				consume := func(row map[string]any) error { result = append(result, row["name"].(string)); return nil }
				if streaming {
					err = client.VisitAccounts(b.Context(), consume)
				} else {
					var rows []map[string]any
					rows, err = client.Accounts(b.Context())
					if err == nil {
						for _, row := range rows {
							_ = consume(row)
						}
					}
				}
				if err != nil || len(result) != total || result[total-1] != "account-1000" {
					b.Fatalf("projection failed: count=%d err=%v", len(result), err)
				}
				runtime.KeepAlive(result)
			}
			b.ReportMetric(float64(retainedTotal)/float64(b.N), "retained-B/op")
		})
	}
}
