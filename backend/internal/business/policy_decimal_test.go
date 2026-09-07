package business

import (
	"context"
	"testing"
)

func TestPolicyChangeThresholdRejectsNonDecimalOrUnboundedValues(t *testing.T) {
	for _, value := range []string{"1/2", "0x1", "1e-1001"} {
		t.Run(value, func(t *testing.T) {
			store := openPolicyStore(t)
			_, err := store.UpdatePolicy(context.Background(), map[string]any{"change_threshold": value}, "operator")
			if err == nil {
				t.Fatalf("unsafe threshold %q accepted", value)
			}
		})
	}
}
