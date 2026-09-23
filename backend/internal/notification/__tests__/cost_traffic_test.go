package notification_test

import (
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/notification"
	"strings"
	"testing"
)

func TestCostTrafficNotificationIncludesOnlyRequestGroupAndEvidence(t *testing.T) {
	incident := business.AlertIncident{IncidentKey: "console:cost-traffic:41:平价", EventType: "account.cost_traffic", ObjectKind: "account", ObjectID: "41", GroupNames: []string{"平价", "旗舰"}, CauseCode: "COST_TRAFFIC:当前账号倍率 0.3 ≥ 分组倍率 0.3；最近 5 分钟 2 次实际请求；最近调用 2026-09-15 17:00:00（北京时间）", Status: "firing"}
	message := notification.BatchMessage([]business.AlertIncident{incident})
	for _, detail := range []string{"无利润／亏损流量", "分组：平价", "当前账号倍率 0.3", "分组倍率 0.3", "2 次实际请求", "最近调用"} {
		if !strings.Contains(message, detail) {
			t.Fatalf("missing %s: %s", detail, message)
		}
	}
	if strings.Contains(message, "旗舰") {
		t.Fatalf("unrelated group included: %s", message)
	}
	incident.Status = "recovered"
	message = notification.BatchMessage([]business.AlertIncident{incident})
	if !strings.Contains(message, "近期未再检测到账号倍率大于等于分组倍率的流量") || strings.Contains(message, "2 次实际请求") {
		t.Fatalf("recovery reused firing evidence: %s", message)
	}
}

func TestCostTrafficNotificationUsesPreciseClassification(t *testing.T) {
	for _, tc := range []struct{ cause, label string }{
		{"COST_TRAFFIC_LOSS:当前账号倍率 0.32 > 分组倍率 0.25", "亏损流量"},
		{"COST_TRAFFIC_BREAK_EVEN:当前账号倍率 0.25 = 分组倍率 0.25", "无利润流量"},
	} {
		for _, status := range []string{"firing", "recovered"} {
			incident := business.AlertIncident{EventType: "account.cost_traffic", CauseCode: tc.cause, Status: status}
			message := notification.BatchMessage([]business.AlertIncident{incident})
			if !strings.Contains(message, tc.label) || strings.Contains(message, "无利润／亏损") {
				t.Fatalf("imprecise notification: %s", message)
			}
		}
	}
}
