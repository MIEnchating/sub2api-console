package accountquality_test

import (
	"github.com/MIEnchating/sub2api-console/backend/internal/accountquality"
	"testing"
	"time"
)

func TestWindowsIncludeBoundariesAndExcludeExpiredAndFutureSamples(t *testing.T) {
	now := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	stats := accountquality.New(now)
	stats.Add(now, now.Add(-24*time.Hour), "passed")
	stats.Add(now, now.Add(-30*24*time.Hour), "failed")
	stats.Add(now, now.Add(-30*24*time.Hour-time.Nanosecond), "failed")
	stats.Add(now, now.Add(time.Nanosecond), "passed")
	stats.Calculate(false)
	if stats.Short.Samples != 1 || *stats.Short.Score != 100 || stats.Long.Samples != 2 || *stats.Long.Score != 50 {
		t.Fatalf("windows: %+v", stats)
	}
}

func TestConfidencePenalizesSmallSamplesAndExcludesInconclusiveResults(t *testing.T) {
	now := time.Now()
	stats := accountquality.New(now)
	stats.Add(now, now, "inconclusive")
	stats.Calculate(true)
	if stats.Short.Score != nil {
		t.Fatal("inconclusive result became a score")
	}
	stats.Add(now, now, "passed")
	stats.Calculate(true)
	if *stats.Short.Score != 20.7 || stats.Short.Inconclusive != 1 {
		t.Fatalf("single match: %+v", stats.Short)
	}
	for i := 0; i < 99; i++ {
		stats.Add(now, now, "passed")
	}
	stats.Calculate(true)
	if *stats.Short.Score != 96.3 {
		t.Fatalf("100 matches: %+v", stats.Short)
	}
}
