package upstreamsync_test

import (
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

func TestDeletedUpstreamDoesNotSendBalanceRecovery(t *testing.T) {
	for _, scoped := range []bool{false, true} {
		name := "full evaluation"
		if scoped {
			name = "balance evaluation"
		}
		t.Run(name, func(t *testing.T) {
			fixture := newBalanceFixture(t, balanceReader{}, "deleted.example", "retained.example")
			if _, err := fixture.store.DeleteUpstreamProjection(t.Context(), "deleted.example", nil, business.UpstreamDeleteAudit{}); err != nil {
				t.Fatal(err)
			}
			if scoped {
				if err := fixture.alerts.EvaluateBalance(t.Context(), "deleted.example"); err != nil {
					t.Fatal(err)
				}
			} else if _, err := fixture.alerts.Evaluate(t.Context()); err != nil {
				t.Fatal(err)
			}
			if len(fixture.sender.messages) != 0 {
				t.Fatalf("deletion sent a recovery notification: %v", fixture.sender.messages)
			}
			var status, detail string
			if err := fixture.db.QueryRow(`SELECT status,delivery_status FROM alert_incidents WHERE object_id='deleted.example'`).Scan(&status, &detail); err != nil {
				t.Fatal(err)
			}
			if status != "closed" || detail != "上游已删除，告警已关闭" {
				t.Fatalf("deleted upstream alert status=%q detail=%q", status, detail)
			}
			if err := fixture.db.QueryRow(`SELECT status FROM alert_incidents WHERE object_id='retained.example'`).Scan(&status); err != nil {
				t.Fatal(err)
			}
			if status != "firing" {
				t.Fatalf("unrelated upstream alert changed to %q", status)
			}
		})
	}
}
