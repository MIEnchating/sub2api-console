package accountworkbench_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
)

func smsLoginInput() accountworkbench.OAuthLoginInput {
	return accountworkbench.OAuthLoginInput{Email: "owner@example.com", SMS: &accountworkbench.OAuthSMSInput{Provider: "smsbower", APIKey: "private-sms-key", Country: "6", MaxPrice: "0.123456789123456789", Confirmed: true}}
}

func installSMSFixture(t *testing.T, f *oauthFixture, finishReply string) <-chan string {
	t.Helper()
	finished := make(chan string, 2)
	f.service.UseProviderTransport(oauthTransportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "smsbower.page" || r.URL.Query().Get("api_key") != "private-sms-key" {
			t.Error("SMS request credentials or endpoint changed")
		}
		switch r.URL.Query().Get("action") {
		case "getNumber":
			if r.URL.Query().Get("country") != "6" || r.URL.Query().Get("maxPrice") != "0.123456789123456789" {
				t.Error("SMS country or exact price cap changed")
			}
			return oauthResponse("ACCESS_NUMBER:order-1:15555550101"), nil
		case "getStatus":
			return oauthResponse("STATUS_OK:654321"), nil
		case "setStatus":
			status := r.URL.Query().Get("status")
			if status == "1" {
				return oauthResponse("ACCESS_READY"), nil
			}
			finished <- status
			if finishReply != "" {
				return oauthResponse(finishReply), nil
			}
			if status == "6" {
				return oauthResponse("ACCESS_ACTIVATION"), nil
			}
			if status == "8" {
				return oauthResponse("ACCESS_CANCEL"), nil
			}
		}
		t.Error("unexpected SMS operation")
		return oauthResponse("BAD_ACTION"), nil
	}))
	return finished
}

func awaitSMSFinish(t *testing.T, finished <-chan string) string {
	t.Helper()
	select {
	case status := <-finished:
		return status
	case <-time.After(8 * time.Second):
		t.Fatal("SMS order was not finalized")
		return ""
	}
}

func TestOAuthSMSConfirmedFlowBindsNumberUsesCodeAndCompletesOrder(t *testing.T) {
	f, browser := newAssistFixture(t, "phone", "sms_code")
	finished := installSMSFixture(t, f, "")
	startAssisted(t, f, smsLoginInput())
	if action := awaitAssistAction(t, browser); action.Stage != "phone" || action.Value != "+15555550101" {
		t.Fatal("acquired phone was not submitted")
	}
	if action := awaitAssistAction(t, browser); action.Stage != "sms_code" || action.Value != "654321" {
		t.Fatal("SMS verification code was not submitted")
	}
	terminal := f.awaitPhase(t, "authorized")
	if status := awaitSMSFinish(t, finished); status != "6" {
		t.Fatalf("successful order not completed: %s", status)
	}
	raw, _ := json.Marshal(terminal)
	for _, secret := range []string{"private-sms-key", "654321", "15555550101"} {
		if strings.Contains(string(raw), secret) {
			t.Fatal("SMS credential appeared in task")
		}
	}
}

func TestOAuthSMSCancellationReleasesAcquiredNumber(t *testing.T) {
	f, browser := newAssistFixture(t, "phone", "sms_code")
	ticks := make(chan time.Time)
	f.service.UseOAuthAssistTicks(ticks)
	finished := installSMSFixture(t, f, "")
	startAssisted(t, f, smsLoginInput())
	ticks <- time.Now()
	awaitAssistAction(t, browser)
	if err := f.service.CancelOAuth("owner", f.view.ID); err != nil {
		t.Fatal(err)
	}
	f.awaitPhase(t, "cancelled")
	if status := awaitSMSFinish(t, finished); status != "8" {
		t.Fatalf("cancelled order not released: %s", status)
	}
}

func TestOAuthSMSUnconfirmedChargeNeverCallsProvider(t *testing.T) {
	f, _ := newAssistFixture(t, "phone")
	var calls atomic.Int32
	f.service.UseProviderTransport(oauthTransportFunc(func(*http.Request) (*http.Response, error) { calls.Add(1); return oauthResponse(""), nil }))
	login := smsLoginInput()
	login.SMS.Confirmed = false
	if _, err := f.service.StartOAuthWithInput(context.Background(), "owner", accountworkbench.OAuthStartInput{Login: &login}); err == nil {
		t.Fatal("unconfirmed phone binding accepted")
	}
	if calls.Load() != 0 {
		t.Fatal("provider contacted before confirmation")
	}
}

func TestOAuthSMSFailedOrderCompletionPersistsReviewWithoutLosingAuthorization(t *testing.T) {
	f, browser := newAssistFixture(t, "phone", "sms_code")
	finished := installSMSFixture(t, f, "BAD_STATUS")
	startAssisted(t, f, smsLoginInput())
	awaitAssistAction(t, browser)
	awaitAssistAction(t, browser)
	f.awaitPhase(t, "authorized")
	awaitSMSFinish(t, finished)
	review := awaitAssistMessage(t, f, "短信订单结束状态未确认")
	if review.Result["sms_order_status"] != "review" || review.Status != "succeeded" {
		t.Fatal("order completion review missing or authorization result discarded")
	}
	view, err := f.service.ReadOAuth(context.Background(), "owner", f.view.ID)
	if err != nil || view.Status != "authorized" {
		t.Fatal("successful authorization lost after order review")
	}
}

func TestOAuthSMSClosingAuthorizedSessionStillCompletesUsedOrder(t *testing.T) {
	f, browser := newAssistFixture(t, "phone", "sms_code")
	release := make(chan struct{})
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})
	browser.closeRelease = release
	finished := installSMSFixture(t, f, "")
	startAssisted(t, f, smsLoginInput())
	awaitAssistAction(t, browser)
	awaitAssistAction(t, browser)
	f.awaitPhase(t, "authorized")
	if err := f.service.CancelOAuth("owner", f.view.ID); err != nil {
		t.Fatal(err)
	}
	close(release)
	if status := awaitSMSFinish(t, finished); status != "6" {
		t.Fatalf("closing authorized session cancelled used SMS order: %s", status)
	}
}
