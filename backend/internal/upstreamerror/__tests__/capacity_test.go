package upstreamerror_test

import (
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/upstreamerror"
)

func TestCapacityRecognizesErrorTextAndIgnoresUnrelatedJSONMetadata(t *testing.T) {
	for _, tt := range []struct {
		name, message string
		want          bool
	}{
		{"plain overload", "Our servers are currently overloaded.", true},
		{"plain whitespace", " The SERVICE\t is\n busy. ", true},
		{"normalized error code", "SERVICE_IS_BUSY", true},
		{"model capacity", "Selected model is at capacity.", true},
		{"escaped newline", `{"code":"server_error","message":"The service\nis busy."}`, true},
		{"unicode escape", `{"message":"The server\u0020is busy."}`, true},
		{"nested response", `{"response":{"error":{"message":"The service\tis busy.","status_code":503}}}`, true},
		{"code only", `{"error":{"type":"overloaded_error"}}`, true},
		{"string error", `{"error":"The service\nis busy."}`, true},
		{"json string", `"The service\nis busy."`, true},
		{"request ID is not error text", `{"message":"Generation rejected","request_id":"overloaded-request"}`, false},
		{"model name is not error text", `{"message":"Generation rejected","model":"overloaded-model"}`, false},
		{"nested metadata is not error text", `{"response":{"error":{"message":"Invalid request"},"metadata":{"message":"server is busy"}}}`, false},
		{"quota without capacity", `{"error":{"code":"insufficient_quota","message":"Quota exceeded"}}`, false},
		{"empty message", "", false},
		{"malformed excerpt retains plain matching", `{"message":"The service is busy`, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := upstreamerror.IsCapacity(tt.message); got != tt.want {
				t.Fatalf("IsCapacity(%q)=%t want=%t", tt.message, got, tt.want)
			}
		})
	}
}

func TestCapacityStopsAtBoundedErrorNesting(t *testing.T) {
	message := strings.Repeat(`{"error":`, 17) + `"The service is busy"` + strings.Repeat("}", 17)
	if upstreamerror.IsCapacity(message) {
		t.Fatal("deep error payload bypassed the bounded inspection")
	}
}
