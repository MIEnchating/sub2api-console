package adminclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestAccountsFollowsExplicitTotalWhenServerCapsPageSize(t *testing.T) {
	var pages []string
	client, server := testClient(t, 1, func(w http.ResponseWriter, request *http.Request) {
		pages = append(pages, request.URL.Query().Get("page"))
		if request.URL.Query().Get("page_size") != "1000" {
			t.Fatalf("page_size=%q", request.URL.Query().Get("page_size"))
		}
		if request.URL.Query().Get("page") == "1" {
			writeJSON(w, `{"data":{"items":[{"id":1},{"id":2}],"total":3}}`)
			return
		}
		writeJSON(w, `{"data":{"items":[{"id":3}],"total":3}}`)
	})
	defer server.Close()

	items, err := client.Accounts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 || strings.Join(pages, ",") != "1,2" {
		t.Fatalf("items=%#v pages=%#v", items, pages)
	}
}

func TestAccountUsageTotalsUsesExactClosedDateAndKeepsAUSemantics(t *testing.T) {
	client, server := testClient(t, 1, func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/v1/admin/usage/stats" || request.URL.Query().Get("account_id") != "41" || request.URL.Query().Get("start_date") != "2026-08-29" || request.URL.Query().Get("end_date") != "2026-08-29" || request.URL.Query().Get("timezone") != "Asia/Shanghai" {
			t.Fatalf("request=%s?%s", request.URL.Path, request.URL.RawQuery)
		}
		writeJSON(w, `{"code":0,"data":{"total_account_cost":8.25,"total_actual_cost":10.5}}`)
	})
	defer server.Close()
	totals, err := client.AccountUsageTotals(context.Background(), "41", "2026-08-29", "Asia/Shanghai")
	if err != nil || totals.AccountCost != 8.25 || totals.ActualCost != 10.5 {
		t.Fatalf("totals=%#v err=%v", totals, err)
	}
}

func TestGroupsFallsBackOnlyForMissingRouteOrContractShape(t *testing.T) {
	t.Run("missing route", func(t *testing.T) {
		client, server := testClient(t, 1, func(w http.ResponseWriter, request *http.Request) {
			if request.URL.Path == "/api/v1/admin/groups" {
				http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
				return
			}
			writeJSON(w, `{"data":[{"id":7,"name":"legacy"}]}`)
		})
		defer server.Close()
		groups, err := client.Groups(context.Background())
		if err != nil || len(groups) != 1 || groups[0]["name"] != "legacy" {
			t.Fatalf("groups=%#v err=%v", groups, err)
		}
	})

	t.Run("canonical shape failure", func(t *testing.T) {
		client, server := testClient(t, 1, func(w http.ResponseWriter, request *http.Request) {
			if request.URL.Path == "/api/v1/admin/groups" {
				writeJSON(w, `{"data":{"groups":[]}}`)
				return
			}
			writeJSON(w, `{"data":[{"id":8,"name":"legacy"}]}`)
		})
		defer server.Close()
		groups, err := client.Groups(context.Background())
		if err != nil || len(groups) != 1 || groups[0]["name"] != "legacy" {
			t.Fatalf("groups=%#v err=%v", groups, err)
		}
	})

	t.Run("authentication failure", func(t *testing.T) {
		var fallbackCalls int
		client, server := testClient(t, 1, func(w http.ResponseWriter, request *http.Request) {
			if request.URL.Path == "/api/v1/admin/groups/all" {
				fallbackCalls++
			}
			http.Error(w, `{"message":"未授权，缺少 items"}`, http.StatusUnauthorized)
		})
		defer server.Close()
		_, err := client.Groups(context.Background())
		var httpError *HTTPError
		if !errors.As(err, &httpError) || httpError.StatusCode != http.StatusUnauthorized || fallbackCalls != 0 {
			t.Fatalf("err=%v fallbackCalls=%d", err, fallbackCalls)
		}
	})
}

func TestExplicitMalformedDataDoesNotReviveTopLevelItems(t *testing.T) {
	for _, rawData := range []string{"null", `{"groups":[]}`} {
		t.Run(rawData, func(t *testing.T) {
			client, server := testClient(t, 1, func(w http.ResponseWriter, _ *http.Request) {
				writeJSON(w, `{"data":`+rawData+`,"items":[{"id":99}]}`)
			})
			defer server.Close()
			_, err := client.Accounts(context.Background())
			if err == nil || !strings.Contains(err.Error(), "data 字段无有效 items") {
				t.Fatalf("err=%v", err)
			}
		})
	}
}

func TestHTTP200BusinessFailureIsRejected(t *testing.T) {
	client, server := testClient(t, 1, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, `{"success":false,"message":"denied","data":{"items":[]}}`)
	})
	defer server.Close()
	_, err := client.Accounts(context.Background())
	if err == nil || !strings.Contains(err.Error(), "业务失败：denied") {
		t.Fatalf("err=%v", err)
	}
}

func TestHTTP200ResponseRejectsTrailingJSONData(t *testing.T) {
	_, err, retry := decodeResponse(response(http.StatusOK, `{"success":true,"data":{}} garbage`))
	if err == nil || !strings.Contains(err.Error(), "尾随数据") || retry {
		t.Fatalf("err=%v retry=%t", err, retry)
	}
}

func TestHTTPResponseRejectsBodyBeyondSafetyLimit(t *testing.T) {
	oversized := `{"success":true,"data":{}}` + strings.Repeat(" ", maximumResponseBytes)
	_, err, retry := decodeResponse(response(http.StatusOK, oversized))
	if err == nil || !strings.Contains(err.Error(), "4 MiB") || retry {
		t.Fatalf("err=%v retry=%t", err, retry)
	}
}

func TestDecodeResponseRetryClassification(t *testing.T) {
	for _, status := range []int{408, 425, 429, 500, 503} {
		_, _, retry := decodeResponse(response(status, `{"message":"retry"}`))
		if !retry {
			t.Fatalf("status %d must retry", status)
		}
	}
	for _, status := range []int{300, 302, 307, 308, 400, 401, 403, 404, 409, 422} {
		_, _, retry := decodeResponse(response(status, `{"message":"stop"}`))
		if retry {
			t.Fatalf("status %d must not retry", status)
		}
	}
}

func TestTransportFailureIsRetriedAndHeadersAreStable(t *testing.T) {
	transport := &retryTransport{}
	client, err := New(Config{BaseURL: "https://admin.example", AdminKey: "secret", Timeout: time.Second, Attempts: 2}, transport)
	if err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	items, err := client.Accounts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || transport.calls.Load() != 2 || time.Since(started) < time.Second {
		t.Fatalf("items=%#v calls=%d elapsed=%s", items, transport.calls.Load(), time.Since(started))
	}
	if transport.apiKey != "secret" || transport.accept != "application/json" {
		t.Fatalf("headers: apiKey=%q accept=%q", transport.apiKey, transport.accept)
	}
}

func TestNonIdempotentTransportFailureIsNotRetried(t *testing.T) {
	for _, method := range []string{http.MethodPost, http.MethodPatch} {
		transport := &retryTransport{}
		client, err := New(Config{BaseURL: "https://admin.example", AdminKey: "secret", Timeout: time.Second, Attempts: 3}, transport)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := client.Mutate(context.Background(), method, "/admin/accounts", map[string]any{"name": "account"}); err == nil {
			t.Fatalf("%s transport failure must be returned", method)
		}
		if calls := transport.calls.Load(); calls != 1 {
			t.Fatalf("%s was retried %d times; non-idempotent writes must not be replayed", method, calls)
		}
	}
}

func TestStableIDsAreRequired(t *testing.T) {
	client, server := testClient(t, 1, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, `{"data":{"id":1}}`)
	})
	defer server.Close()
	for _, id := range []string{"", "0", "-1", "name", " 1 "} {
		if _, err := client.Account(context.Background(), id); err == nil {
			t.Fatalf("account ID %q accepted", id)
		}
		if _, err := client.Group(context.Background(), id); err == nil {
			t.Fatalf("group ID %q accepted", id)
		}
	}
}

func TestCreateAccountAcceptsNestedAccountResponseAfterCapturingIdentityBaseline(t *testing.T) {
	var lists, posts int
	client, server := testClient(t, 1, func(w http.ResponseWriter, request *http.Request) {
		switch request.Method {
		case http.MethodGet:
			lists++
			writeJSON(w, `{"data":{"items":[],"total":0}}`)
		case http.MethodPost:
			posts++
			writeJSON(w, `{"success":true,"data":{"account":{"accountId":42,"name":"alpha"}}}`)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
	defer server.Close()

	created, err := client.CreateAccount(context.Background(), map[string]any{
		"name": "alpha", "platform": "openai", "type": "apikey", "group_ids": []int64{3},
	})
	if err != nil || strings.TrimSpace(fmt.Sprint(created["id"])) != "42" || lists != 1 || posts != 1 {
		t.Fatalf("created=%#v err=%v lists=%d posts=%d", created, err, lists, posts)
	}
}

func TestCreateAccountAcceptsScalarDataID(t *testing.T) {
	var lists, posts int
	client, server := testClient(t, 1, func(w http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodGet {
			lists++
			writeJSON(w, `{"data":{"items":[],"total":0}}`)
			return
		}
		posts++
		writeJSON(w, `{"success":true,"data":42}`)
	})
	defer server.Close()

	created, err := client.CreateAccount(context.Background(), map[string]any{
		"name": "alpha", "platform": "openai", "type": "apikey", "group_ids": []int64{3},
	})
	if err != nil || strings.TrimSpace(fmt.Sprint(created["id"])) != "42" || lists != 1 || posts != 1 {
		t.Fatalf("created=%#v err=%v lists=%d posts=%d", created, err, lists, posts)
	}
}

func TestCreateAccountUsesStableResponseIDWhenIdentityBaselineIsUnavailable(t *testing.T) {
	var lists, posts int
	client, server := testClient(t, 1, func(w http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodGet {
			lists++
			w.WriteHeader(http.StatusServiceUnavailable)
			writeJSON(w, `{"error":"temporarily unavailable"}`)
			return
		}
		posts++
		writeJSON(w, `{"success":true,"data":{"id":42,"name":"alpha"}}`)
	})
	defer server.Close()

	created, err := client.CreateAccount(context.Background(), map[string]any{
		"name": "alpha", "platform": "openai", "type": "apikey", "group_ids": []int64{3},
	})
	if err != nil || strings.TrimSpace(fmt.Sprint(created["id"])) != "42" || lists != 1 || posts != 1 {
		t.Fatalf("created=%#v err=%v lists=%d posts=%d", created, err, lists, posts)
	}
}

func TestCreateAccountRecoversMissingResponseIDFromStableDirectoryDifference(t *testing.T) {
	var lists, posts int
	client, server := testClient(t, 1, func(w http.ResponseWriter, request *http.Request) {
		switch request.Method {
		case http.MethodGet:
			lists++
			items := `[{"id":9,"name":"existing","platform":"openai","type":"apikey","group_ids":[3]}]`
			if lists == 2 {
				items = `[{"id":9,"name":"existing","platform":"openai","type":"apikey","group_ids":[3]},` +
					`{"id":42,"name":"alpha","platform":"openai","type":"apikey","group_ids":[3]}]`
			}
			writeJSON(w, `{"data":{"items":`+items+`,"total":`+strconv.Itoa(lists)+`}}`)
		case http.MethodPost:
			posts++
			writeJSON(w, `{"success":true}`)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
	defer server.Close()

	created, err := client.CreateAccount(context.Background(), map[string]any{
		"name": "alpha", "platform": "openai", "type": "apikey", "group_ids": []int64{3},
	})
	if err != nil || strings.TrimSpace(fmt.Sprint(created["id"])) != "42" || lists != 2 || posts != 1 {
		t.Fatalf("created=%#v err=%v lists=%d posts=%d", created, err, lists, posts)
	}
}

func TestCreateAccountRejectsAmbiguousDirectoryDifferenceWhenResponseIDIsMissing(t *testing.T) {
	var gets, posts int
	client, server := testClient(t, 1, func(w http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodGet {
			gets++
			items := `[]`
			total := 0
			if gets == 2 {
				items = `[{"id":42,"name":"alpha","platform":"openai","type":"apikey","group_ids":[3]},` +
					`{"id":43,"name":"alpha","platform":"openai","type":"apikey","group_ids":[3]}]`
				total = 2
			}
			writeJSON(w, `{"data":{"items":`+items+`,"total":`+strconv.Itoa(total)+`}}`)
			return
		}
		posts++
		writeJSON(w, `{"success":true}`)
	})
	defer server.Close()

	_, err := client.CreateAccount(context.Background(), map[string]any{
		"name": "alpha", "platform": "openai", "type": "apikey", "group_ids": []int64{3},
	})
	if err == nil || !strings.Contains(err.Error(), "无法唯一确认") || gets != 2 || posts != 1 {
		t.Fatalf("err=%v gets=%d posts=%d", err, gets, posts)
	}
}

func TestDeleteAccountRequiresAbsentReadback(t *testing.T) {
	t.Run("confirmed absent", func(t *testing.T) {
		var calls int
		client, server := testClient(t, 1, func(w http.ResponseWriter, request *http.Request) {
			calls++
			if request.Method == http.MethodDelete {
				writeJSON(w, `{"success":true}`)
				return
			}
			http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		})
		defer server.Close()
		result, err := client.DeleteAccount(context.Background(), "42")
		if err != nil || result["confirmed_absent"] != true || calls != 2 {
			t.Fatalf("result=%#v err=%v calls=%d", result, err, calls)
		}
	})

	t.Run("still readable", func(t *testing.T) {
		client, server := testClient(t, 1, func(w http.ResponseWriter, request *http.Request) {
			if request.Method == http.MethodDelete {
				writeJSON(w, `{"success":true}`)
				return
			}
			writeJSON(w, `{"data":{"id":42}}`)
		})
		defer server.Close()
		_, err := client.DeleteAccount(context.Background(), "42")
		if err == nil || !strings.Contains(err.Error(), "删除后仍可读") {
			t.Fatalf("err=%v", err)
		}
	})
}

func TestDeleteAccountCanSkipAbsentReadback(t *testing.T) {
	var calls int
	client, server := testClient(t, 1, func(w http.ResponseWriter, request *http.Request) {
		calls++
		writeJSON(w, `{"success":true}`)
	})
	defer server.Close()
	result, err := client.DeleteAccountWithVerification(context.Background(), "42", false)
	if err != nil || result["deleted"] != true || result["confirmed_absent"] != false || calls != 1 {
		t.Fatalf("result=%#v err=%v calls=%d", result, err, calls)
	}
}

func TestRequestTraceUsesOnlyOpsRequests(t *testing.T) {
	var path string
	var query string
	client, server := testClient(t, 1, func(w http.ResponseWriter, request *http.Request) {
		path = request.URL.Path
		query = request.URL.Query().Get("request_id")
		writeJSON(w, `{"data":{"items":[{"request_id":"req-3","kind":"error"}],"total":1}}`)
	})
	defer server.Close()
	items, err := client.RequestTrace(context.Background(), "req-3", 120, 60)
	if err != nil || len(items) != 1 || path != "/api/v1/admin/ops/requests" || query != "req-3" {
		t.Fatalf("items=%#v err=%v path=%q query=%q", items, err, path, query)
	}
}

func TestRequestTraceAcceptsMultipleEvidenceRowsForTheSameRequest(t *testing.T) {
	client, server := testClient(t, 1, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, `{"data":{"items":[
			{"request_id":"req-shared","account_id":41,"group_id":7,"kind":"success","created_at":"2026-08-28T10:00:00Z"},
			{"request_id":"req-shared","account_id":41,"group_id":8,"kind":"success","created_at":"2026-08-28T10:00:01Z"}
		],"total":2}}`)
	})
	defer server.Close()

	items, err := client.RequestTrace(context.Background(), "req-shared", 120, 60)
	if err != nil || len(items) != 2 {
		t.Fatalf("items=%#v err=%v", items, err)
	}
}

func TestSystemLogsByRequestIDUsesExactIndexedFilter(t *testing.T) {
	var path, requestID, query string
	client, server := testClient(t, 1, func(w http.ResponseWriter, request *http.Request) {
		path = request.URL.Path
		requestID = request.URL.Query().Get("request_id")
		query = request.URL.Query().Get("q")
		writeJSON(w, `{"data":{"items":[{"id":9,"request_id":"req-log","level":"info"}],"total":1}}`)
	})
	defer server.Close()
	items, err := client.SystemLogsByRequestID(context.Background(), "req-log", 120, 60)
	if err != nil || len(items) != 1 || path != "/api/v1/admin/ops/system-logs" || requestID != "req-log" || query != "" {
		t.Fatalf("items=%#v err=%v path=%q requestID=%q query=%q", items, err, path, requestID, query)
	}
}

func TestSystemLogsPreservesFiltersAndPagination(t *testing.T) {
	client, server := testClient(t, 1, func(w http.ResponseWriter, request *http.Request) {
		query := request.URL.Query()
		if request.URL.Path != "/api/v1/admin/ops/system-logs" || query.Get("page") != "2" || query.Get("page_size") != "20" {
			t.Fatalf("path=%q query=%v", request.URL.Path, query)
		}
		if query.Get("request_id") != "req-42" || query.Get("client_request_id") != "client-42" || query.Get("component") != "http.access" || query.Get("q") != "completed" {
			t.Fatalf("filters=%v", query)
		}
		writeJSON(w, `{"data":{"items":[{"id":9,"request_id":"req-42","level":"info"}],"total":21}}`)
	})
	defer server.Close()

	page, err := client.SystemLogs(context.Background(), map[string]string{
		"request_id": "req-42", "client_request_id": "client-42", "component": "http.access", "q": "completed",
	}, 2, 20)
	if err != nil || len(page.Items) != 1 || page.Total != 21 || page.Page != 2 || page.PageSize != 20 {
		t.Fatalf("page=%#v err=%v", page, err)
	}
}

func TestClientDoesNotFollowRedirects(t *testing.T) {
	var destinationCalls int
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		destinationCalls++
		writeJSON(w, `{"data":{"items":[],"total":0}}`)
	}))
	defer destination.Close()
	client, source := testClient(t, 1, func(w http.ResponseWriter, _ *http.Request) {
		http.Redirect(w, &http.Request{}, destination.URL, http.StatusFound)
	})
	defer source.Close()
	_, err := client.Accounts(context.Background())
	if err == nil || destinationCalls != 0 {
		t.Fatalf("err=%v destinationCalls=%d", err, destinationCalls)
	}
}

func TestAccountUpstreamMultiplierPrefersResolvedUpstreamValue(t *testing.T) {
	client, server := testClient(t, 1, func(w http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/api/v1/admin/accounts/41/upstream-billing-probe" {
			t.Fatalf("request=%s %s", request.Method, request.URL.Path)
		}
		writeJSON(w, `{"data":{"account_id":41,"snapshot":{"status":"ok","data":{"billing_scope":"token","group_rate_multiplier":0.198,"user_rate_multiplier":0.15,"resolved_rate_multiplier":0.15,"effective_rate_multiplier":0.3}}}}`)
	})
	defer server.Close()

	value, err := client.AccountUpstreamMultiplier(context.Background(), "41")
	if err != nil || value != "0.15" {
		t.Fatalf("value=%q err=%v", value, err)
	}
}

func TestAccountUpstreamMultiplierFallsBackToResolvedValue(t *testing.T) {
	client, server := testClient(t, 1, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, `{"data":{"snapshot":{"status":"ok","data":{"billing_scope":"token","resolved_rate_multiplier":"0.42"}}}}`)
	})
	defer server.Close()

	value, err := client.AccountUpstreamMultiplier(context.Background(), "41")
	if err != nil || value != "0.42" {
		t.Fatalf("value=%q err=%v", value, err)
	}
}

func TestUpdateAccountGroupsConfirmsNumericIDsIndependentOfResponseOrder(t *testing.T) {
	requests := 0
	client, server := testClient(t, 1, func(w http.ResponseWriter, request *http.Request) {
		requests++
		switch {
		case request.Method == http.MethodPut && request.URL.Path == "/api/v1/admin/accounts/41":
			writeJSON(w, `{"data":{"id":41}}`)
		case request.Method == http.MethodGet && request.URL.Path == "/api/v1/admin/accounts/41":
			writeJSON(w, `{"data":{"id":41,"group_ids":[10,2]}}`)
		default:
			t.Fatalf("request=%s %s", request.Method, request.URL.Path)
		}
	})
	defer server.Close()

	account, err := client.UpdateAccountGroups(context.Background(), "41", []int64{2, 10})
	if err != nil || requests != 2 || fmt.Sprint(account["id"]) != "41" {
		t.Fatalf("requests=%d account=%#v err=%v", requests, account, err)
	}
}

func TestAccountUpstreamMultiplierRejectsFailedOrInvalidProbe(t *testing.T) {
	for _, test := range []struct {
		name string
		body string
		want string
	}{
		{name: "failed", body: `{"data":{"snapshot":{"status":"failed","last_error":"credential rejected"}}}`, want: "credential rejected"},
		{name: "zero", body: `{"data":{"snapshot":{"status":"ok","data":{"effective_rate_multiplier":0}}}}`, want: "非法倍率"},
		{name: "missing", body: `{"data":{"snapshot":{"status":"ok","data":{}}}}`, want: "未返回有效倍率"},
	} {
		t.Run(test.name, func(t *testing.T) {
			client, server := testClient(t, 1, func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, test.body) })
			defer server.Close()
			_, err := client.AccountUpstreamMultiplier(context.Background(), "41")
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("err=%v want=%q", err, test.want)
			}
		})
	}
}

func TestAccountUpstreamMultipliersUsesOneBatchAndKeepsItemFailures(t *testing.T) {
	var requests int
	client, server := testClient(t, 1, func(w http.ResponseWriter, request *http.Request) {
		requests++
		if request.Method != http.MethodPost || request.URL.Path != "/api/v1/admin/accounts/upstream-billing-probe/batch" {
			t.Fatalf("request=%s %s", request.Method, request.URL.Path)
		}
		var body struct {
			AccountIDs []int64 `json:"account_ids"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil || len(body.AccountIDs) != 2 || body.AccountIDs[0] != 41 || body.AccountIDs[1] != 42 {
			t.Fatalf("body=%#v err=%v", body, err)
		}
		writeJSON(w, `{"data":{"results":[{"account_id":41,"snapshot":{"status":"ok","data":{"resolved_rate_multiplier":0.15,"effective_rate_multiplier":0.3}}},{"account_id":42,"snapshot":{"status":"failed","last_error":"upstream rejected"}}]}}`)
	})
	defer server.Close()

	results, err := client.AccountUpstreamMultipliers(context.Background(), []string{"41", "42"})
	if err != nil || requests != 1 || results["41"].Multiplier != "0.15" || results["41"].Err != nil || results["42"].Err == nil || !strings.Contains(results["42"].Err.Error(), "upstream rejected") {
		t.Fatalf("requests=%d results=%#v err=%v", requests, results, err)
	}
}

func testClient(t *testing.T, attempts int, handler http.HandlerFunc) (*Client, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(handler)
	client, err := New(Config{BaseURL: server.URL, AdminKey: "test-key", Timeout: time.Second, Attempts: attempts}, nil)
	if err != nil {
		server.Close()
		t.Fatal(err)
	}
	return client, server
}

func writeJSON(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.WriteString(w, body)
}

func response(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}
}

type retryTransport struct {
	calls  atomic.Int32
	apiKey string
	accept string
}

func (transport *retryTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	call := transport.calls.Add(1)
	transport.apiKey = request.Header.Get("X-API-Key")
	transport.accept = request.Header.Get("Accept")
	if call == 1 {
		return nil, errors.New("temporary transport failure")
	}
	return response(http.StatusOK, `{"data":{"items":[{"id":`+strconv.Itoa(int(call))+`}],"total":1}}`), nil
}
