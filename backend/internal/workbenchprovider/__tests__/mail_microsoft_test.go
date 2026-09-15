package workbenchprovider_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/workbenchprovider"
)

const testMailClientID = "11111111-2222-3333-4444-555555555555"

func microsoftConfig() workbenchprovider.MailConfig {
	return workbenchprovider.MailConfig{Kind: "microsoft", Email: "mailbox@example.com", ClientID: testMailClientID, RefreshToken: "microsoft-rt-private"}
}

func TestMicrosoftMailboxUsesOfficialEndpointsAndCachesRotatedToken(t *testing.T) {
	requests, rotations, reads := 0, 0, 0
	body := `{"value":[]}`
	client := workbenchprovider.NewHTTP(transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "login.microsoftonline.com" {
			requests++
			raw, _ := io.ReadAll(r.Body)
			form, _ := url.ParseQuery(string(raw))
			if r.URL.Path != "/consumers/oauth2/v2.0/token" || r.Method != "POST" || form.Get("refresh_token") != "microsoft-rt-private" || form.Get("client_id") != testMailClientID || form.Get("scope") != "https://graph.microsoft.com/Mail.Read offline_access" || r.Header.Get("Authorization") != "" {
				t.Fatal("Microsoft refresh request escaped the fixed OAuth contract")
			}
			return mailResponse(r, 200, `{"access_token":"microsoft-access-private","refresh_token":"microsoft-rt-rotated-private","expires_in":3600,"token_type":"Bearer","scope":"Mail.Read offline_access"}`), nil
		}
		if r.URL.Host != "graph.microsoft.com" || r.URL.Path != "/v1.0/me/mailFolders/inbox/messages" || r.Method != "GET" || r.Header.Get("Authorization") != "Bearer microsoft-access-private" || r.URL.Query().Get("$top") != "10" {
			t.Fatal("Microsoft mail request escaped its official endpoint")
		}
		reads++
		return mailResponse(r, 200, body), nil
	}))
	mailbox, err := workbenchprovider.NewMailbox(microsoftConfig(), client, func(_ context.Context, rt string) error {
		rotations++
		if rt != "microsoft-rt-rotated-private" {
			t.Fatal("rotated token was not provided to private storage")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(mailbox.Close)
	baseline, err := mailbox.Baseline(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	requestedAt := time.Now()
	body = `{"value":[{"id":"new-message","subject":"ChatGPT verification","receivedDateTime":"` + requestedAt.UTC().Format(time.RFC3339Nano) + `","body":{"content":"<p>Your OpenAI code is <b>123456</b></p>"}}]}`
	candidates, err := mailbox.Fetch(context.Background(), requestedAt, baseline)
	if err != nil || len(candidates) != 1 || candidates[0].Code != "123456" || requests != 1 || rotations != 1 || reads != 2 {
		t.Fatalf("Microsoft candidate flow = %#v, %v, refresh=%d rotations=%d reads=%d", candidates, err, requests, rotations, reads)
	}
}

func TestMicrosoftMailboxFailedPrivateRotationSaveRetriesSaveWithoutReplayingExchange(t *testing.T) {
	requests, reads, rotations := 0, 0, 0
	client := workbenchprovider.NewHTTP(transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "login.microsoftonline.com" {
			requests++
			return mailResponse(r, 200, `{"access_token":"private-access","refresh_token":"private-new-rt","expires_in":3600}`), nil
		}
		reads++
		return mailResponse(r, 200, `{"value":[]}`), nil
	}))
	mailbox, err := workbenchprovider.NewMailbox(microsoftConfig(), client, func(_ context.Context, rt string) error {
		rotations++
		if rt != "private-new-rt" {
			t.Fatal("rotation retried old refresh token")
		}
		if rotations == 1 {
			return errors.New("private-new-rt secret disk detail")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(mailbox.Close)
	_, err = mailbox.Baseline(context.Background())
	var providerError *workbenchprovider.Error
	if !errors.As(err, &providerError) || providerError.Code != "mail_microsoft_rotation_save_failed" || strings.Contains(err.Error(), "private-new-rt") || reads != 0 {
		t.Fatalf("rotation-save failure = %v", err)
	}
	if _, err := mailbox.Baseline(context.Background()); err != nil {
		t.Fatal(err)
	}
	if requests != 1 || rotations != 2 || reads != 1 {
		t.Fatalf("rotation replayed exchange: refresh=%d rotation=%d reads=%d", requests, rotations, reads)
	}
}

func TestMicrosoftMailboxUnauthorizedGraphReadRefreshesOnlyOnNextRequestWithRotatedToken(t *testing.T) {
	refreshes, reads := 0, 0
	client := workbenchprovider.NewHTTP(transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "login.microsoftonline.com" {
			refreshes++
			raw, _ := io.ReadAll(r.Body)
			form, _ := url.ParseQuery(string(raw))
			if refreshes == 2 && form.Get("refresh_token") != "private-rotated-rt" {
				t.Fatal("second refresh used a stale token")
			}
			return mailResponse(r, 200, `{"access_token":"private-access","refresh_token":"private-rotated-rt","expires_in":3600}`), nil
		}
		reads++
		if reads == 1 {
			return mailResponse(r, 401, `{"error":{"message":"private-access"}}`), nil
		}
		return mailResponse(r, 200, `{"value":[]}`), nil
	}))
	mailbox, err := workbenchprovider.NewMailbox(microsoftConfig(), client, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(mailbox.Close)
	if _, err := mailbox.Baseline(context.Background()); err == nil || refreshes != 1 || reads != 1 {
		t.Fatal("unauthorized read automatically replayed token exchange")
	}
	if _, err := mailbox.Baseline(context.Background()); err != nil {
		t.Fatal(err)
	}
	if refreshes != 2 || reads != 2 {
		t.Fatal("next poll did not refresh using the rotated token")
	}
}

func TestMicrosoftMailboxReportsMissingMailReadConsentWithoutLeakingProviderResponse(t *testing.T) {
	for _, scenario := range []string{"scope", "http-forbidden", "business-forbidden"} {
		t.Run(scenario, func(t *testing.T) {
			client := workbenchprovider.NewHTTP(transportFunc(func(r *http.Request) (*http.Response, error) {
				if r.URL.Host == "login.microsoftonline.com" {
					if scenario == "scope" {
						return mailResponse(r, 200, `{"access_token":"private-access","scope":"User.Read"}`), nil
					}
					return mailResponse(r, 200, `{"access_token":"private-access"}`), nil
				}
				if scenario == "http-forbidden" {
					return mailResponse(r, 403, `{"error":{"message":"private-access"}}`), nil
				}
				return mailResponse(r, 200, `{"error":{"code":"ErrorAccessDenied","message":"private-access"}}`), nil
			}))
			mailbox, err := workbenchprovider.NewMailbox(microsoftConfig(), client, nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(mailbox.Close)
			_, err = mailbox.Baseline(context.Background())
			var providerError *workbenchprovider.Error
			if !errors.As(err, &providerError) || providerError.Code != "mail_microsoft_consent_required" || strings.Contains(err.Error(), "private-access") {
				t.Fatalf("missing consent = %v", err)
			}
		})
	}
}

func TestMicrosoftMailboxPreservesRotatedRefreshTokenEvenWhenMailReadScopeIsMissing(t *testing.T) {
	rotated := ""
	client := workbenchprovider.NewHTTP(transportFunc(func(r *http.Request) (*http.Response, error) {
		return mailResponse(r, 200, `{"access_token":"private-access","refresh_token":"private-new-rt","scope":"User.Read"}`), nil
	}))
	mailbox, err := workbenchprovider.NewMailbox(microsoftConfig(), client, func(_ context.Context, token string) error {
		rotated = token
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(mailbox.Close)
	_, err = mailbox.Baseline(context.Background())
	var providerError *workbenchprovider.Error
	if !errors.As(err, &providerError) || providerError.Code != "mail_microsoft_consent_required" || rotated != "private-new-rt" {
		t.Fatalf("scope rejection discarded the rotated token: saved=%t, err=%v", rotated != "", err)
	}
}

func TestMicrosoftMailboxRejectsTokenBusinessFailureBeforeFetchingMessages(t *testing.T) {
	for _, body := range []string{`{"error":"invalid_grant","error_description":"microsoft-rt-private"}`, `{"access_token":"","refresh_token":"private"}`, `{"access_token":"private","expires_in":-1}`} {
		requests := 0
		client := workbenchprovider.NewHTTP(transportFunc(func(r *http.Request) (*http.Response, error) { requests++; return mailResponse(r, 200, body), nil }))
		mailbox, err := workbenchprovider.NewMailbox(microsoftConfig(), client, nil)
		if err != nil {
			t.Fatal(err)
		}
		_, err = mailbox.Baseline(context.Background())
		mailbox.Close()
		if err == nil || requests != 1 || strings.Contains(err.Error(), "private") {
			t.Fatalf("invalid token response = %v, requests=%d", err, requests)
		}
	}
}

func TestMicrosoftMailboxConcurrentReadsShareOneTokenRefresh(t *testing.T) {
	refreshes := 0
	client := workbenchprovider.NewHTTP(transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "login.microsoftonline.com" {
			refreshes++
			return mailResponse(r, 200, `{"access_token":"private-access","expires_in":3600}`), nil
		}
		return mailResponse(r, 200, `{"value":[]}`), nil
	}))
	mailbox, err := workbenchprovider.NewMailbox(microsoftConfig(), client, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(mailbox.Close)
	var workers sync.WaitGroup
	failures := make(chan error, 8)
	for range 8 {
		workers.Go(func() { _, err := mailbox.Baseline(context.Background()); failures <- err })
	}
	workers.Wait()
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
	if refreshes != 1 {
		t.Fatalf("concurrent token rotations = %d", refreshes)
	}
}

func TestMicrosoftCredentialLinePreservesPasswordAndKeepsCredentialsPrivate(t *testing.T) {
	line := "owner@example.com---- password with spaces ----" + testMailClientID + "----private-refresh-token"
	credential, err := workbenchprovider.ParseMicrosoftCredentialLine(line)
	if err != nil || credential.Email != "owner@example.com" || credential.Password != " password with spaces " || credential.ClientID != testMailClientID || credential.RefreshToken != "private-refresh-token" {
		t.Fatalf("four-field credential parse failed: %v", err)
	}
	encoded, err := json.Marshal(credential)
	if err != nil || string(encoded) != "{}" {
		t.Fatal("Microsoft credential serialization exposed private fields")
	}
	if _, err := workbenchprovider.ParseMicrosoftCredentialLine("owner@example.com----private----invalid-client----private-rt"); err == nil || strings.Contains(err.Error(), "private") {
		t.Fatalf("invalid credential validation = %v", err)
	}
}
