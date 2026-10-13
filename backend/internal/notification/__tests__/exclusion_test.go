package notification_test

import (
	"context"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/notification"
	"testing"
)

type excludedDeliveryRepository struct{ *business.Store }

func (r excludedDeliveryRepository) UpstreamExcluded(context.Context, string) (bool, error) {
	return true, nil
}
func TestExclusionAfterNotificationPlanningPreventsDelivery(t *testing.T) {
	f := newUpstreamDeliveryFixture(t)
	service := notification.New(excludedDeliveryRepository{f.repository}, upstreamDeliverySettings{}, f.sender)
	result, err := service.Deliver(t.Context(), false)
	if err != nil || result.Attempted != 0 || len(f.sender.messages) != 0 {
		t.Fatalf("excluded delivery: %+v %v messages=%v", result, err, f.sender.messages)
	}
}
