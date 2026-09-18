package business_test

import (
	"testing"
	"time"
)

func TestRecoveryTrafficReadsHoldIntervalAndOneBoundaryWithStableAccountDeduplication(t *testing.T) {
	store, db := concurrencyStore(t)
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	for _, row := range []struct {
		account, request, group, source string
		age                             time.Duration
		failed                          int
	}{
		{"41", "old", "codex", "traffic", 3 * time.Minute, 1},
		{"41", "boundary", "codex", "traffic", 2 * time.Minute, 0},
		{"41", "edge", "codex", "traffic", time.Minute, 0},
		{"41", "recent", "old-group", "traffic", time.Second, 1},
		{"41", "recent", "new-group", "traffic", time.Second, 0},
		{"42", "other-account", "codex", "traffic", time.Second, 0},
		{"41", "other-source", "codex", "import", time.Second, 0},
		{"41", "future", "codex", "traffic", -time.Minute, 0},
	} {
		_, err := db.Exec(`INSERT INTO usage_records(account_id,request_id,group_name,source,is_error,observed_at,payload_json)
			VALUES(?,?,?,?,?,?,'{}')`, row.account, row.request, row.group, row.source, row.failed, now.Add(-row.age).Format("2006-01-02T15:04:05.000000000Z"))
		if err != nil {
			t.Fatal(err)
		}
	}
	rows, err := store.RoutingRecoveryTraffic(t.Context(), "41", now.Add(-time.Minute), now)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 || rows[0].Payload["request_id"] != "recent" || rows[0].GroupName != "new-group" || rows[0].Result != "通过" || rows[1].Payload["request_id"] != "edge" || rows[2].Payload["request_id"] != "boundary" {
		t.Fatalf("recovery traffic lost temporal boundaries or stable ownership: %+v", rows)
	}
}

func TestRecoveryTrafficRejectsDamagedEvidenceInsteadOfProvingHealthyHold(t *testing.T) {
	store, db := concurrencyStore(t)
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	_, err := db.Exec(`INSERT INTO usage_records(account_id,request_id,group_name,source,is_error,observed_at,payload_json)
		VALUES('41','broken','codex','traffic',0,?,'null')`, now.Format("2006-01-02T15:04:05.000000000Z"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.RoutingRecoveryTraffic(t.Context(), "41", now.Add(-time.Minute), now); err == nil {
		t.Fatal("damaged hold evidence must fail the read")
	}
}
