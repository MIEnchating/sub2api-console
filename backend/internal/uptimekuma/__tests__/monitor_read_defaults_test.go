package uptimekuma_test

import (
	"encoding/json"
	"fmt"
	"strconv"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/uptimekuma"
)

func TestMonitorReadPreservesOptionalFieldDefaults(t *testing.T) {
	for _, tc := range []struct {
		name, fields       string
		timeout, retries   int
		method             string
		ignoreTLS, headers bool
	}{
		{"missing", ``, 16, 60, "GET", false, false},
		{"null", `,"timeout":null,"retryInterval":null,"method":null,"ignoreTls":null,"headers":null`, 16, 60, "GET", false, false},
		{"native", `,"timeout":30,"retryInterval":90,"method":"POST","ignoreTls":true,"headers":"private-secret"`, 30, 90, "POST", true, true},
		{"legacy numbers", `,"timeout":"30","retryInterval":"90","ignoreTls":"1"`, 30, 90, "GET", true, false},
		{"zero", `,"timeout":0,"retryInterval":0,"ignoreTls":0`, 0, 0, "GET", false, false},
		{"invalid optional types", `,"timeout":{},"retryInterval":[],"method":42,"ignoreTls":{},"headers":false`, 16, 60, "GET", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := json.RawMessage(`{"id":19,"name":"服务","type":"http"` + tc.fields + `}`)
			f := newResourceValidationFixture(t, map[string]json.RawMessage{"19": raw}, map[string]json.RawMessage{})
			list, err := f.service.Resources(t.Context(), "status-pages")
			if err != nil || len(list.Monitors) != 1 {
				t.Fatalf("monitor read failed: %v", err)
			}
			m := list.Monitors[0]
			o := m.Options
			if m.ID != 19 || m.Revision == "" || o == nil {
				t.Fatal("missing monitor identity/options")
			}
			if o.Timeout != tc.timeout || o.RetryInterval != tc.retries || o.Method != tc.method || o.IgnoreTLS != tc.ignoreTLS || o.HeadersConfigured != tc.headers {
				t.Fatalf("optional field behavior changed: %+v", o)
			}
			if o.Headers != "" || o.AuthPassword != "" {
				t.Fatal("private options exposed")
			}
			if o.DNSRecordType != "A" || o.DNSResolver != "1.1.1.1" || o.AuthMethod != "none" || o.MaxRedirects != 10 || len(o.AcceptedStatusCodes) != 1 || o.AcceptedStatusCodes[0] != "200-299" || len(o.NotificationIDs) != 0 {
				t.Fatalf("missing-field defaults changed: %+v", o)
			}
		})
	}
}

func TestMonitorReadRejectsInvalidRequiredFieldTypes(t *testing.T) {
	for _, fields := range []string{`"id":"19","name":"服务","type":"http"`, `"id":19,"name":42,"type":"http"`, `"id":19,"name":"服务","type":"http","active":1`} {
		t.Run(fields, func(t *testing.T) {
			f := newResourceValidationFixture(t, map[string]json.RawMessage{"19": json.RawMessage(`{` + fields + `}`)}, map[string]json.RawMessage{})
			_, err := f.service.Resources(t.Context(), "status-pages")
			if err == nil || uptimekuma.PublicError(err).Code != "kuma_protocol_error" {
				t.Fatalf("invalid monitor accepted: %v", err)
			}
		})
	}
}

func BenchmarkMonitorDirectoryDefaults(b *testing.B) {
	for _, count := range []int{1, 1000} {
		b.Run(fmt.Sprintf("monitors=%d", count), func(b *testing.B) {
			monitors := make(map[string]json.RawMessage, count)
			for id := 1; id <= count; id++ {
				monitors[strconv.Itoa(id)] = json.RawMessage(fmt.Sprintf(`{"id":%d,"name":"服务","type":"http","interval":60,"active":true,"url":"https://isolated.test/health"}`, id))
			}
			f := newResourceValidationFixture(b, monitors, map[string]json.RawMessage{})
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				list, err := f.service.Resources(b.Context(), "status-pages")
				if err != nil || len(list.Monitors) != count {
					b.Fatalf("monitor directory failed: %v", err)
				}
			}
		})
	}
}
