package accountworkbench_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin"
)

func attachSMSInput(revision string) accountworkbench.OAuthSMSAttachmentInput {
	return accountworkbench.OAuthSMSAttachmentInput{Revision: revision, SMS: *smsLoginInput().SMS}
}

func TestLiveOAuthSMSManualSessionAttachesConfirmedProviderAndCompletesOneOrder(t *testing.T) {
	f, browser := newAssistFixture(t, "phone", "sms_code")
	ticks := make(chan time.Time)
	f.service.UseOAuthAssistTicks(ticks)
	finished := installSMSFixture(t, f, "")
	f.startOAuth(t)
	state, err := f.service.ReadOAuthSMSAttachment(context.Background(), "owner", f.view.ID, "")
	if err != nil || !state.CanAttach || state.Configured || state.Stage != "phone" || state.Scope != accountworkbench.ScopeManaged {
		t.Fatalf("manual phone attachment = %+v, %v", state, err)
	}
	input := attachSMSInput(state.Revision)
	if err := f.service.AttachOAuthSMS(context.Background(), "owner", f.view.ID, input); err != nil {
		t.Fatal(err)
	}
	configured, err := f.service.ReadOAuthSMSAttachment(context.Background(), "owner", f.view.ID, "")
	if err != nil || configured.CanAttach || !configured.Configured {
		t.Fatalf("configured phone attachment = %+v, %v", configured, err)
	}
	if err := f.service.AttachOAuthSMS(context.Background(), "owner", f.view.ID, input); err == nil {
		t.Fatal("provider configuration could be submitted twice")
	}
	ticks <- time.Now()
	if action := awaitAssistAction(t, browser); action.Stage != "phone" || action.Value != "+15555550101" {
		t.Fatal("attached SMS provider did not submit its purchased phone")
	}
	ticks <- time.Now()
	if action := awaitAssistAction(t, browser); action.Stage != "sms_code" || action.Value != "654321" {
		t.Fatal("attached SMS provider did not submit the current order code")
	}
	ticks <- time.Now()
	terminal := f.awaitPhase(t, "authorized")
	if status := awaitSMSFinish(t, finished); status != "6" {
		t.Fatal("completed authorization did not complete its SMS order")
	}
	public, _ := json.Marshal([]any{state, configured, terminal})
	for _, secret := range []string{"private-sms-key", "654321", "15555550101"} {
		if strings.Contains(string(public), secret) {
			t.Fatal("SMS attachment response exposed provider credentials")
		}
	}
}

func TestLiveOAuthSMSPausedAssistResumesOnlyPhoneAndSMSInLocalScope(t *testing.T) {
	f, browser := newAssistFixture(t, "phone", "password")
	ticks := make(chan time.Time)
	f.service.UseOAuthAssistTicks(ticks)
	finished := installSMSFixture(t, f, "")
	var err error
	f.view, err = f.service.StartOAuthWithInput(context.Background(), "owner", accountworkbench.OAuthStartInput{Scope: accountworkbench.ScopeLocalExport, Login: &accountworkbench.OAuthLoginInput{Email: "owner@example.com", Password: "private-previous-password"}})
	if err != nil {
		t.Fatal(err)
	}
	f.awaitPhase(t, "waiting")
	if err := f.service.InputOAuth(context.Background(), "owner", f.view.ID, browserlogin.Input{Kind: "key", Key: "Tab"}); err != nil {
		t.Fatal(err)
	}
	state, err := f.service.ReadOAuthSMSAttachment(context.Background(), "owner", f.view.ID, "")
	if err != nil || !state.CanAttach || state.Scope != accountworkbench.ScopeLocalExport {
		t.Fatalf("local phone attachment = %+v, %v", state, err)
	}
	if err := f.service.AttachOAuthSMS(context.Background(), "owner", f.view.ID, attachSMSInput(state.Revision)); err != nil {
		t.Fatal(err)
	}
	ticks <- time.Now()
	awaitAssistAction(t, browser)
	ticks <- time.Now()
	awaitAssistMessage(t, f, "短信步骤以外")
	if len(browser.actions) != 0 {
		t.Fatal("attaching SMS resumed old password assistance")
	}
	if err := f.service.CancelOAuth("owner", f.view.ID); err != nil {
		t.Fatal(err)
	}
	f.awaitPhase(t, "cancelled")
	if status := awaitSMSFinish(t, finished); status != "8" {
		t.Fatal("cancelled attached SMS order was not released")
	}
	local, err := f.service.SMSReceiptsScoped(context.Background(), "owner", accountworkbench.ScopeLocalExport)
	managed, managedErr := f.service.SMSReceipts(context.Background(), "owner")
	if err != nil || managedErr != nil || len(local) != 1 || len(managed) != 0 {
		t.Fatal("attached SMS order did not retain local scope")
	}
}

func TestLiveOAuthSMSRejectsUnconfirmedChangedPageScopeAndOwnerWithoutPurchasing(t *testing.T) {
	f, browser := newAssistFixture(t, "phone", "email")
	ticks := make(chan time.Time)
	f.service.UseOAuthAssistTicks(ticks)
	var calls atomic.Int32
	f.service.UseProviderTransport(oauthTransportFunc(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return nil, errors.New("unexpected provider request")
	}))
	f.startOAuth(t)
	state, err := f.service.ReadOAuthSMSAttachment(context.Background(), "owner", f.view.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []string{"confirmation", "revision", "scope", "owner", "page"} {
		t.Run(invalid, func(t *testing.T) {
			input := attachSMSInput(state.Revision)
			owner := "owner"
			switch invalid {
			case "confirmation":
				input.SMS.Confirmed = false
			case "revision":
				input.Revision = strings.Repeat("0", 64)
			case "scope":
				input.Scope = accountworkbench.ScopeLocalExport
			case "owner":
				owner = "other-owner"
			case "page":
				browser.flowMu.Lock()
				browser.position = 1
				browser.flowMu.Unlock()
			}
			if err := f.service.AttachOAuthSMS(context.Background(), owner, f.view.ID, input); err == nil {
				t.Fatal("invalid SMS attachment was accepted")
			}
		})
	}
	if calls.Load() != 0 {
		t.Fatal("rejected SMS attachment purchased a number")
	}
}

func TestLiveOAuthSMSUncertainPurchaseCannotBeReconfiguredOrReplayed(t *testing.T) {
	f, _ := newAssistFixture(t, "phone")
	ticks := make(chan time.Time)
	f.service.UseOAuthAssistTicks(ticks)
	var calls atomic.Int32
	f.service.UseProviderTransport(oauthTransportFunc(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return nil, errors.New("isolated response lost")
	}))
	f.startOAuth(t)
	state, err := f.service.ReadOAuthSMSAttachment(context.Background(), "owner", f.view.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	input := attachSMSInput(state.Revision)
	if err := f.service.AttachOAuthSMS(context.Background(), "owner", f.view.ID, input); err != nil {
		t.Fatal(err)
	}
	ticks <- time.Now()
	awaitAssistMessage(t, f, "尚未确定")
	if err := f.service.AttachOAuthSMS(context.Background(), "owner", f.view.ID, input); err == nil {
		t.Fatal("uncertain purchase could be replaced by another purchase")
	}
	ticks <- time.Now()
	ticks <- time.Now()
	if calls.Load() != 1 {
		t.Fatal("uncertain SMS purchase was replayed")
	}
}

func TestLiveOAuthSMSChangedPhoneRevisionBeforeTickDoesNotPurchase(t *testing.T) {
	f, browser := newAssistFixture(t, "phone", "phone")
	ticks := make(chan time.Time)
	f.service.UseOAuthAssistTicks(ticks)
	var calls atomic.Int32
	f.service.UseProviderTransport(oauthTransportFunc(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return nil, errors.New("unexpected provider request")
	}))
	f.startOAuth(t)
	state, err := f.service.ReadOAuthSMSAttachment(context.Background(), "owner", f.view.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.service.AttachOAuthSMS(context.Background(), "owner", f.view.ID, attachSMSInput(state.Revision)); err != nil {
		t.Fatal(err)
	}
	browser.flowMu.Lock()
	browser.position = 1
	browser.flowMu.Unlock()
	ticks <- time.Now()
	awaitAssistMessage(t, f, "官方授权页面已变化")
	if calls.Load() != 0 {
		t.Fatal("changed phone page triggered a purchase")
	}
}
