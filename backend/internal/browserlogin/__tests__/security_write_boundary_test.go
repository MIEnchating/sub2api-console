package browserlogin_test

import (
	"errors"
	"net/http"
	"runtime"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin"
)

func TestIsolatedSecurityRejectsPasswordRequestBeforeConfirmedSubmission(t *testing.T) {
	fixture := &securityHTTPFixture{t: t}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/log-in" {
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<html><body><button autofocus onclick="fetch('/api/accounts/password/add',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({password:'StrongPassword-2026!'})}).catch(()=>{}).finally(()=>location.assign('https://chatgpt.com/'))">Complete login</button></body></html>`))
			return
		}
		fixture.ServeHTTP(w, r)
	})
	ctx, browser := isolatedSecurityBrowser(t, handler)
	completeSecurityLogin(t, ctx, browser)
	if fixture.passwords.Load() != 0 {
		t.Fatal("official page sent password write before confirmed submission")
	}
}

func TestIsolatedSecurityRepeatedPagePasswordRequestsReachOfficialEndpointOnce(t *testing.T) {
	fixture := &securityHTTPFixture{t: t}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/reset-password/new-password" {
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<html><body><form action="/api/accounts/password/add" onsubmit="event.preventDefault();const options={method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({password:this.password.value})};Promise.allSettled([fetch(this.action,options),fetch(this.action,options)]).then(()=>location.assign('https://chatgpt.com/'))"><input name="password" type="password"><button type="submit">Save password</button></form></body></html>`))
			return
		}
		fixture.ServeHTTP(w, r)
	})
	ctx, browser := isolatedSecurityBrowser(t, handler)
	identity := completeSecurityLogin(t, ctx, browser)
	if err := browser.BeginPassword(ctx, identity); err != nil {
		t.Fatal(err)
	}
	form := awaitSecurityPage(t, ctx, browser, "new_password")
	if err := browser.SubmitPassword(ctx, browserlogin.AuthAction{Stage: form.Stage, Revision: form.Revision, Value: "StrongPassword-2026!"}); err != nil {
		t.Fatal(err)
	}
	for ctx.Err() == nil {
		ok, err := browser.PasswordResult(ctx)
		if ok && err == nil {
			break
		}
		if !errors.Is(err, browserlogin.ErrSecurityPending) {
			t.Fatal(err)
		}
		runtime.Gosched()
	}
	if ctx.Err() != nil {
		t.Fatal("official password result was not verified")
	}
	if fixture.passwords.Load() != 1 {
		t.Fatal("page repeated a confirmed password write")
	}
}

func TestIsolatedSecurityPasswordBusinessErrorWithContinueURLIsNotSuccess(t *testing.T) {
	fixture := &securityHTTPFixture{t: t}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/accounts/password/add" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"error":{"code":"password_rejected"},"continue_url":"https://chatgpt.com/"}`))
			return
		}
		fixture.ServeHTTP(w, r)
	})
	ctx, browser := isolatedSecurityBrowser(t, handler)
	identity := completeSecurityLogin(t, ctx, browser)
	if err := browser.BeginPassword(ctx, identity); err != nil {
		t.Fatal(err)
	}
	form := awaitSecurityPage(t, ctx, browser, "new_password")
	if err := browser.SubmitPassword(ctx, browserlogin.AuthAction{Stage: form.Stage, Revision: form.Revision, Value: "StrongPassword-2026!"}); err != nil {
		t.Fatal(err)
	}
	for ctx.Err() == nil {
		if _, err := browser.Identity(ctx); err == nil {
			break
		}
		runtime.Gosched()
	}
	if ctx.Err() != nil {
		t.Fatal("test page did not finish processing the business failure")
	}
	if ok, err := browser.PasswordResult(ctx); ok || err == nil {
		t.Fatal("HTTP 200 business failure was reported as a successful password write")
	}
}
