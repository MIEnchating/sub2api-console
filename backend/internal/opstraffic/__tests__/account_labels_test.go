package opstraffic_test

import (
	"context"
	"database/sql"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/opstraffic"
)

func accountLabelStore(t testing.TB) (*business.Store, *sql.DB) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "log-labels.sqlite3")
	store, err := business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Bootstrap(context.Background()); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`INSERT INTO accounts(id,name,metadata_json,updated_at) VALUES
		('41','请求账号','{}','now'),('42','重试账号','{}','now');
		INSERT INTO account_groups(account_id,group_name,group_id) VALUES
		('41','原分组','7'),('42','重试分组','8');`); err != nil {
		t.Fatal(err)
	}
	return store, db
}

func TestSystemLogLabelsDoNotDependOnHealthHistory(t *testing.T) {
	store, db := accountLabelStore(t)
	if _, err := db.Exec(`DROP TABLE account_stability_samples`); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"items":[{"id":1,"account_id":41,"request_id":"request-1","level":"info"}],"total":1}}`)
	}))
	defer server.Close()
	page, err := opstraffic.New(targetRepository{url: server.URL}, store).SearchSystemLogs(context.Background(), opstraffic.SystemLogQuery{Page: 1, PageSize: 20})
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("system logs = %#v, %v", page, err)
	}
	if name := page.Items[0].AccountName; name == nil || *name != "请求账号" {
		t.Fatalf("unrelated health history prevented account label: %#v", page.Items[0])
	}
}

func TestRequestTraceLabelsDoNotDependOnHealthHistory(t *testing.T) {
	store, db := accountLabelStore(t)
	if _, err := db.Exec(`DROP TABLE account_stability_samples`); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("kind") == "error" {
			_, _ = io.WriteString(w, `{"data":{"items":[],"total":0}}`)
			return
		}
		_, _ = io.WriteString(w, `{"data":{"items":[{"request_id":"request-1","account_id":41,"group_id":7,"kind":"error"},{"request_id":"request-1","account_id":42,"group_id":8,"kind":"success"}],"total":2}}`)
	}))
	defer server.Close()
	trace, err := opstraffic.New(targetRepository{url: server.URL}, store).RequestTrace(context.Background(), "request-1")
	if err != nil || len(trace.Records) != 2 {
		t.Fatalf("request trace = %#v, %v", trace, err)
	}
	for index, want := range []struct{ name, group string }{{"请求账号", "原分组"}, {"重试账号", "重试分组"}} {
		record := trace.Records[index]
		if record.AccountName == nil || *record.AccountName != want.name || record.GroupName == nil || *record.GroupName != want.group {
			t.Fatalf("unrelated health history prevented trace identity: %#v", record)
		}
	}
}

func TestTraceWithoutFirstAccountStillLabelsOtherMatchedAccounts(t *testing.T) {
	store, _ := accountLabelStore(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"items":[{"request_id":"request-1","kind":"error"},{"request_id":"request-1","account_id":42,"group_id":8,"kind":"success"}],"total":2}}`)
	}))
	defer server.Close()
	trace, err := opstraffic.New(targetRepository{url: server.URL}, store).RequestTrace(context.Background(), "request-1")
	if err != nil || len(trace.Records) != 2 {
		t.Fatalf("request trace = %#v, %v", trace, err)
	}
	if record := trace.Records[1]; record.AccountName == nil || *record.AccountName != "重试账号" {
		t.Fatalf("first unknown account prevented later account label: %#v", record)
	}
	if trace.AccountID != nil || trace.Records[0].AccountName != nil {
		t.Fatalf("trace inferred identity for unknown first account: %#v", trace)
	}
}

func TestSystemLogInvalidOuterAccountUsesTheSameNestedIdentityForIDAndName(t *testing.T) {
	store, _ := accountLabelStore(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"items":[{"id":1,"account_id":0,"extra":{"account_id":42},"level":"error"}],"total":1}}`)
	}))
	defer server.Close()
	page, err := opstraffic.New(targetRepository{url: server.URL}, store).SearchSystemLogs(context.Background(), opstraffic.SystemLogQuery{Page: 1, PageSize: 20})
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("system logs = %#v, %v", page, err)
	}
	if record := page.Items[0]; record.AccountID == nil || *record.AccountID != "42" || record.AccountName == nil || *record.AccountName != "重试账号" {
		t.Fatalf("account ID and name describe different identities: %#v", record)
	}
}

func TestRequestTraceRecentErrorsExcludeUnrelatedAccounts(t *testing.T) {
	store, _ := accountLabelStore(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("kind") == "error" {
			_, _ = io.WriteString(w, `{"data":{"items":[{"request_id":"other-error","account_id":42,"kind":"error"},{"request_id":"own-success","account_id":41,"kind":"success"},{"request_id":"own-error","account_id":41,"kind":"error"}],"total":3}}`)
			return
		}
		_, _ = io.WriteString(w, `{"data":{"items":[{"request_id":"request-1","account_id":41,"kind":"success"}],"total":1}}`)
	}))
	defer server.Close()
	trace, err := opstraffic.New(targetRepository{url: server.URL}, store).RequestTrace(context.Background(), "request-1")
	if err != nil || len(trace.RecentErrors) != 1 || trace.RecentErrors[0].RequestID != "own-error" {
		t.Fatalf("recent errors include another account: %#v, %v", trace.RecentErrors, err)
	}
}

func TestSystemLogLabelReadFailureKeepsRemoteRecords(t *testing.T) {
	store, db := accountLabelStore(t)
	if _, err := db.Exec(`DROP TABLE account_groups; DROP TABLE accounts`); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"items":[{"id":1,"account_id":41,"request_id":"request-1","level":"error","message":"保留原始错误"}],"total":1}}`)
	}))
	defer server.Close()
	page, err := opstraffic.New(targetRepository{url: server.URL}, store).SearchSystemLogs(context.Background(), opstraffic.SystemLogQuery{Page: 1, PageSize: 20})
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("label read failure hid remote logs: %#v, %v", page, err)
	}
	if record := page.Items[0]; record.AccountName != nil || record.AccountID == nil || *record.AccountID != "41" || record.ErrorReason == nil || *record.ErrorReason != "保留原始错误" {
		t.Fatalf("label failure changed remote identity/error: %#v", record)
	}
}
