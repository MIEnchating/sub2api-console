package routing

import (
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

func TestFreshSourceWindowSelectsNewestInstantAcrossTimestampFormats(t *testing.T) {
	now := time.Date(2026, 9, 7, 12, 0, 1, 0, time.UTC)
	for _, older := range []string{"2026-09-07T12:00:00Z", "2026-09-07T14:00:00+02:00"} {
		t.Run(older, func(t *testing.T) {
			samples := []business.RoutingSample{
				{AccountID: "41", Source: "traffic", ObservedAt: older, Result: "失败"},
				{AccountID: "41", Source: "traffic", ObservedAt: "2026-09-07T12:00:00.1Z", Result: "通过"},
			}
			selected := freshSourceSamples(samples, "traffic", now, time.Minute, 1)
			if len(selected) != 1 || selected[0].Result != "通过" {
				t.Fatalf("latest health window selected older evidence: %#v", selected)
			}
		})
	}
}
