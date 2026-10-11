package uptimekuma_test

import (
	"fmt"
	"strings"
	"testing"
)

func BenchmarkMetricsSnapshot(b *testing.B) {
	for _, count := range []int{100, 1000} {
		for _, legacy := range []bool{false, true} {
			b.Run(fmt.Sprintf("monitors=%d/legacy=%t", count, legacy), func(b *testing.B) {
				var body strings.Builder
				body.WriteString("# HELP monitor_status Monitor Status\n")
				for _, metric := range []struct{ name, value string }{
					{"monitor_status", "1"}, {"monitor_response_time", "42.5"},
					{"monitor_cert_days_remaining", "90"}, {"monitor_uptime_ratio", "0.98"},
				} {
					fmt.Fprintf(&body, "# TYPE %s gauge\n", metric.name)
					for id := 1; id <= count; id++ {
						identity := fmt.Sprintf("monitor_id=\"%d\",", id)
						if legacy {
							identity = ""
						}
						fmt.Fprintf(&body, "%s{%smonitor_name=\"service-%d\",monitor_type=\"http\",monitor_url=\"https://monitor.example/%s/%d?key=private\",monitor_hostname=\"monitor.example\",monitor_port=\"443\",window=\"1d\"} %s\n",
							metric.name, identity, id, strings.Repeat("a", 128), id, metric.value)
					}
				}
				service := newMetricsFixture(b, body.String())
				b.ReportAllocs()
				b.SetBytes(int64(body.Len()))
				b.ResetTimer()
				for range b.N {
					snapshot, err := service.Snapshot(b.Context())
					if err != nil || len(snapshot.Monitors) != count {
						b.Fatalf("snapshot count=%d error=%v", len(snapshot.Monitors), err)
					}
				}
			})
		}
	}
}
