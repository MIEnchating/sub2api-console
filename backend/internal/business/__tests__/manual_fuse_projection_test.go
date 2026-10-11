package business_test

import (
	"encoding/json"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

func TestManualFuseFlagFollowsConfirmedControlInListAndDetail(t *testing.T) {
	for _, mode := range []string{"完全模式", "监控模式"} {
		t.Run(mode, func(t *testing.T) {
			store, _ := monitoringProjectionStore(t)
			if _, err := store.SetMode(t.Context(), mode); err != nil {
				t.Fatal(err)
			}
			for _, action := range []string{"initial", "fuse", "recover"} {
				if action != "initial" {
					if err := store.CommitAccountControlReadback(t.Context(), "41", action, "test", action == "recover", business.AccountOperation{
						OperationID: action, OperationType: "account.control", State: "succeeded", Phase: "readback", RemoteConfirmed: true, ReadbackConfirmed: true,
					}); err != nil {
						t.Fatal(err)
					}
				}
				detail, err := store.Account(t.Context(), "41")
				if err != nil {
					t.Fatal(err)
				}
				rows, err := store.Accounts(t.Context())
				if err != nil {
					t.Fatal(err)
				}
				if len(rows) != 1 {
					t.Fatalf("unexpected accounts: %d", len(rows))
				}
				for _, value := range []any{detail, rows[0]} {
					raw, err := json.Marshal(value)
					if err != nil {
						t.Fatal(err)
					}
					var payload map[string]any
					if err := json.Unmarshal(raw, &payload); err != nil {
						t.Fatal(err)
					}
					if payload["manual_fused"] != (action == "fuse") {
						t.Fatalf("%s: manual_fused=%v", action, payload["manual_fused"])
					}
				}
			}
		})
	}
}
