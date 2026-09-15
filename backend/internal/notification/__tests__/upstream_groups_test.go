package notification_test

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/notification"
)

func upstreamIncident(key, host, event, cause, status string) business.AlertIncident {
	return business.AlertIncident{
		IncidentKey: key, ObjectKind: "host", ObjectID: host, EventType: event,
		CauseCode: cause, Status: status, LastSeenAt: "2026-09-14T10:45:49Z",
	}
}

func TestSameUpstreamAlertsBelowMergeThresholdProduceOneNotification(t *testing.T) {
	incidents := []business.AlertIncident{
		upstreamIncident("balance", "first.example", "upstream.balance", "BALANCE:20", "firing"),
		upstreamIncident("other", "second.example", "upstream.auth", "AUTH", "firing"),
		upstreamIncident("rate", "first.example", "upstream.rate_sync", "RATE_SYNC", "firing"),
	}
	batches := notification.NotificationBatches(incidents, 10)
	if len(batches) != 2 || len(batches[0].Incidents) != 2 || len(batches[1].Incidents) != 1 {
		t.Fatalf("expected one notification per upstream, got %+v", batches)
	}
	message := batches[0].Message
	for _, want := range []string{"上游余额不足", "余额达到或低于 20", "上游倍率同步失败", "2026-09-14 18:45:49"} {
		if !strings.Contains(message, want) {
			t.Errorf("merged upstream notification missing %q: %s", want, message)
		}
	}
	if strings.Count(message, "first.example") != 1 || strings.Contains(message, "second.example") {
		t.Fatalf("expected the upstream once with no unrelated host: %s", message)
	}
}

func TestSameUpstreamRecoveryAndFiringKeepTheirOwnStatus(t *testing.T) {
	incidents := []business.AlertIncident{
		upstreamIncident("balance", "first.example", "upstream.balance", "BALANCE:20", "recovered"),
		upstreamIncident("auth", "first.example", "upstream.auth", "AUTH", "firing"),
	}
	batches := notification.NotificationBatches(incidents, 10)
	if len(batches) != 1 {
		t.Fatalf("expected one mixed notification, got %+v", batches)
	}
	message := batches[0].Message
	for _, want := range []string{
		"Sub2API · 告警与恢复汇总", "| 上游余额已恢复 | 余额已高于告警阈值 20 | 已恢复 |",
		"| 上游鉴权失败 | 上游鉴权失败 | 告警中 |",
	} {
		if !strings.Contains(message, want) {
			t.Errorf("mixed notification missing %q: %s", want, message)
		}
	}
}

func TestSameUpstreamRecoveriesProduceOneRecoveryNotification(t *testing.T) {
	batches := notification.NotificationBatches([]business.AlertIncident{
		upstreamIncident("balance", "first.example", "upstream.balance", "BALANCE:20", "recovered"),
		upstreamIncident("auth", "first.example", "upstream.auth", "AUTH", "recovered"),
	}, 10)
	if len(batches) != 1 || !strings.Contains(batches[0].Message, "Sub2API · 恢复通知") || strings.Contains(batches[0].Message, "告警中") {
		t.Fatalf("expected all recoveries in one recovery notification, got %+v", batches)
	}
}

func TestUpstreamGroupingRequiresTheSameNonemptyHostID(t *testing.T) {
	for _, scenario := range []struct {
		name string
		host string
		kind string
	}{
		{name: "different hosts with the same display name", host: "second.example", kind: "host"},
		{name: "missing host IDs", kind: "host"},
		{name: "account with the same object ID", host: "first.example", kind: "account"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			first := upstreamIncident("first", "first.example", "upstream.balance", "BALANCE:20", "firing")
			second := upstreamIncident("second", scenario.host, "upstream.auth", "AUTH", "firing")
			second.ObjectKind = scenario.kind
			name := "same display name"
			first.ObjectName, second.ObjectName = &name, &name
			if scenario.host == "" {
				first.ObjectID = ""
			}
			batches := notification.NotificationBatches([]business.AlertIncident{first, second}, 10)
			if len(batches) != 2 {
				t.Fatalf("unrelated or unidentified objects were merged: %+v", batches)
			}
		})
	}
}

func TestLargeSummaryKeepsEveryCauseUnderItsUpstream(t *testing.T) {
	incidents := []business.AlertIncident{
		upstreamIncident("balance", "first.example", "upstream.balance", "BALANCE:20", "firing"),
		upstreamIncident("auth", "first.example", "upstream.auth", "AUTH", "firing"),
		upstreamIncident("other", "second.example", "upstream.balance", "BALANCE:5", "firing"),
	}
	batches := notification.NotificationBatches(incidents, 2)
	if len(batches) != 1 || len(batches[0].Incidents) != 3 {
		t.Fatalf("expected one complete summary, got %+v", batches)
	}
	for _, want := range []string{"余额达到或低于 20", "上游鉴权失败", "余额达到或低于 5"} {
		if !strings.Contains(batches[0].Message, want) {
			t.Errorf("summary missing %q: %s", want, batches[0].Message)
		}
	}
	if strings.Count(batches[0].Message, "first.example") != 1 {
		t.Fatalf("summary repeated the same upstream: %s", batches[0].Message)
	}
}

func TestOversizedUpstreamGroupSplitsWithoutLosingIncidentDetails(t *testing.T) {
	for _, threshold := range []int{2, 100} {
		t.Run(fmt.Sprintf("threshold-%d", threshold), func(t *testing.T) {
			incidents := make([]business.AlertIncident, 40)
			for index := range incidents {
				key := fmt.Sprintf("detail-%02d", index)
				incidents[index] = upstreamIncident(key, "first.example", "upstream.configuration", "CONFIG_AUTH_STATUS_UNKNOWN:"+strings.Repeat("原因", 80)+key, "firing")
			}
			batches := notification.NotificationBatches(incidents, threshold)
			if len(batches) < 2 || len(batches) >= len(incidents) {
				t.Fatalf("expected length-bounded merged messages, got %d", len(batches))
			}
			seen := make(map[string]int)
			for _, batch := range batches {
				if utf8.RuneCountInString(batch.Message) > 4000 {
					t.Fatal("notification exceeded the message limit")
				}
				for _, incident := range batch.Incidents {
					seen[incident.IncidentKey]++
					if !strings.Contains(batch.Message, incident.IncidentKey) {
						t.Errorf("notification omitted detail for %s", incident.IncidentKey)
					}
				}
			}
			for _, incident := range incidents {
				if seen[incident.IncidentKey] != 1 {
					t.Errorf("incident %s occurs in %d batches", incident.IncidentKey, seen[incident.IncidentKey])
				}
			}
		})
	}
}
