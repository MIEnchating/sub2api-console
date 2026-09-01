package pricing

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/adminclient"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/upstreamsync"
)

type fakeRevenueUsageReader struct {
	sub2api   map[string]upstreamsync.KeyUsageObservation
	newapi    *upstreamsync.NewAPIUsageObservations
	newAPIErr error
}

type blockingRevenueUsageReader struct {
	started chan string
	release chan struct{}
}

func (reader blockingRevenueUsageReader) ReadSub2APIKeyUsage(ctx context.Context, _ configstore.AuthRecord, keyID, _, _ string) (upstreamsync.KeyUsageObservation, error) {
	select {
	case reader.started <- keyID:
	case <-ctx.Done():
		return upstreamsync.KeyUsageObservation{}, ctx.Err()
	}
	select {
	case <-reader.release:
		return upstreamsync.KeyUsageObservation{Cost: 1}, nil
	case <-ctx.Done():
		return upstreamsync.KeyUsageObservation{}, ctx.Err()
	}
}

func (blockingRevenueUsageReader) ReadNewAPIKeyUsage(context.Context, configstore.AuthRecord, time.Time, time.Time) (upstreamsync.NewAPIUsageObservations, error) {
	return upstreamsync.NewAPIUsageObservations{}, errors.New("unexpected NewAPI read")
}

func (reader fakeRevenueUsageReader) ReadSub2APIKeyUsage(_ context.Context, _ configstore.AuthRecord, keyID, _, _ string) (upstreamsync.KeyUsageObservation, error) {
	value, found := reader.sub2api[keyID]
	if !found {
		return upstreamsync.KeyUsageObservation{}, errors.New("missing key")
	}
	return value, nil
}

func (reader fakeRevenueUsageReader) ReadNewAPIKeyUsage(context.Context, configstore.AuthRecord, time.Time, time.Time) (upstreamsync.NewAPIUsageObservations, error) {
	if reader.newAPIErr != nil {
		return upstreamsync.NewAPIUsageObservations{}, reader.newAPIErr
	}
	if reader.newapi == nil {
		return upstreamsync.NewAPIUsageObservations{}, errors.New("unexpected NewAPI read")
	}
	return *reader.newapi, nil
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

func TestBuildRevenueRowUsesDecimalDivisionAndKeepsNumericJSONContract(t *testing.T) {
	account := business.RevenueAccount{ID: "41", Bindings: []business.RevenueBinding{{
		AuthHost: "api.example", UpstreamKeyID: "91", RechargeRate: "0.1",
	}}}
	first, second := 0.1, 0.2
	row := buildRevenueRow(account, localUsageResult{totals: adminclient.UsageTotals{
		AccountCost: 3, ActualCost: 3,
	}}, map[string]upstreamsync.KeyUsageObservation{
		revenueKey("api.example", "91"): {Cost: first + second},
	}, map[string]struct{}{})
	if row.Category != "正常" || row.UpstreamCost == nil || *row.UpstreamCost != 3 || row.Difference == nil || *row.Difference != 0 {
		t.Fatalf("row=%#v", row)
	}
	encoded, err := json.Marshal(row)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatal(err)
	}
	if _, ok := payload["upstream_cost"].(float64); !ok {
		t.Fatalf("upstream_cost stopped being a JSON number: %s", encoded)
	}
}

func TestBuildRevenueRowTreatsDecimalTwoDollarBoundaryAsNormal(t *testing.T) {
	account := business.RevenueAccount{ID: "41", Bindings: []business.RevenueBinding{{
		AuthHost: "api.example", UpstreamKeyID: "91", RechargeRate: "1",
	}}}
	base, increment := 2.1, 0.2
	row := buildRevenueRow(account, localUsageResult{totals: adminclient.UsageTotals{
		AccountCost: base + increment, ActualCost: base + increment,
	}}, map[string]upstreamsync.KeyUsageObservation{
		revenueKey("api.example", "91"): {Cost: 0.3},
	}, map[string]struct{}{})
	if row.Category != "正常" || row.Difference == nil || *row.Difference != 2 {
		t.Fatalf("two-dollar boundary was not exact: %#v", row)
	}
}

func TestBuildRevenueRowTreatsAmountAboveTwoDollarBoundaryAsAbnormal(t *testing.T) {
	account := business.RevenueAccount{ID: "41", Bindings: []business.RevenueBinding{{
		AuthHost: "api.example", UpstreamKeyID: "91", RechargeRate: "1",
	}}}
	row := buildRevenueRow(account, localUsageResult{totals: adminclient.UsageTotals{
		AccountCost: 2.000001, ActualCost: 2.000001,
	}}, map[string]upstreamsync.KeyUsageObservation{
		revenueKey("api.example", "91"): {Cost: 0},
	}, map[string]struct{}{})
	if row.Category != "计费异常" || row.Difference == nil || *row.Difference != 2.000001 {
		t.Fatalf("amount above two-dollar boundary was not abnormal: %#v", row)
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

func TestSummarizeRevenueDoesNotExposeBinaryFloatAccumulation(t *testing.T) {
	value := func(v float64) *float64 { return &v }
	rows := []RevenueRow{
		{LocalGroup: "codex", AttributionLevel: "key", AccountCost: value(0.1), ActualCost: value(0.1), UpstreamRawCost: value(0.1), UpstreamCost: value(0.1), Difference: value(0.1), Revenue: value(0.1)},
		{LocalGroup: "codex", AttributionLevel: "key", AccountCost: value(0.2), ActualCost: value(0.2), UpstreamRawCost: value(0.2), UpstreamCost: value(0.2), Difference: value(0.2), Revenue: value(0.2)},
	}
	summaries := summarizeRevenue(rows)
	if len(summaries) != 2 || summaries[0].AccountCost != 0.3 || summaries[1].AccountCost != 0.3 {
		t.Fatalf("summaries expose binary accumulation: %#v", summaries)
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

func TestCalculateRevenueReturnsCanceledContextInsteadOfUnavailableReport(t *testing.T) {
	management := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("canceled revenue calculation unexpectedly reached the management API")
	}))
	defer management.Close()
	token := "token"
	repository := &fakeRepository{revenue: business.RevenueCatalog{Accounts: []business.RevenueAccount{{
		ID: "41", Name: "alpha", Bindings: []business.RevenueBinding{{
			AuthHost: "api.example", UpstreamHost: "api.example", UpstreamType: "sub2api",
			UpstreamKeyID: "91", RechargeRate: "1",
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
	service.usage = fakeRevenueUsageReader{sub2api: map[string]upstreamsync.KeyUsageObservation{"91": {Cost: 1}}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := service.CalculateRevenue(ctx, RevenueRequest{Date: "2020-01-01"}, "tester")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("CalculateRevenue error = %v, want context.Canceled", err)
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

func TestFetchRevenueHostKeepsNewAPIFlowFailureUnavailableWithoutFabricatedCost(t *testing.T) {
	token := "token"
	targets := &fakeTargets{auth: map[string]configstore.AuthRecord{"api.example": {
		Host: "api.example", BaseURL: "https://api.example", UpstreamType: "newapi",
		AuthMode: "newapi_user_token", AccessToken: &token, Headers: map[string]string{}, Cookies: map[string]string{},
	}}}
	service := New(&fakeRepository{}, targets, nil)
	service.usage = fakeRevenueUsageReader{newAPIErr: errors.New("NewAPI 版本不支持稳定 Token ID 消费聚合，无法精确核对")}
	result := service.fetchRevenueHost(context.Background(), targets, "api.example", []business.RevenueBinding{
		{AuthHost: "api.example", UpstreamKeyID: "91"},
	}, "2020-01-01", time.Unix(1, 0), time.Unix(2, 0), "tester")
	if !strings.Contains(result.issue, "稳定 Token ID") || len(result.values) != 0 {
		t.Fatalf("result=%#v", result)
	}
}

func TestFetchRevenueHostReadsSub2APIKeysWithBoundedConcurrency(t *testing.T) {
	token := "token"
	targets := &fakeTargets{auth: map[string]configstore.AuthRecord{"api.example": {
		Host: "api.example", BaseURL: "https://api.example", UpstreamType: "sub2api",
		AuthMode: "sub2api_user_token", AccessToken: &token, Headers: map[string]string{}, Cookies: map[string]string{},
	}}}
	reader := blockingRevenueUsageReader{started: make(chan string, 3), release: make(chan struct{})}
	service := New(&fakeRepository{}, targets, nil)
	service.usage = reader
	done := make(chan upstreamUsageResult, 1)
	go func() {
		done <- service.fetchRevenueHost(context.Background(), targets, "api.example", []business.RevenueBinding{
			{AuthHost: "api.example", UpstreamKeyID: "91"},
			{AuthHost: "api.example", UpstreamKeyID: "92"},
			{AuthHost: "api.example", UpstreamKeyID: "93"},
		}, "2020-01-01", time.Unix(1, 0), time.Unix(2, 0), "tester")
	}()
	for index := 0; index < 2; index++ {
		select {
		case <-reader.started:
		case <-time.After(time.Second):
			t.Fatal("a second Key read did not start before the first was released")
		}
	}
	close(reader.release)
	select {
	case result := <-done:
		if result.issue != "" || len(result.values) != 3 {
			t.Fatalf("result=%#v", result)
		}
	case <-time.After(time.Second):
		t.Fatal("concurrent Key reads did not complete")
	}
}
