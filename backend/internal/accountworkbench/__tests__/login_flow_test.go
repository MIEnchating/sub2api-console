package accountworkbench_test

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin"
	"github.com/pquerna/otp/totp"
)

type loginBrowser struct {
	options    browserlogin.OAuthOptions
	stages     []string
	actions    []browserlogin.AuthAction
	closed     bool
	wrongState bool
	opens      int
}

func (b *loginBrowser) OpenOAuth(_ context.Context, options browserlogin.OAuthOptions) (browserlogin.OAuthBrowser, error) {
	if err := options.Validate(); err != nil {
		return nil, err
	}
	b.options = options
	b.actions = nil
	b.opens++
	return b, nil
}
func (b *loginBrowser) Screenshot(context.Context) ([]byte, error)      { return nil, nil }
func (b *loginBrowser) Input(context.Context, browserlogin.Input) error { return nil }
func (b *loginBrowser) Close()                                          { b.closed = true }
func (b *loginBrowser) AuthorizationCode(context.Context) (browserlogin.OAuthResult, error) {
	if len(b.actions) < len(b.stages) {
		return browserlogin.OAuthResult{}, browserlogin.ErrOAuthPending
	}
	state := b.options.State
	if b.wrongState {
		state = "another-transaction"
	}
	return browserlogin.OAuthResult{Code: "test-once-code", State: state}, nil
}
func (b *loginBrowser) InspectAuth(context.Context) (browserlogin.AuthPage, error) {
	return browserlogin.AuthPage{Stage: b.stages[len(b.actions)], Revision: strings.Repeat("a", 64)}, nil
}
func (b *loginBrowser) ApplyAuth(_ context.Context, action browserlogin.AuthAction) error {
	if err := action.Validate(); err != nil {
		return err
	}
	b.actions = append(b.actions, action)
	return nil
}

func TestLoginAssistanceUsesRecognizedDetailsAndExchangesFreshPKCEOnce(t *testing.T) {
	for _, tc := range []struct {
		name, input string
		stages      []string
		wrongState  bool
	}{
		{name: "password and totp", input: "run@example.test----test-password----JBSWY3DPEHPK3PXP", stages: []string{"email", "password", "totp_code"}},
		{name: "mail uses only new code", input: "run@example.test----https://mail.example.test/inbox", stages: []string{"email", "email_code"}},
		{name: "foreign state never exchanged", input: "run@example.test", wrongState: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service, store := fixture(t, `{}`)
			owner, _ := previewOwner(t, store)
			runner, tasks := runTasks(t, service)
			browser := &loginBrowser{stages: tc.stages, wrongState: tc.wrongState}
			service.UseExecution(tasks, runner, nil, browser, t.TempDir())
			exchanges := 0
			signedRunInput(t, service, func(request *http.Request) error {
				exchanges++
				if err := request.ParseForm(); err != nil {
					return err
				}
				u, _ := url.Parse(browser.options.AuthorizationURL)
				challenge := sha256.Sum256([]byte(request.Form.Get("code_verifier")))
				if request.Form.Get("code") != "test-once-code" || request.Form.Get("grant_type") != "authorization_code" || u.Query().Get("code_challenge") != base64.RawURLEncoding.EncodeToString(challenge[:]) {
					return errors.New("OAuth binding mismatch")
				}
				return nil
			})
			mailReads := 0
			service.UseMailTransport(transportFunc(func(request *http.Request) (*http.Response, error) {
				if request.URL.String() != "https://mail.example.test/inbox" {
					return nil, errors.New("unexpected mailbox endpoint")
				}
				mailReads++
				items := []any{map[string]any{"id": "old", "otp": "123456"}}
				if mailReads > 1 {
					items = append(items, map[string]any{"id": "new", "otp": "234567"})
				}
				return upstreamResponse(map[string]any{"messages": items}), nil
			}))
			preview, err := service.Preview(context.Background(), owner, accountworkbench.PreviewInput{Action: "export", Content: tc.input})
			if err != nil {
				t.Fatal(err)
			}
			queued, err := service.Start(context.Background(), owner, accountworkbench.RunConfirmation{ID: preview.ID, Revision: preview.Revision})
			if err != nil {
				t.Fatal(err)
			}
			awaitRun(t, runner)
			result, err := service.Run(context.Background(), owner, queued.ID)
			if err != nil {
				t.Fatal(err)
			}
			if !browser.closed {
				t.Fatal("official browser left open")
			}
			if tc.wrongState {
				if exchanges != 0 || result.Status == "completed" {
					t.Fatal("foreign callback accepted")
				}
				return
			}
			if result.Status != "completed" || exchanges != 1 {
				t.Fatalf("authorization failed: %+v", result.Items)
			}
			for _, action := range browser.actions {
				switch action.Stage {
				case "email":
					if action.Value != "run@example.test" {
						t.Fatal("wrong email")
					}
				case "password":
					if action.Value != "test-password" {
						t.Fatal("wrong password")
					}
				case "totp_code":
					if !totp.Validate(action.Value, "JBSWY3DPEHPK3PXP") {
						t.Fatal("invalid generated totp")
					}
				case "email_code":
					if action.Value != "234567" {
						t.Fatal("baseline code reused")
					}
				}
			}
			raw, _ := json.Marshal(result)
			for _, secret := range []string{"test-password", "JBSWY3DPEHPK3PXP", "234567", "rt_run-private"} {
				if strings.Contains(string(raw), secret) {
					t.Fatal("login secret exposed in result")
				}
			}
		})
	}
}
