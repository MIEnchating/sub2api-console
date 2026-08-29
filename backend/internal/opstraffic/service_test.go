package opstraffic

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

type staticTarget struct {
	baseURL string
}

func (target staticTarget) TargetSettings(context.Context) (configstore.TargetSettings, error) {
	return configstore.TargetSettings{BaseURL: target.baseURL, AdminKey: "admin-key", TimeoutSeconds: 5}, nil
}

type staticAccounts struct{}

func (staticAccounts) Accounts(context.Context) ([]business.AccountStatus, error) {
	return []business.AccountStatus{{ID: "42", Name: "账号 A"}}, nil
}

func (staticAccounts) Account(context.Context, string) (*business.AccountDetail, error) {
	groupID := "7"
	return &business.AccountDetail{
		AccountStatus: business.AccountStatus{ID: "42", Name: "账号 A"},
		GroupIDs:      map[string]*string{"默认分组": &groupID},
	}, nil
}

func TestRequestTraceReadsSuccessAndErrorsOnlyFromOpsRequests(t *testing.T) {
	paths := []string{}
	kinds := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		paths = append(paths, request.URL.Path)
		kinds = append(kinds, request.URL.Query().Get("kind"))
		writer.Header().Set("Content-Type", "application/json")
		if request.URL.Query().Get("request_id") == "req-1" {
			_, _ = io.WriteString(writer, `{"data":{"items":[{"request_id":"req-1","kind":"success","account_id":42,"group_id":7,"duration_ms":123,"created_at":"2026-08-27T10:00:00Z"}],"total":1}}`)
			return
		}
		_, _ = io.WriteString(writer, `{"data":{"items":[{"request_id":"req-error","kind":"error","account_id":42,"group_id":7,"duration_ms":456,"created_at":"2026-08-27T10:01:00Z","message":"rate limited"}],"total":1}}`)
	}))
	defer server.Close()

	service := New(staticTarget{baseURL: server.URL}, staticAccounts{})
	trace, err := service.RequestTrace(context.Background(), "req-1")
	if err != nil {
		t.Fatal(err)
	}
	if !trace.Matched || trace.AccountID == nil || *trace.AccountID != "42" || trace.AccountName == nil || *trace.AccountName != "账号 A" {
		t.Fatalf("trace identity=%#v", trace)
	}
	if len(trace.Records) != 1 || trace.Records[0].DurationMS == nil || *trace.Records[0].DurationMS != "123" || trace.Records[0].GroupName == nil || *trace.Records[0].GroupName != "默认分组" {
		t.Fatalf("records=%#v", trace.Records)
	}
	if len(trace.RecentErrors) != 1 || trace.RecentErrors[0].IsError == nil || !*trace.RecentErrors[0].IsError || trace.RecentErrors[0].ErrorReason == nil || *trace.RecentErrors[0].ErrorReason != "rate limited" {
		t.Fatalf("recent errors=%#v", trace.RecentErrors)
	}
	if len(paths) != 2 || paths[0] != "/api/v1/admin/ops/requests" || paths[1] != "/api/v1/admin/ops/requests" || kinds[0] != "all" || kinds[1] != "error" {
		t.Fatalf("paths=%#v kinds=%#v", paths, kinds)
	}
}

func TestRequestTraceFallsBackToIndexedSystemLogs(t *testing.T) {
	paths := []string{}
	var systemLogWindow time.Duration
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		paths = append(paths, request.URL.Path)
		writer.Header().Set("Content-Type", "application/json")
		if request.URL.Path == "/api/v1/admin/ops/system-logs" {
			start, startErr := time.Parse(time.RFC3339Nano, request.URL.Query().Get("start_time"))
			end, endErr := time.Parse(time.RFC3339Nano, request.URL.Query().Get("end_time"))
			if startErr != nil || endErr != nil {
				t.Fatalf("invalid system-log time range: start=%v end=%v", startErr, endErr)
			}
			systemLogWindow = end.Sub(start)
			_, _ = io.WriteString(writer, `{"data":{"items":[{"id":9,"request_id":"req-log","level":"info","component":"http.access","message":"http request completed status=200","account_id":42,"created_at":"2026-08-27T10:00:00Z","extra":{"latency_ms":27150}}],"total":1}}`)
			return
		}
		_, _ = io.WriteString(writer, `{"data":{"items":[],"total":0}}`)
	}))
	defer server.Close()

	service := New(staticTarget{baseURL: server.URL}, staticAccounts{})
	trace, err := service.RequestTrace(context.Background(), "req-log")
	if err != nil {
		t.Fatal(err)
	}
	if !trace.Matched || trace.AccountID == nil || *trace.AccountID != "42" || trace.AccountName == nil || *trace.AccountName != "账号 A" {
		t.Fatalf("trace identity=%#v", trace)
	}
	if len(trace.Records) != 1 || trace.Records[0].Source != "system-log" || trace.Records[0].Summary == nil || *trace.Records[0].Summary != "http request completed status=200" || trace.Records[0].DurationMS == nil || *trace.Records[0].DurationMS != "27150" {
		t.Fatalf("records=%#v", trace.Records)
	}
	if len(paths) != 3 || paths[0] != "/api/v1/admin/ops/requests" || paths[1] != "/api/v1/admin/ops/system-logs" || paths[2] != "/api/v1/admin/ops/requests" {
		t.Fatalf("paths=%#v", paths)
	}
	if systemLogWindow != time.Hour {
		t.Fatalf("system-log initial window=%s, want 1h", systemLogWindow)
	}
}

func TestRequestTraceReportsSystemLogFallbackFailureInsteadOfNotFound(t *testing.T) {
	var opsCalls, systemLogCalls int
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/api/v1/admin/ops/system-logs" {
			systemLogCalls++
			http.Error(writer, `{"message":"system log query unavailable"}`, http.StatusServiceUnavailable)
			return
		}
		opsCalls++
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"data":{"items":[],"total":0}}`)
	}))
	defer server.Close()

	trace, err := New(staticTarget{baseURL: server.URL}, staticAccounts{}).RequestTrace(context.Background(), "req-missing")
	if err == nil || trace.Matched || !errors.Is(err, errSystemLogLookupFailed) {
		t.Fatalf("trace=%#v err=%v", trace, err)
	}
	if opsCalls != 1 || systemLogCalls != 1 {
		t.Fatalf("ops calls=%d system log calls=%d", opsCalls, systemLogCalls)
	}
}

func TestRequestTraceExpandsSystemLogLookupToFullRetentionWindow(t *testing.T) {
	var systemLogCalls int
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if request.URL.Path != "/api/v1/admin/ops/system-logs" {
			_, _ = io.WriteString(writer, `{"data":{"items":[],"total":0}}`)
			return
		}
		systemLogCalls++
		if systemLogCalls == 1 {
			_, _ = io.WriteString(writer, `{"data":{"items":[],"total":0}}`)
			return
		}
		start, _ := time.Parse(time.RFC3339Nano, request.URL.Query().Get("start_time"))
		end, _ := time.Parse(time.RFC3339Nano, request.URL.Query().Get("end_time"))
		if end.Sub(start) != 30*24*time.Hour {
			t.Fatalf("expanded system-log window=%s, want 30d", end.Sub(start))
		}
		_, _ = io.WriteString(writer, `{"data":{"items":[{"id":10,"request_id":"req-old","level":"info","message":"completed","account_id":42}],"total":1}}`)
	}))
	defer server.Close()

	trace, err := New(staticTarget{baseURL: server.URL}, staticAccounts{}).RequestTrace(context.Background(), "req-old")
	if err != nil || !trace.Matched || len(trace.Records) != 1 || trace.Records[0].Source != "system-log" {
		t.Fatalf("trace=%#v err=%v", trace, err)
	}
	if systemLogCalls != 2 {
		t.Fatalf("system-log calls=%d, want 2", systemLogCalls)
	}
}

func TestRequestTraceKeepsMatchedRecordWhenRecentErrorsFail(t *testing.T) {
	var traceCalls, recentErrorCalls int
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Query().Get("kind") == "error" {
			recentErrorCalls++
			http.Error(writer, `{"message":"recent errors unavailable"}`, http.StatusServiceUnavailable)
			return
		}
		traceCalls++
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"data":{"items":[{"request_id":"req-found","kind":"success","account_id":42,"group_id":7,"duration_ms":123}],"total":1}}`)
	}))
	defer server.Close()

	trace, err := New(staticTarget{baseURL: server.URL}, staticAccounts{}).RequestTrace(context.Background(), "req-found")
	if err != nil || !trace.Matched || len(trace.Records) != 1 || len(trace.RecentErrors) != 0 {
		t.Fatalf("trace=%#v err=%v", trace, err)
	}
	if traceCalls != 1 || recentErrorCalls != 1 {
		t.Fatalf("trace calls=%d recent error calls=%d", traceCalls, recentErrorCalls)
	}
}

func TestSearchSystemLogsForwardsManagementPlatformFilters(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		query := request.URL.Query()
		if query.Get("time_range") != "6h" || query.Get("host") != "node-1" || query.Get("level") != "info" || query.Get("component") != "http.access" {
			t.Fatalf("primary filters=%v", query)
		}
		if query.Get("request_id") != "req-42" || query.Get("client_request_id") != "client-42" || query.Get("user_id") != "7" || query.Get("api_key_id") != "8" || query.Get("account_id") != "42" {
			t.Fatalf("identity filters=%v", query)
		}
		if query.Get("platform") != "openai" || query.Get("model") != "gpt-5" || query.Get("q") != "completed" {
			t.Fatalf("detail filters=%v", query)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"data":{"items":[{"id":9,"request_id":"req-42","level":"info","component":"http.access","message":"completed","account_id":42}],"total":1}}`)
	}))
	defer server.Close()

	page, err := New(staticTarget{baseURL: server.URL}, staticAccounts{}).SearchSystemLogs(context.Background(), SystemLogQuery{
		TimeRange: "6h", Host: "node-1", Level: "info", Component: "http.access",
		RequestID: "req-42", ClientRequestID: "client-42", UserID: "7", APIKeyID: "8", AccountID: "42",
		Platform: "openai", Model: "gpt-5", Keyword: "completed", Page: 1, PageSize: 20,
	})
	if err != nil || page.Total != 1 || len(page.Items) != 1 || page.Items[0].Source != "system-log" || page.Items[0].AccountName == nil || *page.Items[0].AccountName != "账号 A" {
		t.Fatalf("page=%#v err=%v", page, err)
	}
}

func TestSearchSystemLogsCorrelatesLegacyAccountTestErrorWithUniqueRequestPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"data":{"items":[
			{"id":10,"host":"node-1","level":"error","component":"stdlog","message":"Account test error: API returned 403","created_at":"2026-08-29T14:22:14.100Z","extra":{"legacy_stdlog":true}},
			{"id":11,"host":"node-1","level":"info","component":"http","message":"http request completed","created_at":"2026-08-29T14:22:14.101Z","extra":{"path":"/api/v1/admin/accounts/42/test","method":"POST","status":200}}
		],"total":2}}`)
	}))
	defer server.Close()

	page, err := New(staticTarget{baseURL: server.URL}, staticAccounts{}).SearchSystemLogs(context.Background(), SystemLogQuery{
		TimeRange: "1h", Page: 1, PageSize: 20,
	})
	if err != nil || len(page.Items) != 2 {
		t.Fatalf("page=%#v err=%v", page, err)
	}
	for _, item := range page.Items {
		if item.AccountID == nil || *item.AccountID != "42" || item.AccountName == nil || *item.AccountName != "账号 A" {
			t.Fatalf("account identity not enriched: %#v", item)
		}
	}
}

func TestSearchSystemLogsDoesNotGuessLegacyAccountWhenConcurrentTestsAreAmbiguous(t *testing.T) {
	rows := []map[string]any{
		{"message": "Account test error: first", "host": "node-1", "created_at": "2026-08-29T14:22:14.100Z"},
		{"message": "http request completed", "host": "node-1", "created_at": "2026-08-29T14:22:14.101Z", "extra": map[string]any{"path": "/api/v1/admin/accounts/42/test"}},
		{"message": "http request completed", "host": "node-1", "created_at": "2026-08-29T14:22:14.102Z", "extra": map[string]any{"path": "/api/v1/admin/accounts/43/test"}},
	}

	enriched := enrichLegacyAccountTestLogs(rows)
	if accountID := accountIDFromRow(enriched[0]); accountID != "" {
		t.Fatalf("ambiguous account ID should stay empty, got %q", accountID)
	}
}
