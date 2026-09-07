package newapimanagement

import (
	"encoding/json"
	"testing"
)

func TestChannelResultPreservesNumericStableIDWithoutPrecisionLoss(t *testing.T) {
	for _, field := range []string{"id", "channel_id"} {
		t.Run(field, func(t *testing.T) {
			result := publicChannelResult(map[string]any{field: json.Number("9007199254740993")}, "channel")
			if result["id"] != "9007199254740993" {
				t.Fatalf("numeric channel identity lost: %#v", result)
			}
		})
	}
}
