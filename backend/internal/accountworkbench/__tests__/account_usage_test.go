package accountworkbench_test

import (
	"encoding/json"
	"testing"
)

func TestAccountUsagePreservesZeroAndOnlyReturnsValidatedSnapshotFields(t *testing.T) {
	service, _ := fixture(t, `{"data":{"items":[{"id":41,"platform":"openai","type":"oauth","extra":{"codex_5h_used_percent":0,"codex_5h_reset_at":"2099-01-01T05:00:00Z","codex_7d_used_percent":62.5,"codex_usage_updated_at":"2026-10-12T00:00:00Z","password":"private"}},{"id":42,"platform":"openai","type":"oauth","extra":{"codex_5h_used_percent":-1,"codex_7d_used_percent":"private","codex_usage_updated_at":"private"}}],"total":2}}`)
	accounts, err := service.Accounts(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(accounts)
	var rows []map[string]any
	if err := json.Unmarshal(raw, &rows); err != nil {
		t.Fatal(err)
	}
	usage, ok := rows[0]["usage"].(map[string]any)
	if !ok {
		t.Fatal("missing usage snapshot")
	}
	if usage["five_hour"].(map[string]any)["used_percent"] != float64(0) || usage["seven_day"].(map[string]any)["used_percent"] != 62.5 || usage["updated_at"] != "2026-10-12T00:00:00Z" {
		t.Fatalf("usage not preserved: %s", raw)
	}
	if rows[1]["usage"] != nil {
		t.Fatalf("invalid usage must remain unavailable: %s", raw)
	}
}

func TestAccountUsageLegacyWindowsUseExplicitDurationsAndResetFromSnapshotTime(t *testing.T) {
	service, _ := fixture(t, `{"data":{"items":[{"id":41,"platform":"openai","type":"oauth","extra":{"codex_primary_window_minutes":10080,"codex_primary_used_percent":75,"codex_secondary_window_minutes":300,"codex_secondary_used_percent":20,"codex_secondary_reset_after_seconds":3600,"codex_usage_updated_at":"2026-10-12T00:00:00Z"}},{"id":42,"platform":"openai","type":"oauth","extra":{"codex_primary_used_percent":10}}],"total":2}}`)
	accounts, err := service.Accounts(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(accounts)
	var rows []map[string]any
	_ = json.Unmarshal(raw, &rows)
	usage, ok := rows[0]["usage"].(map[string]any)
	if !ok {
		t.Fatal("missing legacy usage")
	}
	five := usage["five_hour"].(map[string]any)
	if five["used_percent"] != float64(20) || five["reset_at"] != "2026-10-12T01:00:00Z" || usage["seven_day"].(map[string]any)["used_percent"] != float64(75) {
		t.Fatalf("legacy windows swapped: %s", raw)
	}
	if rows[1]["usage"] != nil {
		t.Fatalf("unknown window was guessed: %s", raw)
	}
}
