package pricing_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/pricing"
)

func BenchmarkRevenueReads(b *testing.B) {
	for _, fixture := range []struct {
		name        string
		hosts       int
		keysPerHost int
		delay       time.Duration
	}{
		{name: "two-endpoints-20ms", hosts: 2, keysPerHost: 4, delay: 20 * time.Millisecond},
		{name: "512-accounts", hosts: 8, keysPerHost: 64},
	} {
		b.Run(fixture.name, func(b *testing.B) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if fixture.delay > 0 {
					timer := time.NewTimer(fixture.delay)
					defer timer.Stop()
					select {
					case <-timer.C:
					case <-r.Context().Done():
						return
					}
				}
				response := revenueUpstreamResponse
				if r.URL.Path == "/api/v1/admin/usage/stats" {
					response = revenueLocalResponse
				}
				_, _ = io.WriteString(w, response)
			}))
			b.Cleanup(server.Close)
			store, targets := revenueReadFixture(b, server.URL, server.URL, fixture.hosts, fixture.keysPerHost)
			service := pricing.New(store, targets, nil)
			accounts := fixture.hosts * fixture.keysPerHost
			b.ReportAllocs()
			for b.Loop() {
				report, err := service.CalculateRevenue(b.Context(), pricing.RevenueRequest{Date: "2020-01-01"}, "benchmark")
				if err != nil || report.Comparable != accounts || len(report.Issues) != 0 {
					b.Fatalf("report comparable=%d issues=%v err=%v", report.Comparable, report.Issues, err)
				}
			}
		})
	}
}
