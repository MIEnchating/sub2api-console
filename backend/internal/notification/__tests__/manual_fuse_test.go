package notification_test

import (
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/notification"
)

func TestManualFuseNotificationDoesNotClaimConsecutiveFailures(t *testing.T) {
	for _, cause := range []string{"MANUAL_FUSE", "ROUTING_BREAKER:人工熔断"} {
		t.Run(cause, func(t *testing.T) {
			message := notification.BatchMessage([]business.AlertIncident{{
				EventType: "account.routing_breaker", ObjectKind: "account", ObjectID: "998",
				CauseCode: cause, Status: "firing",
			}})
			if strings.Contains(message, "连续失败") || !strings.Contains(message, "人工熔断，等待手动解除") {
				t.Fatalf("manual fuse was reported as an automatic failure: %s", message)
			}
		})
	}
}
