package workbenchprovider_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/workbenchprovider"
)

func TestMailCodesUseNearestValidTimestampFromSameText(t *testing.T) {
	payload := "2026-09-14T01:00:00Z OpenAI code 123456 " + strings.Repeat(". ", 150) + "2026-09-14T02:00:00Z OpenAI code 234567"
	candidates, err := workbenchprovider.ExtractMailCandidates([]byte(payload))
	if err != nil || len(candidates) != 2 {
		t.Fatalf("candidates=%#v, error=%v", candidates, err)
	}
	for _, candidate := range candidates {
		wantHour := 1
		if candidate.Code == "234567" {
			wantHour = 2
		}
		if !candidate.ReceivedAt.Equal(time.Date(2026, 9, 14, wantHour, 0, 0, 0, time.UTC)) {
			t.Fatalf("code %s attached to wrong timestamp: %s", candidate.Code, candidate.ReceivedAt)
		}
	}
}

func TestInvalidNearbyTimestampDoesNotHideValidTimestamp(t *testing.T) {
	candidates, err := workbenchprovider.ExtractMailCandidates([]byte("2026-09-14T01:00:00Z 2026-99-99T02:00:00Z OpenAI code 123456"))
	if err != nil || len(candidates) != 1 || !candidates[0].ReceivedAt.Equal(time.Date(2026, 9, 14, 1, 0, 0, 0, time.UTC)) {
		t.Fatalf("invalid timestamp replaced valid received time: %#v, %v", candidates, err)
	}
}

func BenchmarkTimestampedMailHistory(b *testing.B) {
	var payload strings.Builder
	for index := range 1000 {
		fmt.Fprintf(&payload, "2026-09-14T01:00:00Z OpenAI verification code %06d\n", 100000+index)
	}
	raw := []byte(payload.String())
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		candidates, err := workbenchprovider.ExtractMailCandidates(raw)
		if err != nil || len(candidates) != 1000 {
			b.Fatalf("candidates=%d, error=%v", len(candidates), err)
		}
	}
}
