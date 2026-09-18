package accountworkbench_test

import (
	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"testing"
)

func TestMaintenanceClassifiesRefreshAndVerificationSignals(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value map[string]any
		kind  string
	}{
		{"structured auth", map[string]any{"status_code": 401}, "auth"},
		{"nested check auth", map[string]any{"result": map[string]any{"errors": []any{"HTTP 401: unauthorized"}}}, "auth"},
		{"invalid grant", map[string]any{"error": "invalid_grant"}, "auth"},
		{"rate limit before auth", map[string]any{"status": 429, "error": "token expired"}, "transient"},
		{"permission needs human", map[string]any{"errors": []any{"upstream error 403"}}, "manual"},
		{"permanent before auth", map[string]any{"status_code": 401, "message": "account_deactivated"}, "permanent"},
		{"credentials ignored", map[string]any{"status": "active", "credentials": map[string]any{"access_token": "invalid_grant"}}, "healthy"},
		{"network cooldown", map[string]any{"error": "connection reset"}, "transient"},
		{"unknown business failure", map[string]any{"success": false}, "manual"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := accountworkbench.ClassifyMaintenanceSignal(tc.value); got.Kind != tc.kind {
				t.Fatalf("condition=%+v", got)
			}
		})
	}
}
