package accountops_test

import (
	"context"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountops"
)

func TestAccountSettingsRejectNonDecimalAndUnboundedLoadFactorsBeforeQueueing(t *testing.T) {
	f := newSettingsFixture(t)
	for _, value := range []string{"3/2", "0x10", "1_000", "1e1001", strings.Repeat("9", 129)} {
		_, err := f.service.EnqueueSettings(context.Background(), "41", accountops.SettingsInput{
			Priority: 20, LoadFactor: value, Concurrency: 3,
		}, "test")
		if err == nil || f.runner.run != nil {
			t.Fatalf("invalid load factor %q reached task execution: err=%v", value, err)
		}
	}
}
