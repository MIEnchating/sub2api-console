package workbenchprovider_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/workbenchprovider"
)

type receiptJournal func(context.Context, workbenchprovider.SMSJournalEvent) error

func (f receiptJournal) RecordSMS(ctx context.Context, event workbenchprovider.SMSJournalEvent) error {
	return f(ctx, event)
}

func TestSMSJournalSubmissionFailurePreventsPurchaseAndReplay(t *testing.T) {
	requests := 0
	client := workbenchprovider.NewHTTP(transportFunc(func(request *http.Request) (*http.Response, error) {
		requests++
		return smsResponse(request, "ACCESS_NUMBER:order-1:17005550123"), nil
	}))
	defer client.Close()
	provider, err := workbenchprovider.NewSMS(workbenchprovider.SMSConfig{Provider: "smsbower", APIKey: "isolated-key", Country: "1001"}, client)
	if err != nil {
		t.Fatal(err)
	}
	defer provider.Close()
	if err := provider.UseJournal(receiptJournal(func(context.Context, workbenchprovider.SMSJournalEvent) error {
		return errors.New("private store unavailable")
	})); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, err := provider.Acquire(context.Background(), "purchase-1"); err == nil {
			t.Fatal("purchase accepted without durable submission")
		}
	}
	if requests != 0 {
		t.Fatal("supplier called despite receipt failure")
	}
}

func TestSMSJournalPersistsPurchasedOrderAfterRequestCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := []workbenchprovider.SMSJournalEvent{}
	client := workbenchprovider.NewHTTP(transportFunc(func(request *http.Request) (*http.Response, error) {
		cancel()
		return smsResponse(request, "ACCESS_NUMBER:order-1:17005550123"), nil
	}))
	defer client.Close()
	provider, err := workbenchprovider.NewSMS(workbenchprovider.SMSConfig{Provider: "smsbower", APIKey: "isolated-key", Country: "1001"}, client)
	if err != nil {
		t.Fatal(err)
	}
	defer provider.Close()
	if err := provider.UseJournal(receiptJournal(func(ctx context.Context, event workbenchprovider.SMSJournalEvent) error {
		if err := ctx.Err(); err != nil {
			t.Fatal("receipt outcome inherited cancelled context")
		}
		events = append(events, event)
		return nil
	})); err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Acquire(ctx, "purchase-1"); err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[0].State != "submitted" || events[1].State != "confirmed" || events[1].Number.RequestID != "order-1" {
		t.Fatal("purchased order receipt not recorded")
	}
}

func TestSMSUncertainCompletionPreventsConflictingRelease(t *testing.T) {
	writes := 0
	client := workbenchprovider.NewHTTP(transportFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Query().Get("action") == "getNumber" {
			return smsResponse(request, "ACCESS_NUMBER:order-1:17005550123"), nil
		}
		writes++
		return nil, errors.New("supplier connection lost")
	}))
	defer client.Close()
	provider, err := workbenchprovider.NewSMS(workbenchprovider.SMSConfig{Provider: "smsbower", APIKey: "isolated-key", Country: "1001"}, client)
	if err != nil {
		t.Fatal(err)
	}
	defer provider.Close()
	number, err := provider.Acquire(context.Background(), "purchase-1")
	if err != nil {
		t.Fatal(err)
	}
	if err := provider.Complete(context.Background(), number.RequestID); err == nil {
		t.Fatal("uncertain completion accepted")
	}
	if err := provider.Release(context.Background(), number.RequestID); err == nil {
		t.Fatal("conflicting release accepted")
	}
	if writes != 1 {
		t.Fatal("uncertain terminal operation was followed by another supplier write")
	}
}

func TestSMSRestoredOrderOnlyPermitsPolling(t *testing.T) {
	requests := 0
	client := workbenchprovider.NewHTTP(transportFunc(func(request *http.Request) (*http.Response, error) {
		requests++
		if request.URL.Query().Get("action") != "getStatus" {
			t.Fatal("restored order mutated supplier")
		}
		return smsResponse(request, "STATUS_WAIT_CODE"), nil
	}))
	defer client.Close()
	provider, err := workbenchprovider.NewSMS(workbenchprovider.SMSConfig{Provider: "smsbower", APIKey: "isolated-key"}, client)
	if err != nil {
		t.Fatal(err)
	}
	defer provider.Close()
	if err := provider.RestoreActivation("purchase-1", workbenchprovider.SMSNumber{RequestID: "order-1", Phone: "+17005550123"}); err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Poll(context.Background(), "order-1"); err != nil {
		t.Fatal(err)
	}
	if err := provider.Release(context.Background(), "order-1"); err == nil {
		t.Fatal("restored order allowed write")
	}
	if requests != 1 {
		t.Fatal("restoration contacted supplier outside explicit poll")
	}
}
