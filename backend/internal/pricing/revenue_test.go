package pricing

import (
	"context"
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/adminclient"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/upstreamsync"
)

type fakeRevenueUsageReader struct {
	sub2api map[string]upstreamsync.KeyUsageObservation
}

func (reader fakeRevenueUsageReader) ReadSub2APIKeyUsage(_ context.Context, _ configstore.AuthRecord, keyID, _, _ string) (upstreamsync.KeyUsageObservation, error) {
	value, found := reader.sub2api[keyID]
	if !found {
		return upstreamsync.KeyUsageObservation{}, errors.New("missing key")
	}
	return value, nil
}

func (fakeRevenueUsageReader) ReadNewAPIKeyUsage(context.Context, configstore.AuthRecord, time.Time, time.Time) (upstreamsync.NewAPIUsageObservations, error) {
	return upstreamsync.NewAPIUsageObservations{}, errors.New("unexpected NewAPI read")
}

func TestBuildRevenueRowUsesActualCostForRevenueAndAccountCostForDifference(t *testing.T) {
	account := business.RevenueAccount{
		ID: "41", Name: "alpha", Groups: []string{"codex"},
		Bindings: []business.RevenueBinding{{
			AuthHost: "api.example", UpstreamKeyID: "91", UpstreamKeyName: "alpha-key", RechargeRate: "2",
		}},
	}
	row := buildRevenueRow(account, localUsageResult{totals: adminclient.UsageTotals{
		AccountCost: 8, ActualCost: 10,
	}}, map[string]upstreamsync.KeyUsageObservation{
		revenueKey("api.example", "91"): {Cost: 6},
	}, map[string]struct{}{})
	if row.Category != "计费异常" || row.UpstreamCost == nil || *row.UpstreamCost != 3 || row.Difference == nil || *row.Difference != 5 || row.Revenue == nil || *row.Revenue != 7 {
		t.Fatalf("row=%#v", row)
	}
}

func TestBuildRevenueRowKeepsSharedStableKeyUnavailable(t *testing.T) {
	account := business.RevenueAccount{ID: "41", Bindings: []business.RevenueBinding{{
		AuthHost: "api.example", UpstreamKeyID: "91", RechargeRate: "1",
	}}}
	row := buildRevenueRow(account, localUsageResult{totals: adminclient.UsageTotals{}},
		map[string]upstreamsync.KeyUsageObservation{revenueKey("api.example", "91"): {Cost: 1}},
		map[string]struct{}{revenueKey("api.example", "91"): {}},
	)
	if row.Category != "无法核对" || row.Revenue != nil || row.AttributionLevel != "unavailable" {
		t.Fatalf("row=%#v", row)
	}
}

func TestBuildRevenueRowPreservesLocalReadFailure(t *testing.T) {
	account := business.RevenueAccount{ID: "41"}
	row := buildRevenueRow(account, localUsageResult{err: errors.New("stats down")}, nil, nil)
	if row.AccountCost != nil || row.ActualCost != nil || row.Category != "无法核对" {
		t.Fatalf("row=%#v", row)
	}
}

func TestRevenueWindowDefaultsToPreviousCompletedShanghaiDay(t *testing.T) {
	now := time.Date(2026, 8, 30, 3, 0, 0, 0, time.UTC)
	date, start, end, err := revenueWindow("", now)
	if err != nil {
		t.Fatal(err)
	}
	if date != "2026-08-29" || end.Sub(start) != 24*time.Hour || start.UTC().Format(time.RFC3339) != "2026-08-28T16:00:00Z" {
		t.Fatalf("date=%s start=%s end=%s", date, start, end)
	}
	if _, _, _, err := revenueWindow("2026-08-30", now); err == nil {
		t.Fatal("active day was accepted")
	}
}

func TestSummarizeRevenueIncludesOnlyExactRows(t *testing.T) {
	value := func(v float64) *float64 { return &v }
	rows := []RevenueRow{
		{LocalGroup: "codex", AttributionLevel: "key", AccountCost: value(4), ActualCost: value(5), UpstreamRawCost: value(3), UpstreamCost: value(2), Difference: value(2), Revenue: value(3)},
		{LocalGroup: "codex", AttributionLevel: "unavailable", AccountCost: value(99)},
	}
	summaries := summarizeRevenue(rows)
	if len(summaries) != 2 || summaries[0].Accounts != 1 || math.Abs(summaries[1].Revenue-3) > 1e-9 {
		t.Fatalf("summaries=%#v", summaries)
	}
}

func TestCalculateRevenueReadsLocalAUAndStableUpstreamKey(t *testing.T) {
	management := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/v1/admin/usage/stats" || request.URL.Query().Get("account_id") != "41" {
			t.Fatalf("request=%s?%s", request.URL.Path, request.URL.RawQuery)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"code":0,"data":{"total_account_cost":8,"total_actual_cost":10}}`)
	}))
	defer management.Close()
	token := "token"
	repository := &fakeRepository{revenue: business.RevenueCatalog{Accounts: []business.RevenueAccount{{
		ID: "41", Name: "alpha", Groups: []string{"codex"}, Bindings: []business.RevenueBinding{{
			AuthHost: "api.example", UpstreamHost: "api.example", UpstreamType: "sub2api",
			UpstreamKeyID: "91", UpstreamKeyName: "alpha-key", RechargeRate: "2",
		}},
	}}}}
	targets := &fakeTargets{
		settings: configstore.TargetSettings{BaseURL: management.URL, AdminKey: "secret", TimeoutSeconds: 5},
		auth: map[string]configstore.AuthRecord{"api.example": {
			Host: "api.example", BaseURL: "https://api.example", UpstreamType: "sub2api",
			AuthMode: "sub2api_user_token", AccessToken: &token, Headers: map[string]string{}, Cookies: map[string]string{},
		}},
	}
	service := New(repository, targets, nil)
	service.usage = fakeRevenueUsageReader{sub2api: map[string]upstreamsync.KeyUsageObservation{"91": {Cost: 6}}}
	report, err := service.CalculateRevenue(context.Background(), RevenueRequest{Date: "2020-01-01"}, "tester")
	if err != nil {
		t.Fatal(err)
	}
	if report.Comparable != 1 || report.Abnormal != 1 || len(report.Summaries) != 2 || report.Rows[0].Revenue == nil || *report.Rows[0].Revenue != 7 {
		t.Fatalf("report=%#v", report)
	}
}

func TestFetchRevenueHostKeepsSuccessfulSub2APIKeysWhenSiblingReadFails(t *testing.T) {
	token := "token"
	targets := &fakeTargets{auth: map[string]configstore.AuthRecord{"api.example": {
		Host: "api.example", BaseURL: "https://api.example", UpstreamType: "sub2api",
		AuthMode: "sub2api_user_token", AccessToken: &token, Headers: map[string]string{}, Cookies: map[string]string{},
	}}}
	service := New(&fakeRepository{}, targets, nil)
	service.usage = fakeRevenueUsageReader{sub2api: map[string]upstreamsync.KeyUsageObservation{"91": {Cost: 1}}}
	result := service.fetchRevenueHost(context.Background(), targets, "api.example", []business.RevenueBinding{
		{AuthHost: "api.example", UpstreamKeyID: "91"},
		{AuthHost: "api.example", UpstreamKeyID: "92"},
	}, "2020-01-01", time.Unix(1, 0), time.Unix(2, 0), "tester")
	if result.issue == "" || result.values["91"].Cost != 1 {
		t.Fatalf("result=%#v", result)
	}
}
