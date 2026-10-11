package uptimekuma_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/MIEnchating/sub2api-console/backend/internal/uptimekuma"
)

func TestMetricsSnapshotPreservesLabelsValuesAndRedaction(t *testing.T) {
	body := `# HELP monitor_status Monitor Status
# TYPE monitor_status gauge
monitor_status{monitor_id="19",monitor_name="同名\"服务",monitor_type="http",monitor_url="https://user:private-key@monitor.example/health?token=private-key#private-key",extra="ignored"} 0
monitor_status{monitor_id="20",monitor_name="同名\"服务",monitor_type="http"} 3
# TYPE monitor_response_time gauge
monitor_response_time{monitor_id="19",monitor_name="同名\"服务",monitor_type="http"} 42.5
monitor_response_time{monitor_id="20",monitor_name="同名\"服务",monitor_type="http"} -1
# TYPE monitor_cert_days_remaining gauge
monitor_cert_days_remaining{monitor_id="19",monitor_name="同名\"服务",monitor_type="http"} -2
# TYPE monitor_uptime_ratio gauge
monitor_uptime_ratio{monitor_id="19",monitor_name="同名\"服务",monitor_type="http",window="1d"} 0.98
monitor_uptime_ratio{monitor_id="19",monitor_name="同名\"服务",monitor_type="http",window="30d"} 0.5
process_cpu_seconds_total 1
`
	snapshot, err := newMetricsFixture(t, body).Snapshot(t.Context())
	if err != nil || len(snapshot.Monitors) != 2 {
		t.Fatalf("snapshot count=%d error=%v", len(snapshot.Monitors), err)
	}
	first, second := snapshot.Monitors[0], snapshot.Monitors[1]
	if first.ID != 19 || first.Key != "id:19" || first.Name != "同名\"服务" || first.Type != "http" || !first.Active ||
		first.URL != "https://monitor.example/health" || !first.URLRedacted || first.Status == nil || *first.Status != 0 ||
		first.ResponseTime == nil || *first.ResponseTime != 42.5 || first.CertificateDays == nil || *first.CertificateDays != -2 ||
		first.Uptime == nil || *first.Uptime != 0.98 || second.ID != 20 || second.Status == nil || *second.Status != 3 || second.ResponseTime != nil {
		t.Fatalf("metric values or identities changed: %+v %+v", first, second)
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil || strings.Contains(string(encoded), "private-key") || strings.Contains(string(encoded), "isolated-metrics-key") {
		t.Fatal("public snapshot contains credentials or cannot be encoded")
	}
}

func TestMetricsSnapshotLegacyIdentityUsesHostnameAndPort(t *testing.T) {
	body := `# TYPE monitor_status gauge
monitor_status{monitor_name="同名",monitor_type="port",monitor_hostname="a.example",monitor_port="443"} 1
monitor_status{monitor_name="同名",monitor_type="port",monitor_hostname="a.example",monitor_port="8443"} 0
monitor_status{monitor_name="同名",monitor_type="port",monitor_hostname="b.example",monitor_port="443"} 2
# TYPE monitor_response_time gauge
monitor_response_time{monitor_name="同名",monitor_type="port",monitor_hostname="a.example",monitor_port="443"} 12
`
	snapshot, err := newMetricsFixture(t, body).Snapshot(t.Context())
	if err != nil || len(snapshot.Monitors) != 3 {
		t.Fatalf("legacy count=%d error=%v", len(snapshot.Monitors), err)
	}
	seen := map[string]bool{}
	responses := 0
	for _, monitor := range snapshot.Monitors {
		if monitor.ID != 0 || !strings.HasPrefix(monitor.Key, "metric:") || seen[monitor.Key] {
			t.Fatalf("legacy target invented or merged: %+v", monitor)
		}
		seen[monitor.Key] = true
		if monitor.ResponseTime != nil {
			responses++
			if *monitor.ResponseTime != 12 || monitor.Status == nil || *monitor.Status != 1 {
				t.Fatal("response time associated with another legacy target")
			}
		}
	}
	if responses != 1 {
		t.Fatalf("response values=%d, want 1", responses)
	}
}

func TestMetricsSnapshotRejectsInvalidResponsesWithoutPartialResults(t *testing.T) {
	for _, tc := range []struct{ name, body string }{
		{"unrelated", "process_cpu_seconds_total 1\n"},
		{"trailing malformed", snapshotMetrics + "invalid{broken\n"},
		{"wrong metric type", "# TYPE monitor_status counter\nmonitor_status{monitor_name=\"service\",monitor_type=\"http\"} 1\n"},
		{"missing identity", "# TYPE monitor_status gauge\nmonitor_status{monitor_id=\"19\",monitor_type=\"http\"} 1\n"},
		{"invalid stable ID", "# TYPE monitor_status gauge\nmonitor_status{monitor_id=\"0\",monitor_name=\"service\",monitor_type=\"http\"} 1\n"},
		{"fractional status", "# TYPE monitor_status gauge\nmonitor_status{monitor_name=\"service\",monitor_type=\"http\"} 1.5\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			snapshot, err := newMetricsFixture(t, tc.body).Snapshot(t.Context())
			if err == nil || uptimekuma.PublicError(err).Code != "kuma_protocol_error" || len(snapshot.Monitors) != 0 {
				t.Fatalf("invalid body accepted: count=%d error=%v", len(snapshot.Monitors), err)
			}
		})
	}
}

func TestMetricsSnapshotNonFiniteValuesRemainUnavailable(t *testing.T) {
	for _, value := range []string{"NaN", "+Inf", "-Inf"} {
		t.Run(value, func(t *testing.T) {
			var body strings.Builder
			for _, metric := range []string{"monitor_status", "monitor_response_time", "monitor_cert_days_remaining", "monitor_uptime_ratio"} {
				fmt.Fprintf(&body, "# TYPE %s gauge\n%s{monitor_id=\"19\",monitor_name=\"service\",monitor_type=\"http\",window=\"1d\"} %s\n", metric, metric, value)
			}
			snapshot, err := newMetricsFixture(t, body.String()).Snapshot(t.Context())
			if err != nil || len(snapshot.Monitors) != 1 {
				t.Fatalf("count=%d error=%v", len(snapshot.Monitors), err)
			}
			m := snapshot.Monitors[0]
			if m.Status != nil || m.ResponseTime != nil || m.CertificateDays != nil || m.Uptime != nil {
				t.Fatalf("non-finite values became available: %+v", m)
			}
		})
	}
}

type metricsResponseTransport struct{ body io.ReadCloser }

func (transport metricsResponseTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: transport.body, Request: r}, nil
}

type metricsResponseBody struct {
	io.Reader
	closed bool
}

func (body *metricsResponseBody) Close() error { body.closed = true; return nil }

func TestMetricsSnapshotEmptyDeclarationAcrossReadBoundaries(t *testing.T) {
	body := &metricsResponseBody{Reader: iotest.OneByteReader(strings.NewReader("# HELP monitor_status Monitor Status\n# TYPE monitor_status gauge\n"))}
	service := metricsService(t, "http://metrics.invalid", &http.Client{Transport: metricsResponseTransport{body}})
	snapshot, err := service.Snapshot(t.Context())
	if err != nil || len(snapshot.Monitors) != 0 || snapshot.Monitors == nil || !body.closed {
		t.Fatalf("empty snapshot count=%d closed=%v error=%v", len(snapshot.Monitors), body.closed, err)
	}
}

func TestMetricsSnapshotBodyReadFailureClosesResponseAndRejectsPartialData(t *testing.T) {
	body := &metricsResponseBody{Reader: io.MultiReader(strings.NewReader(snapshotMetrics), iotest.ErrReader(io.ErrUnexpectedEOF))}
	service := metricsService(t, "http://metrics.invalid", &http.Client{Transport: metricsResponseTransport{body}})
	snapshot, err := service.Snapshot(t.Context())
	if err == nil || uptimekuma.PublicError(err).Code != "kuma_protocol_error" || len(snapshot.Monitors) != 0 || !body.closed {
		t.Fatalf("partial response accepted: count=%d closed=%v error=%v", len(snapshot.Monitors), body.closed, err)
	}
}

func TestMetricsSnapshotEnforcesResponseSizeIncludingTrailingComments(t *testing.T) {
	for _, extra := range []int{0, 1} {
		t.Run(fmt.Sprintf("extra=%d", extra), func(t *testing.T) {
			const maxSize = 8 << 20
			prefix := "# HELP monitor_status Monitor Status\n# "
			body := prefix + strings.Repeat("x", maxSize-len(prefix)-1+extra) + "\n"
			snapshot, err := newMetricsFixture(t, body).Snapshot(t.Context())
			if extra == 0 {
				if err != nil || len(snapshot.Monitors) != 0 {
					t.Fatalf("limit-sized empty snapshot rejected: %v", err)
				}
			} else if err == nil || uptimekuma.PublicError(err).Code != "kuma_protocol_error" || len(snapshot.Monitors) != 0 {
				t.Fatalf("oversized snapshot accepted: %v", err)
			}
		})
	}
}
