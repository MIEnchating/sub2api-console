package accountworkbench_test

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

func TestDeletingAuthorizedHistoryWhileSMSCompletionIsPendingPreventsLateRecreation(t *testing.T) {
	f, browser := newAssistFixture(t, "phone", "sms_code")
	ticks := make(chan time.Time, 3)
	f.service.UseOAuthAssistTicks(ticks)
	completionStarted := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	finish := func() { once.Do(func() { close(release) }) }
	t.Cleanup(finish)
	f.service.UseProviderTransport(oauthTransportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "smsbower.page" {
			return nil, errors.New("unexpected isolated endpoint")
		}
		switch r.URL.Query().Get("action") {
		case "getNumber":
			return oauthResponse("ACCESS_NUMBER:order-1:15555550101"), nil
		case "getStatus":
			return oauthResponse("STATUS_OK:654321"), nil
		case "setStatus":
			if r.URL.Query().Get("status") == "1" {
				return oauthResponse("ACCESS_READY"), nil
			}
			close(completionStarted)
			<-release
			return oauthResponse("BAD_STATUS"), nil
		}
		return nil, errors.New("unexpected isolated SMS operation")
	}))
	startAssisted(t, f, smsLoginInput())
	ticks <- time.Now()
	awaitAssistAction(t, browser)
	ticks <- time.Now()
	awaitAssistAction(t, browser)
	ticks <- time.Now()
	terminal := f.awaitPhase(t, "authorized")
	select {
	case <-completionStarted:
	case <-time.After(3 * time.Second):
		t.Fatal("SMS finalization did not reach its isolated boundary")
	}
	if err := f.service.DeleteHistory(context.Background(), accountworkbench.HistoryActionInput{Confirmed: true, Items: []taskstore.HistorySelection{{ID: terminal.ID, UpdatedAt: terminal.UpdatedAt}}}); err != nil {
		t.Fatal(err)
	}
	finish()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := f.runner.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := f.tasks.Get(context.Background(), terminal.ID); !errors.Is(err, taskstore.ErrNotFound) {
		t.Fatalf("SMS cleanup recreated the deleted history record: %v", err)
	}
	view, err := f.service.ReadOAuth(context.Background(), "owner", f.view.ID)
	if err != nil || view.Status != "authorized" {
		t.Fatalf("history-only deletion discarded available authorization: %+v, %v", view, err)
	}
}
