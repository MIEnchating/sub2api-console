package uptimekuma

import (
	"strings"
	"testing"
)

func TestMetricsReadStatusAndRedactCredentialURL(t *testing.T) {
	input := `# HELP monitor_status Monitor Status
# TYPE monitor_status gauge
monitor_status{monitor_id="19",monitor_name="智谱\"主线",monitor_type="http",monitor_url="https://user:password@monitor.example/health?token=secret"} 0
# TYPE monitor_response_time gauge
monitor_response_time{monitor_id="19",monitor_name="智谱\"主线",monitor_type="http"} -1
# TYPE monitor_uptime_ratio gauge
monitor_uptime_ratio{monitor_id="19",monitor_name="智谱\"主线",monitor_type="http",window="1d"} 0.98
`
	got, err := parseMetrics([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != 19 || got[0].Status == nil || *got[0].Status != 0 || got[0].Name != "智谱\"主线" {
		t.Fatalf("unexpected monitors: %#v", got)
	}
	if got[0].URL != "https://monitor.example/health" || !got[0].URLRedacted || got[0].ResponseTime != nil || got[0].Uptime == nil || *got[0].Uptime != 0.98 {
		t.Fatalf("incorrect metrics: %#v", got[0])
	}
}
func TestLegacyMetricsNeverInventManagementIDs(t *testing.T) {
	got, err := parseMetrics([]byte("# TYPE monitor_status gauge\nmonitor_status{monitor_name=\"same\",monitor_type=\"http\",monitor_url=\"https://a.example\"} 1\nmonitor_status{monitor_name=\"same\",monitor_type=\"http\",monitor_url=\"https://b.example\"} 0\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != 0 || got[1].ID != 0 || got[0].Key == got[1].Key {
		t.Fatalf("legacy identifiers: %#v", got)
	}
}
func TestMetricsEmptyAndInvalidResponses(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		valid      bool
	}{
		{"empty Kuma", "# HELP monitor_status Monitor Status\n# TYPE monitor_status gauge\n", true},
		{"dashboard HTML", "<html>login</html>", false},
		{"unrelated metrics", "process_cpu_seconds_total 1\n", false},
		{"invalid status", "# TYPE monitor_status gauge\nmonitor_status{monitor_name=\"test\",monitor_type=\"http\"} 9\n", false},
		{"invalid ID", "# TYPE monitor_status gauge\nmonitor_status{monitor_id=\"bad\",monitor_name=\"test\",monitor_type=\"http\"} 1\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseMetrics([]byte(tc.body))
			if (err == nil) != tc.valid {
				t.Fatalf("unexpected parse result: %v %#v", err, got)
			}
			if tc.valid && len(got) != 0 {
				t.Fatal("empty instance must return an empty list")
			}
		})
	}
}
func TestPublicURLNeverReturnsNonHTTPOrMalformedCredentials(t *testing.T) {
	for _, raw := range []string{"javascript:alert(1)", "https://x.example/%zz", "https://user:secret@x.example/path?key=secret#secret"} {
		value, _ := publicURL(raw)
		if strings.Contains(value, "secret") || strings.HasPrefix(value, "javascript:") {
			t.Fatalf("unsafe URL %q", value)
		}
	}
}
