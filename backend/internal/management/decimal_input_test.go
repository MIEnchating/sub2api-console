package management

import "testing"

func TestManagementMultiplierRejectsUnboundedAndNonDecimalInput(t *testing.T) {
	for _, value := range []string{"1e1001", "1/2", "0x10"} {
		t.Run(value, func(t *testing.T) {
			if _, err := managementAccountMultiplier(map[string]any{"rate_multiplier": value}); err == nil {
				t.Fatal("invalid remote ratio accepted")
			}
		})
	}
}
