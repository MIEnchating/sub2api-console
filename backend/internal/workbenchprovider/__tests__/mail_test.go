package workbenchprovider_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/workbenchprovider"
)

func mailResponse(r *http.Request, status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}
}

func TestMailboxUntimedCandidatesRequireBaselineAndExcludePreviouslyPresentCodes(t *testing.T) {
	body := `{"messages":[{"id":"old","otp":"123456"}]}`
	client := workbenchprovider.NewHTTP(transportFunc(func(r *http.Request) (*http.Response, error) { return mailResponse(r, 200, body), nil }))
	mailbox, err := workbenchprovider.NewMailbox(workbenchprovider.MailConfig{Kind: "http", URL: "https://mail.example/inbox"}, client, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(mailbox.Close)
	withoutBaseline, err := mailbox.Fetch(context.Background(), time.Now(), workbenchprovider.MailBaseline{})
	if err != nil || len(withoutBaseline) != 0 {
		t.Fatal("untimed code accepted without baseline")
	}
	baseline, err := mailbox.Baseline(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	requestedAt := time.Now()
	body = `{"messages":[{"id":"old-changed-id","otp":"123456"},{"id":"new","otp":"234567"}]}`
	candidates, err := mailbox.Fetch(context.Background(), requestedAt, baseline)
	if err != nil || len(candidates) != 1 || candidates[0].Code != "234567" {
		t.Fatalf("baseline-filtered candidates = %#v, %v", candidates, err)
	}
}

func TestMailboxBaselineFromAnotherMailboxCannotAuthorizeUntimedCandidate(t *testing.T) {
	client := workbenchprovider.NewHTTP(transportFunc(func(r *http.Request) (*http.Response, error) { return mailResponse(r, 200, `{"otp":"123456"}`), nil }))
	first, err := workbenchprovider.NewMailbox(workbenchprovider.MailConfig{Kind: "http", URL: "https://mail.example/first"}, client, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(first.Close)
	second, err := workbenchprovider.NewMailbox(workbenchprovider.MailConfig{Kind: "http", URL: "https://mail.example/second"}, client, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(second.Close)
	baseline, err := first.Baseline(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	candidates, err := second.Fetch(context.Background(), time.Now(), baseline)
	if err != nil || len(candidates) != 0 {
		t.Fatal("foreign mailbox baseline accepted")
	}
}

func TestMailboxSameSecondNewMessageIsAcceptedButBaselineCodeStaysExcluded(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	stamp := now.Format(time.RFC3339)
	body := `{"messages":[{"id":"old","received_at":"` + stamp + `","otp":"123456"}]}`
	client := workbenchprovider.NewHTTP(transportFunc(func(r *http.Request) (*http.Response, error) { return mailResponse(r, 200, body), nil }))
	mailbox, err := workbenchprovider.NewMailbox(workbenchprovider.MailConfig{Kind: "http", URL: "https://mail.example/inbox"}, client, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(mailbox.Close)
	baseline, err := mailbox.Baseline(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	requestedAt := time.Now()
	stamp = requestedAt.Truncate(time.Second).Format(time.RFC3339)
	body = `{"messages":[{"id":"old","received_at":"` + stamp + `","otp":"123456"},{"id":"new","received_at":"` + stamp + `","otp":"234567"}]}`
	candidates, err := mailbox.Fetch(context.Background(), requestedAt, baseline)
	if err != nil || len(candidates) != 1 || candidates[0].Code != "234567" {
		t.Fatalf("same-second candidates = %#v, %v", candidates, err)
	}
}

func TestMailboxWithoutBaselineRejectsTimedCodeBeforeRequest(t *testing.T) {
	requestedAt := time.Now().UTC()
	old := requestedAt.Add(-time.Second).Format(time.RFC3339Nano)
	fresh := requestedAt.Format(time.RFC3339Nano)
	client := workbenchprovider.NewHTTP(transportFunc(func(r *http.Request) (*http.Response, error) {
		return mailResponse(r, 200, `[{"received_at":"`+old+`","otp":"123456"},{"received_at":"`+fresh+`","otp":"234567"}]`), nil
	}))
	mailbox, err := workbenchprovider.NewMailbox(workbenchprovider.MailConfig{Kind: "http", URL: "https://mail.example/inbox"}, client, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(mailbox.Close)
	candidates, err := mailbox.Fetch(context.Background(), requestedAt, workbenchprovider.MailBaseline{})
	if err != nil || len(candidates) != 1 || candidates[0].Code != "234567" {
		t.Fatalf("request time filter = %#v, %v", candidates, err)
	}
}

func TestMailboxPOSTKeepsConfiguredCredentialsInsideTransportAndPrivateConfig(t *testing.T) {
	config := workbenchprovider.MailConfig{Kind: "http", URL: "https://mail.example/inbox?key=private-url-key", Method: "POST", Headers: map[string]string{"Authorization": "Bearer private-token"}, Body: `{"password":"private-password"}`}
	client := workbenchprovider.NewHTTP(transportFunc(func(r *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(r.Body)
		if r.Method != "POST" || r.Header.Get("Authorization") != "Bearer private-token" || string(body) != config.Body {
			t.Fatal("mail request changed its configured credentials")
		}
		return mailResponse(r, 200, `{"otp":"123456"}`), nil
	}))
	mailbox, err := workbenchprovider.NewMailbox(config, client, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mailbox.Baseline(context.Background()); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(config)
	if err != nil || string(encoded) != `{}` {
		t.Fatal("private mail configuration serialized")
	}
	mailbox.Close()
	if _, err := mailbox.Baseline(context.Background()); err == nil {
		t.Fatal("closed mailbox fetched more credentials")
	}
}
