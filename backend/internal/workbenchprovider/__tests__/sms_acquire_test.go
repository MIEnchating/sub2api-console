package workbenchprovider_test

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/workbenchprovider"
)

func TestSMSConcurrentSameOperationAcquiresOnlyOneProviderOrder(t *testing.T) {
	var requests atomic.Int32
	client := workbenchprovider.NewHTTP(transportFunc(func(request *http.Request) (*http.Response, error) {
		requests.Add(1)
		return smsResponse(request, "ACCESS_NUMBER:activation-1:17005550123"), nil
	}))
	provider, err := workbenchprovider.NewSMS(workbenchprovider.SMSConfig{Provider: "smsbower", APIKey: "isolated-key", Country: "1001"}, client)
	if err != nil {
		t.Fatal(err)
	}
	var workers sync.WaitGroup
	results := make(chan workbenchprovider.SMSNumber, 2)
	for range 2 {
		workers.Go(func() {
			number, err := provider.Acquire(context.Background(), "same-operation")
			if err != nil {
				t.Error(err)
			}
			results <- number
		})
	}
	workers.Wait()
	if first, second := <-results, <-results; first != second || first.RequestID != "activation-1" || requests.Load() != 1 {
		t.Fatal("concurrent acquisition created separate provider orders")
	}
}

func TestSMSInvalidPhonePreservesKnownOrderForExplicitCancellation(t *testing.T) {
	cancelled := false
	client := workbenchprovider.NewHTTP(transportFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Query().Get("action") == "getNumber" {
			return smsResponse(request, "ACCESS_NUMBER:activation-1:123"), nil
		}
		if request.URL.Query().Get("action") == "setStatus" && request.URL.Query().Get("id") == "activation-1" && request.URL.Query().Get("status") == "8" {
			cancelled = true
			return smsResponse(request, "ACCESS_CANCEL"), nil
		}
		t.Fatal("unexpected provider mutation")
		return nil, context.Canceled
	}))
	provider, err := workbenchprovider.NewSMS(workbenchprovider.SMSConfig{Provider: "smsbower", APIKey: "isolated-key", Country: "1001"}, client)
	if err != nil {
		t.Fatal(err)
	}
	number, err := provider.Acquire(context.Background(), "one-purchase")
	if err == nil || number.RequestID != "activation-1" || number.Phone != "" || cancelled {
		t.Fatal("unusable number lost its known order or was implicitly cancelled")
	}
	if err := provider.Release(context.Background(), number.RequestID); err != nil || !cancelled {
		t.Fatalf("known order could not be cancelled: %v", err)
	}
}

func TestSMSDuplicateProviderIDCannotCancelAnotherActiveOrder(t *testing.T) {
	client := workbenchprovider.NewHTTP(transportFunc(func(request *http.Request) (*http.Response, error) {
		return smsResponse(request, "ACCESS_NUMBER:activation-1:17005550123"), nil
	}))
	provider, err := workbenchprovider.NewSMS(workbenchprovider.SMSConfig{Provider: "smsbower", APIKey: "isolated-key", Country: "1001"}, client)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Acquire(context.Background(), "first-purchase"); err != nil {
		t.Fatal(err)
	}
	if number, err := provider.Acquire(context.Background(), "second-purchase"); err == nil || number.RequestID != "" {
		t.Fatal("duplicate order ID became an independently cancellable acquisition")
	}
}
