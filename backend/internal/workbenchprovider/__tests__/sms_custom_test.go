package workbenchprovider_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/workbenchprovider"
)

func TestCustomSMSRejectsInvalidDuplicateAndPrivateEndpoints(t *testing.T) {
	for _, entries := range []string{"", "+17005550123----http://sms.example/messages", "+17005550123----https://127.0.0.1/messages", "+17005550123----https://sms.example/a\n+17005550123----https://sms.example/b", "17005550123----https://sms.example/a"} {
		if _, err := workbenchprovider.NewSMS(workbenchprovider.SMSConfig{Provider: "custom", CustomEntries: entries}, workbenchprovider.NewHTTP(transportFunc(func(*http.Request) (*http.Response, error) {
			t.Fatal("invalid config made a network request")
			return nil, errors.New("invalid test request")
		}))); err == nil {
			t.Fatalf("invalid custom entries accepted: %q", entries)
		}
	}
}

func TestCustomSMSBaselineExcludesOldCodesAndOnlyReturnsFreshMessageOnce(t *testing.T) {
	read := 0
	client := workbenchprovider.NewHTTP(transportFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.String() != "https://sms.example/inbox?key=private" {
			t.Fatal("custom SMS endpoint changed")
		}
		read++
		if read <= 2 {
			return smsResponse(request, `{"messages":[{"id":"old","code":"111111"}]}`), nil
		}
		return smsResponse(request, `{"messages":[{"id":"old","code":"111111"},{"id":"new","code":"222222"},{"id":"delayed-old","code":"999999","received_at":"2020-01-01T00:00:00Z"}]}`), nil
	}))
	provider, err := workbenchprovider.NewSMS(workbenchprovider.SMSConfig{Provider: "custom", CustomEntries: "+17005550123----https://sms.example/inbox?key=private"}, client)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	number, err := provider.Acquire(ctx, "one-number")
	if err != nil {
		t.Fatal(err)
	}
	if message, err := provider.Poll(ctx, number.RequestID); err != nil || !message.Pending || message.Code != "" {
		t.Fatal("baseline code was reused")
	}
	if message, err := provider.Poll(ctx, number.RequestID); err != nil || message.Pending || message.Code != "222222" {
		t.Fatalf("fresh custom code not delivered: %v", err)
	}
	if message, err := provider.Poll(ctx, number.RequestID); err != nil || !message.Pending {
		t.Fatal("already delivered code was replayed")
	}
	if err := provider.Release(ctx, number.RequestID); err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Acquire(ctx, "new-operation"); err == nil {
		t.Fatal("released custom phone was assigned again")
	}
}

func TestCustomSMSFailedBaselineStillConsumesNumberAndDoesNotReplayReadForSameOperation(t *testing.T) {
	reads := map[string]int{}
	client := workbenchprovider.NewHTTP(transportFunc(func(request *http.Request) (*http.Response, error) {
		reads[request.URL.Path]++
		if request.URL.Path == "/first" {
			return nil, errors.New("https://sms.example/first?private-key")
		}
		return smsResponse(request, "123456"), nil
	}))
	provider, err := workbenchprovider.NewSMS(workbenchprovider.SMSConfig{Provider: "custom", CustomEntries: "+17005550123----https://sms.example/first\n+17005550124----https://sms.example/second"}, client)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := provider.Acquire(ctx, "first-operation"); err == nil {
		t.Fatal("failed baseline accepted")
	}
	if _, err := provider.Acquire(ctx, "first-operation"); err == nil || reads["/first"] != 1 {
		t.Fatal("same failed acquisition was replayed")
	}
	number, err := provider.Acquire(ctx, "second-operation")
	if err != nil || number.Phone != "+17005550124" {
		t.Fatalf("next operation reused consumed phone: %v", err)
	}
	if waiting, err := provider.Poll(ctx, number.RequestID); err != nil || !waiting.Pending {
		t.Fatal("plain-text baseline code was reused")
	}
}

func TestSMSCloseClearsSessionWithoutImplicitProviderWrites(t *testing.T) {
	requests := 0
	client := workbenchprovider.NewHTTP(transportFunc(func(request *http.Request) (*http.Response, error) {
		requests++
		return smsResponse(request, "ACCESS_NUMBER:activation-1:17005550123"), nil
	}))
	provider, err := workbenchprovider.NewSMS(workbenchprovider.SMSConfig{Provider: "smsbower", APIKey: "isolated-key", Country: "1001"}, client)
	if err != nil {
		t.Fatal(err)
	}
	number, err := provider.Acquire(context.Background(), "purchase")
	if err != nil {
		t.Fatal(err)
	}
	provider.Close()
	if _, err := provider.Poll(context.Background(), number.RequestID); err == nil || requests != 1 {
		t.Fatal("closed session remained usable or performed an implicit supplier write")
	}
}

func TestCustomSMSSharedPoolDoesNotReuseNumbersAcrossSessionsOrReorderedLists(t *testing.T) {
	pool := workbenchprovider.NewSMSPool()
	client := workbenchprovider.NewHTTP(transportFunc(func(request *http.Request) (*http.Response, error) {
		return smsResponse(request, `[]`), nil
	}))
	first, err := workbenchprovider.NewSMS(workbenchprovider.SMSConfig{Provider: "custom", CustomEntries: "+17005550123----https://sms.example/first\n+17005550124----https://sms.example/second"}, client, pool)
	if err != nil {
		t.Fatal(err)
	}
	number, err := first.Acquire(context.Background(), "first-operation")
	if err != nil || number.Phone != "+17005550123" {
		t.Fatal("first custom number not acquired")
	}
	first.Close()
	second, err := workbenchprovider.NewSMS(workbenchprovider.SMSConfig{Provider: "custom", CustomEntries: "+17005550124----https://sms.example/second\n+17005550123----https://sms.example/first"}, client, pool)
	if err != nil {
		t.Fatal(err)
	}
	number, err = second.Acquire(context.Background(), "second-operation")
	if err != nil || number.Phone != "+17005550124" {
		t.Fatal("new session reused a number from the same reordered list")
	}
	if _, err := second.Acquire(context.Background(), "third-operation"); err == nil {
		t.Fatal("shared pool reused an already assigned number")
	}
}
