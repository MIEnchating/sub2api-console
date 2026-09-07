package evidence

import (
	"context"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

type futureTrafficAdmin struct {
	observedAt time.Time
}

func (admin futureTrafficAdmin) RequestDetails(context.Context, string, int, int) ([]map[string]any, error) {
	return []map[string]any{{
		"account_id": "41", "request_id": "future-request", "kind": "success",
		"created_at": admin.observedAt.Format(time.RFC3339Nano),
	}}, nil
}

func TestCollectRejectsFutureTrafficAndContinuesActiveProbe(t *testing.T) {
	now := time.Now().UTC()
	repository := &lateTrafficRepository{targets: []business.EvidenceTarget{{AccountID: "41", GroupName: "codex"}}}
	result, err := New(repository, &lateTrafficProbeRunner{}).Collect(
		context.Background(), testTrafficProbePolicy(), futureTrafficAdmin{observedAt: now.Add(time.Hour)},
		Options{FetchTraffic: true, ProbesAllowed: true, Now: now},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(repository.samples) != 0 || result.ProbesPersisted != 1 || result.MalformedRows == 0 {
		t.Fatalf("future upstream timestamp suppressed probe or entered evidence: samples=%#v result=%#v", repository.samples, result)
	}
}

func TestFutureStoredEvidenceDoesNotPostponeProbe(t *testing.T) {
	now := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	future := now.Add(time.Hour)
	policy, err := parsePolicy(testTrafficProbePolicy())
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range []business.EvidenceTarget{
		{AccountID: "41", GroupName: "codex", TrafficAt: &future},
		{AccountID: "41", GroupName: "codex", ProbeAt: &future},
	} {
		if !membershipProbeDue(target, policy, now, false, false) {
			t.Errorf("future evidence postponed required probe: %#v", target)
		}
	}
}

func TestFutureTrafficFetchDoesNotPostponeRefresh(t *testing.T) {
	now := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	future := now.Add(time.Hour)
	if !trafficFetchDue([]business.EvidenceTarget{{AccountID: "41", TrafficFetchAt: &future}}, now, time.Minute) {
		t.Fatal("future fetch timestamp postponed traffic refresh")
	}
}

func TestFutureTrafficDoesNotCountAsFreshEvidence(t *testing.T) {
	now := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	future := now.Add(time.Hour)
	if hasFreshTraffic([]business.EvidenceTarget{{TrafficAt: &future}}, now, time.Minute) {
		t.Error("future stored traffic counted as fresh evidence")
	}
	if trafficSamplesFresh([]business.TrafficSample{{ObservedAt: future.Format(time.RFC3339Nano)}}, now, time.Minute) {
		t.Error("future fetched traffic counted as fresh evidence")
	}
}

func TestTrafficWithinOneMinuteClockSkewStillCountsAsFresh(t *testing.T) {
	now := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	withinSkew := now.Add(time.Minute)
	if !trafficSamplesFresh([]business.TrafficSample{{ObservedAt: withinSkew.Format(time.RFC3339Nano)}}, now, time.Minute) {
		t.Fatal("accepted upstream clock skew incorrectly invalidated fresh traffic")
	}
}
