package accountworkbench_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
	"github.com/pquerna/otp/totp"
)

type assistBrowser struct {
	*oauthTestBrowser
	flowMu      sync.Mutex
	stages      []string
	position    int
	actions     chan browserlogin.AuthAction
	fail        bool
	inspections chan struct{}
}

func (b *assistBrowser) OpenOAuth(ctx context.Context, options browserlogin.OAuthOptions) (browserlogin.OAuthBrowser, error) {
	_, err := b.oauthTestBrowser.OpenOAuth(ctx, options)
	return b, err
}
func (b *assistBrowser) AuthorizationCode(ctx context.Context) (browserlogin.OAuthResult, error) {
	b.flowMu.Lock()
	done := b.position >= len(b.stages)
	b.flowMu.Unlock()
	if !done {
		return browserlogin.OAuthResult{}, browserlogin.ErrOAuthPending
	}
	return b.oauthTestBrowser.AuthorizationCode(ctx)
}
func (b *assistBrowser) InspectAuth(context.Context) (browserlogin.AuthPage, error) {
	b.flowMu.Lock()
	defer b.flowMu.Unlock()
	if b.inspections != nil {
		b.inspections <- struct{}{}
	}
	return browserlogin.AuthPage{Stage: b.stages[b.position], Revision: fmt.Sprintf("%064x", b.position+1)}, nil
}
func (b *assistBrowser) ApplyAuth(_ context.Context, action browserlogin.AuthAction) error {
	b.flowMu.Lock()
	defer b.flowMu.Unlock()
	b.actions <- action
	if b.fail {
		return errors.New("private transport echoed credential:" + action.Value)
	}
	b.position++
	return nil
}

func newAssistFixture(t *testing.T, stages ...string) (*oauthFixture, *assistBrowser) {
	t.Helper()
	f := newOAuthFixture(t)
	browser := &assistBrowser{oauthTestBrowser: f.browser, stages: stages, actions: make(chan browserlogin.AuthAction, 10)}
	f.service.UseOAuthBrowser(browser)
	f.service.UseOAuthTransport(oauthTransportFunc(func(*http.Request) (*http.Response, error) {
		f.calls.Add(1)
		payload := base64.RawURLEncoding.EncodeToString([]byte(`{"email":"owner@example.com"}`))
		return oauthResponse(`{"access_token":"eyJhbGciOiJub25lIn0.` + payload + `.test-only","refresh_token":"rt_test_private","token_type":"Bearer"}`), nil
	}))
	return f, browser
}

func startAssisted(t *testing.T, f *oauthFixture, login accountworkbench.OAuthLoginInput) {
	t.Helper()
	var err error
	f.view, err = f.service.StartOAuthWithInput(context.Background(), "owner", accountworkbench.OAuthStartInput{Login: &login})
	if err != nil {
		t.Fatal(err)
	}
	f.awaitPhase(t, "waiting")
}

func awaitAssistAction(t *testing.T, b *assistBrowser) browserlogin.AuthAction {
	t.Helper()
	select {
	case action := <-b.actions:
		return action
	case <-time.After(8 * time.Second):
		t.Fatal("authorization form was not submitted")
		return browserlogin.AuthAction{}
	}
}

func awaitAssistMessage(t *testing.T, f *oauthFixture, message string) taskstore.Task {
	t.Helper()
	deadline := time.NewTimer(8 * time.Second)
	defer deadline.Stop()
	for {
		select {
		case task := <-f.events:
			if strings.Contains(task.Message, message) {
				return task
			}
		case <-deadline.C:
			t.Fatal("authorization message not observed: " + message)
			return taskstore.Task{}
		}
	}
}

func TestOAuthAssistUsesNewMailboxCodeAfterBaselineAndReturnsNoSecrets(t *testing.T) {
	f, browser := newAssistFixture(t, "email", "email_code")
	var mailCalls atomic.Int32
	f.service.UseProviderTransport(oauthTransportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != "https://mail.example/messages" || r.Header.Get("Authorization") != "Bearer mailbox-private" {
			t.Error("mailbox request changed")
		}
		if mailCalls.Add(1) == 1 {
			return oauthResponse(`{"messages":[{"id":"old","code":"123456"}]}`), nil
		}
		return oauthResponse(`{"messages":[{"id":"old","code":"123456"},{"id":"new","code":"654321"}]}`), nil
	}))
	startAssisted(t, f, accountworkbench.OAuthLoginInput{Email: "owner@example.com", Mailbox: &accountworkbench.OAuthMailboxInput{Kind: "http", URL: "https://mail.example/messages", Headers: map[string]string{"Authorization": "Bearer mailbox-private"}}})
	if action := awaitAssistAction(t, browser); action.Stage != "email" || action.Value != "owner@example.com" || mailCalls.Load() < 1 {
		t.Fatal("login started without mailbox baseline")
	}
	if action := awaitAssistAction(t, browser); action.Stage != "email_code" || action.Value != "654321" {
		t.Fatalf("wrong verification candidate: stage=%s", action.Stage)
	}
	task := f.awaitPhase(t, "authorized")
	raw, _ := json.Marshal(task)
	for _, secret := range []string{"654321", "mailbox-private", "rt_test_private"} {
		if strings.Contains(string(raw), secret) {
			t.Fatal("task leaked login credential")
		}
	}
}

func TestOAuthAssistGeneratesTOTPForRecognizedChallenge(t *testing.T) {
	f, browser := newAssistFixture(t, "totp_code")
	secret := "JBSWY3DPEHPK3PXP"
	startAssisted(t, f, accountworkbench.OAuthLoginInput{Email: "owner@example.com", TOTPSecret: secret})
	action := awaitAssistAction(t, browser)
	if action.Stage != "totp_code" || !totp.Validate(action.Value, secret) {
		t.Fatal("invalid generated TOTP")
	}
	f.awaitPhase(t, "authorized")
}

func TestOAuthAssistUncertainSubmitPausesWithoutLeakingPassword(t *testing.T) {
	f, browser := newAssistFixture(t, "password")
	browser.fail = true
	ticks := make(chan time.Time)
	f.service.UseOAuthAssistTicks(ticks)
	startAssisted(t, f, accountworkbench.OAuthLoginInput{Email: "owner@example.com", Password: "private-password"})
	ticks <- time.Now()
	awaitAssistAction(t, browser)
	task := awaitAssistMessage(t, f, "自动填写未确认成功")
	if strings.Contains(task.Message, "private-password") {
		t.Fatal("password leaked in task message")
	}
	ticks <- time.Now()
	ticks <- time.Now()
	if err := f.service.FinishOAuth("owner", f.view.ID); err != nil {
		t.Fatal(err)
	}
	f.awaitPhase(t, "waiting")
	if len(browser.actions) != 0 || f.calls.Load() != 0 {
		t.Fatal("uncertain credential submit replayed")
	}
}

func TestOAuthAssistManualInputStopsSubsequentAutomaticSubmissions(t *testing.T) {
	f, browser := newAssistFixture(t, "email", "password")
	ticks := make(chan time.Time)
	f.service.UseOAuthAssistTicks(ticks)
	startAssisted(t, f, accountworkbench.OAuthLoginInput{Email: "owner@example.com", Password: "private-password"})
	ticks <- time.Now()
	awaitAssistAction(t, browser)
	if err := f.service.InputOAuth(context.Background(), "owner", f.view.ID, browserlogin.Input{Kind: "key", Key: "Tab"}); err != nil {
		t.Fatal(err)
	}
	ticks <- time.Now()
	ticks <- time.Now()
	if err := f.service.FinishOAuth("owner", f.view.ID); err != nil {
		t.Fatal(err)
	}
	f.awaitPhase(t, "waiting")
	if len(browser.actions) != 0 {
		t.Fatal("automation continued after operator input")
	}
}

func TestOAuthAssistRejectsMismatchedMailboxAndInvalidTOTPBeforeLaunch(t *testing.T) {
	for _, login := range []accountworkbench.OAuthLoginInput{
		{Email: "invalid"},
		{Email: "owner@example.com", TOTPSecret: "INVALID!BASE32KEY"},
		{Email: "owner@example.com", Mailbox: &accountworkbench.OAuthMailboxInput{Kind: "microsoft", Email: "other@example.com"}},
	} {
		f, _ := newAssistFixture(t, "email")
		if _, err := f.service.StartOAuthWithInput(context.Background(), "owner", accountworkbench.OAuthStartInput{Login: &login}); err == nil {
			t.Fatal("invalid login input accepted")
		}
	}
}

func TestOAuthAssistRejectsAuthorizationForDifferentEmail(t *testing.T) {
	f, _ := newAssistFixture(t)
	startAssisted(t, f, accountworkbench.OAuthLoginInput{Email: "different@example.com"})
	f.awaitPhase(t, "failed")
	if _, err := f.service.PreviewOAuth(context.Background(), "owner", f.view.ID, accountworkbench.OAuthPreviewInput{}); err == nil {
		t.Fatal("different identity exposed to import")
	}
}

func TestOAuthAssistDoesNotResubmitPasswordWhenRejectedFormGetsNewRevision(t *testing.T) {
	f, browser := newAssistFixture(t, "password", "password")
	ticks := make(chan time.Time)
	f.service.UseOAuthAssistTicks(ticks)
	startAssisted(t, f, accountworkbench.OAuthLoginInput{Email: "owner@example.com", Password: "private-password"})
	ticks <- time.Now()
	awaitAssistAction(t, browser)
	ticks <- time.Now()
	ticks <- time.Now()
	if len(browser.actions) != 0 {
		t.Fatal("rejected password was automatically resubmitted after form revision changed")
	}
}
