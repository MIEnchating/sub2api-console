package browserlogin_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/api"
	"github.com/MIEnchating/sub2api-console/backend/internal/authrecovery"
	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/config"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskrunner"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/upstreamauth"
)

func TestBrowserAPIRequiresValidOwnerCookieAndPreservesCredentialsOn401(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	private, err := configstore.Open(filepath.Join(dir, "private.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer private.Close()
	repo, err := business.Open(filepath.Join(dir, "business.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	tasks, err := taskstore.Open(filepath.Join(dir, "tasks.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer tasks.Close()
	if err = private.Initialize(ctx, "operator", "isolated-test-password", "https://admin.example.test", "isolated-key"); err != nil {
		t.Fatal(err)
	}
	token, err := private.CreateSession(ctx, "operator", time.Hour, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	other, err := private.CreateSession(ctx, "operator", time.Hour, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(401)
		_, _ = w.Write([]byte(`{"code":"SESSION_BINDING_MISMATCH","message":"network mismatch"}`))
	}))
	defer upstream.Close()
	old := "original-secret"
	record := configstore.AuthRecord{Host: "login.example", BaseURL: upstream.URL, UpstreamType: "sub2api", AuthMode: "sub2api_user_token", AccessToken: &old, RefreshToken: &old}
	if err = private.SaveAuthRecord(ctx, record, nil); err != nil {
		t.Fatal(err)
	}
	f := &factoryFixture{browser: &browserFixture{closed: make(chan struct{}), access: "candidate-secret"}, open: make(chan struct{}), proceed: make(chan struct{})}
	close(f.proceed)
	// The external browser boundary returns a candidate for the selected target.
	factory := &recordFactory{base: f, record: record}
	runner := taskrunner.New(ctx)
	defer func() { c, cancel := context.WithTimeout(ctx, 5*time.Second); defer cancel(); _ = runner.Shutdown(c) }()
	manager := browserlogin.New(factory, tasks, runner)
	service := authrecovery.New(repo, private, upstreamauth.New(upstream.Client()), nil, nil, tasks)
	service.UseBrowserLogin(manager)
	handler := api.New(config.Config{AdminToken: "test-admin-token"}, private, repo, api.Dependencies{AuthRecovery: service, Tasks: tasks})
	request := func(method, path, cookie string, payload any, bearer bool) *httptest.ResponseRecorder {
		raw, _ := json.Marshal(payload)
		req := httptest.NewRequest(method, path, bytes.NewReader(raw))
		req.Host = "127.0.0.1"
		req.RemoteAddr = "127.0.0.1:1234"
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", "http://127.0.0.1")
		if cookie != "" {
			req.AddCookie(&http.Cookie{Name: "sub2api_console_session", Value: cookie})
		}
		if bearer {
			req.Header.Set("Authorization", "Bearer test-admin-token")
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		return response
	}
	denied := request("POST", "/api/auth-recovery/browser", "fake", map[string]string{"host": record.Host}, true)
	if denied.Code != 403 {
		t.Fatalf("forged cookie accepted: %d %s", denied.Code, denied.Body.String())
	}
	started := request("POST", "/api/auth-recovery/browser", token, map[string]string{"host": record.Host}, false)
	if started.Code != 202 {
		t.Fatalf("start: %d %s", started.Code, started.Body.String())
	}
	var view browserlogin.View
	if err = json.Unmarshal(started.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	path := "/api/auth-recovery/browser/" + view.ID
	if started.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("sensitive response lacks no-store")
	}
	if response := request("GET", path, other, nil, false); response.Code != 404 {
		t.Fatalf("different session accessed image: %d", response.Code)
	}
	await(t, f.open)
	deadline := time.After(5 * time.Second)
	for {
		response := request("GET", path, token, nil, false)
		if response.Code != 200 {
			t.Fatal(response.Body.String())
		}
		_ = json.Unmarshal(response.Body.Bytes(), &view)
		if view.Status == "waiting" {
			break
		}
		select {
		case <-deadline:
			t.Fatal("browser did not become ready")
		default:
			runtime.Gosched()
		}
	}
	if response := request("POST", path+"/finish", token, map[string]any{}, false); response.Code != 202 {
		t.Fatal(response.Body.String())
	}
	await(t, f.browser.closed)
	stored, err := private.AuthRecord(ctx, record.Host)
	if err != nil || stored.AccessToken == nil || *stored.AccessToken != old {
		t.Fatal("failed backend verification replaced original credentials")
	}
	result := request("GET", path, token, nil, false)
	_ = json.Unmarshal(result.Body.Bytes(), &view)
	if view.Status != "failed" {
		t.Fatalf("401 verification reported %s", view.Status)
	}
}

type recordFactory struct {
	base   *factoryFixture
	record configstore.AuthRecord
}

func (f *recordFactory) Open(ctx context.Context, r configstore.AuthRecord) (browserlogin.Browser, error) {
	b, err := f.base.Open(ctx, r)
	return &recordBrowser{Browser: b, record: f.record}, err
}

type recordBrowser struct {
	browserlogin.Browser
	record configstore.AuthRecord
}

func (b *recordBrowser) Credentials(ctx context.Context) (configstore.AuthRecord, error) {
	value, err := b.Browser.Credentials(ctx)
	r := b.record
	r.AccessToken = value.AccessToken
	return r, err
}
