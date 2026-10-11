package business_test

import (
	"context"
	"errors"
	"reflect"
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

func TestRecoveryTrafficComparesNanosecondsAcrossUTCYearBoundaries(t *testing.T) {
	for _, scenario := range []struct {
		name, since, boundary, lower, upper, future string
	}{
		{"minimum-year", "0000-01-01T00:00:00Z", "0000-01-01T00:59:59.999999999+01:00", "0000-01-01T01:00:00+01:00", "0000-01-01T01:01:00+01:00", "0000-01-01T00:01:00.000000001Z"},
		{"maximum-year", "9999-12-31T23:59:00Z", "9999-12-31T22:58:59.999999999-01:00", "9999-12-31T22:59:00-01:00", "9999-12-31T23:00:00-01:00", "9999-12-31T23:00:00.000000001-01:00"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			store, db := concurrencyStore(t)
			for _, row := range []struct{ request, stamp string }{
				{"boundary", scenario.boundary}, {"lower", scenario.lower},
				{"upper", scenario.upper}, {"future", scenario.future},
			} {
				if _, err := db.Exec(`INSERT INTO usage_records(account_id,request_id,group_name,source,is_error,observed_at,payload_json)
					VALUES('41',?,'codex','traffic',0,?,'{}')`, row.request, row.stamp); err != nil {
					t.Fatal(err)
				}
			}
			since, err := time.Parse(time.RFC3339Nano, scenario.since)
			if err != nil {
				t.Fatal(err)
			}
			rows, err := store.RoutingRecoveryTraffic(t.Context(), "41", since, since.Add(time.Minute))
			if err != nil {
				t.Fatal(err)
			}
			var requests []string
			for _, row := range rows {
				requests = append(requests, row.Payload["request_id"].(string))
			}
			if want := []string{"upper", "lower", "boundary"}; !reflect.DeepEqual(requests, want) {
				t.Fatalf("UTC year boundary selection=%v, want %v", requests, want)
			}
		})
	}
}

func TestRecoveryTrafficSkipsUnparseableDatesInsteadOfUsingThemAsRecoveryBoundary(t *testing.T) {
	store, db := concurrencyStore(t)
	if _, err := db.Exec(`INSERT INTO usage_records(account_id,request_id,group_name,source,is_error,observed_at,payload_json) VALUES
		('41','invalid','codex','traffic',0,'invalid','null'),
		('41','missing','codex','traffic',0,NULL,'null'),
		('41','empty','codex','traffic',0,'','null'),
		('41','bad-date','codex','traffic',0,'2026-09-31T00:00:00.000000000Z','null'),
		('41','boundary','codex','traffic',1,'2026-09-30T23:59:59Z','{}')`); err != nil {
		t.Fatal(err)
	}
	since := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	rows, err := store.RoutingRecoveryTraffic(t.Context(), "41", since, since.Add(time.Minute))
	if err != nil || len(rows) != 1 || rows[0].Payload["request_id"] != "boundary" || rows[0].Result != "失败" {
		t.Fatalf("invalid dates replaced the actual preceding failure: rows=%+v error=%v", rows, err)
	}
}

func TestRecoveryTrafficIgnoresDamagedSupersededPayloadAndKeepsLatestCorrection(t *testing.T) {
	store, db := concurrencyStore(t)
	if _, err := db.Exec(`INSERT INTO usage_records(account_id,request_id,group_name,source,is_error,observed_at,payload_json) VALUES
		('41','corrected','older-group','traffic',1,'2026-10-01T12:00:00Z','null'),
		('41','corrected','latest-group','traffic',0,'2026-10-01T12:00:00.000000000Z','{}')`); err != nil {
		t.Fatal(err)
	}
	since := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	rows, err := store.RoutingRecoveryTraffic(t.Context(), "41", since, since.Add(time.Minute))
	if err != nil || len(rows) != 1 || rows[0].GroupName != "latest-group" || rows[0].Result != "通过" {
		t.Fatalf("superseded payload blocked the latest correction: rows=%+v error=%v", rows, err)
	}
}

func TestRecoveryTrafficReturnsEmptyForAnAccountWithoutEvidence(t *testing.T) {
	store, _ := concurrencyStore(t)
	since := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	rows, err := store.RoutingRecoveryTraffic(t.Context(), "41", since, since.Add(time.Minute))
	if err != nil || len(rows) != 0 {
		t.Fatalf("empty history: rows=%+v error=%v", rows, err)
	}
}

func TestRecoveryTrafficHonorsCancellation(t *testing.T) {
	store, _ := concurrencyStore(t)
	since := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := store.RoutingRecoveryTraffic(ctx, "41", since, since.Add(time.Minute)); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled recovery read error=%v", err)
	}
}

func TestRecoveryTrafficUsesActualInstantsForInclusiveIntervalAndNearestBoundary(t *testing.T) {
	store, db := concurrencyStore(t)
	for _, row := range []struct {
		request, stamp string
		failed         int
	}{
		{"older-boundary", "2026-10-01T19:58:00+08:00", 0},
		{"nearest-boundary", "2026-10-01T03:59:59.999999999-08:00", 1},
		{"lower-edge", "2026-10-01T20:00:00+08:00", 0},
		{"middle", "2026-10-01T04:00:30.12-08:00", 0},
		{"upper-edge", "2026-10-01T12:01:00Z", 0},
		{"future", "2026-10-01T04:01:00.000000001-08:00", 0},
		{"invalid", "2026-10-00T12:00:00Z", 0},
	} {
		if _, err := db.Exec(`INSERT INTO usage_records(account_id,request_id,group_name,source,is_error,observed_at,payload_json)
			VALUES('41',?,'codex','TRAFFIC',?,?,'{}')`, row.request, row.failed, row.stamp); err != nil {
			t.Fatal(err)
		}
	}
	since := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	rows, err := store.RoutingRecoveryTraffic(t.Context(), "41", since, since.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	var requests []string
	for _, row := range rows {
		requests = append(requests, row.Payload["request_id"].(string))
	}
	want := []string{"upper-edge", "middle", "lower-edge", "nearest-boundary"}
	if !reflect.DeepEqual(requests, want) {
		t.Fatalf("actual interval and boundary = %v, want %v", requests, want)
	}
	if rows[len(rows)-1].Result != "失败" {
		t.Fatalf("nearest preceding failure was lost: %+v", rows[len(rows)-1])
	}
}

func TestRecoveryTrafficDeduplicatesByLatestInstantThenRowIDAcrossTimestampFormats(t *testing.T) {
	store, db := concurrencyStore(t)
	for _, row := range []struct {
		request, group, stamp string
	}{
		{"corrected", "older", "2026-10-01T20:00:00+08:00"},
		{"corrected", "newer", "2026-10-01T04:00:30-08:00"},
		{"tied", "first", "2026-10-01T20:00:45+08:00"},
		{"tied", "last", "2026-10-01T12:00:45.000000000Z"},
	} {
		if _, err := db.Exec(`INSERT INTO usage_records(account_id,request_id,group_name,source,is_error,observed_at,payload_json)
			VALUES('41',?,?,'traffic',0,?,'{}')`, row.request, row.group, row.stamp); err != nil {
			t.Fatal(err)
		}
	}
	since := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	rows, err := store.RoutingRecoveryTraffic(t.Context(), "41", since, since.Add(24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].GroupName != "last" || rows[1].GroupName != "newer" {
		t.Fatalf("latest corrections must win across formats: %+v", rows)
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
