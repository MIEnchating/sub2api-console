package business

import "testing"

func TestStoredJSONObjectRejectsTrailingData(t *testing.T) {
	for _, raw := range []string{`{"scope":{}} {"scope":{"paused_account_ids":["11"]}}`, `{"scope":{}} trailing`} {
		t.Run(raw, func(t *testing.T) {
			if _, err := decodeJSONObject(raw); err == nil {
				t.Error("policy decoder accepted malformed stored document")
			}
			if _, err := decodeObject(raw); err == nil {
				t.Error("projection decoder accepted malformed stored document")
			}
		})
	}
}
