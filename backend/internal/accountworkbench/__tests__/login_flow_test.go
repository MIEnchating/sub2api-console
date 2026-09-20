package accountworkbench_test

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/pquerna/otp/totp"
)

type protocolFixture struct {
	t                                   *testing.T
	state, challenge                    string
	emailCode, withMFA, wrongState      bool
	bootstraps, passwords, totps, codes int
	inputNeeded                         chan struct{}
	maliciousRedirect                   bool
}

func (f *protocolFixture) response(request *http.Request) (*http.Response, error) {
	f.t.Helper()
	response := func(value any) *http.Response { return upstreamResponse(value) }
	redirect := func(target string) *http.Response {
		return &http.Response{StatusCode: 302, Header: http.Header{"Location": {target}}, Body: io.NopCloser(strings.NewReader(""))}
	}
	var payload map[string]any
	if request.Body != nil && strings.Contains(request.Header.Get("Content-Type"), "application/json") {
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			return nil, err
		}
	}
	switch request.URL.Host + request.URL.Path {
	case "chatgpt.com/":
		f.bootstraps++
		result := response(map[string]any{})
		result.Header.Add("Set-Cookie", "oai-did=test-device; Secure; Path=/")
		return result, nil
	case "chatgpt.com/api/auth/providers":
		return response(map[string]any{"openai": map[string]any{}}), nil
	case "chatgpt.com/api/auth/csrf":
		result := response(map[string]any{"csrfToken": "test-csrf"})
		result.Header.Add("Set-Cookie", "__Host-next-auth.csrf-token=test-csrf; Secure; Path=/")
		return result, nil
	case "chatgpt.com/api/auth/signin/openai":
		if request.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
			f.t.Error("wrong signin content type")
		}
		if err := request.ParseForm(); err != nil {
			return nil, err
		}
		if request.PostForm.Get("csrfToken") != "test-csrf" || request.URL.Query().Get("login_hint") != "run@example.test" {
			f.t.Error("signin parameters not bound")
		}
		target := "https://auth.openai.com/log-in/password"
		if f.emailCode {
			target = "https://auth.openai.com/email-verification"
		}
		if f.maliciousRedirect {
			target = "https://untrusted.example.test/capture"
		}
		return response(map[string]any{"url": target}), nil
	case "auth.openai.com/log-in/password", "auth.openai.com/email-verification":
		if f.inputNeeded != nil {
			select {
			case f.inputNeeded <- struct{}{}:
			default:
			}
		}
		return response(map[string]any{}), nil
	case "sentinel.openai.com/backend-api/sentinel/sdk.js":
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`'https://sentinel.openai.com/sentinel/20260219f9f6/sdk.js'`))}, nil
	case "sentinel.openai.com/sentinel/20260219f9f6/sdk.js":
		sdk, err := os.ReadFile("../../protocolsdk/__tests__/testdata/official-sdk-20260219f9f6.js")
		if err != nil {
			return nil, err
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(string(sdk)))}, nil
	case "sentinel.openai.com/backend-api/sentinel/req":
		return response(map[string]any{"token": "test-sentinel-token", "proofofwork": map[string]any{"required": false}, "turnstile": map[string]any{"required": false}}), nil
	case "auth.openai.com/api/accounts/password/verify":
		f.passwords++
		f.verifySecurity(request, "password_verify")
		if payload["password"] != "test-password" {
			f.t.Error("wrong protocol password")
		}
		if f.withMFA {
			return response(map[string]any{"page": map[string]any{"type": "mfa_challenge"}, "continue_url": "https://auth.openai.com/mfa-challenge/factor-1", "oai-client-auth-session": map[string]any{"mfa_challenge_factors": []any{map[string]any{"factor_type": "totp", "id": "factor-1"}}}}), nil
		}
		return response(map[string]any{"continue_url": "https://chatgpt.com/api/auth/callback/openai"}), nil
	case "auth.openai.com/api/accounts/mfa/issue_challenge":
		if payload["id"] != "factor-1" || payload["force_fresh_challenge"] != false {
			f.t.Error("MFA factor mismatch")
		}
		return response(map[string]any{}), nil
	case "auth.openai.com/api/accounts/mfa/verify":
		f.totps++
		f.verifySecurity(request, "password_verify")
		code, _ := payload["code"].(string)
		if payload["id"] != "factor-1" || !totp.Validate(code, "JBSWY3DPEHPK3PXP") {
			f.t.Error("invalid protocol TOTP")
		}
		return response(map[string]any{"continue_url": "https://chatgpt.com/api/auth/callback/openai"}), nil
	case "auth.openai.com/api/accounts/email-otp/validate":
		f.codes++
		f.verifySecurity(request, "email_otp_validate")
		if payload["code"] != "234567" {
			f.t.Error("old or incorrect email code used")
		}
		return response(map[string]any{"continue_url": "https://chatgpt.com/api/auth/callback/openai"}), nil
	case "chatgpt.com/api/auth/callback/openai":
		return response(map[string]any{}), nil
	case "auth.openai.com/oauth/authorize":
		q := request.URL.Query()
		f.state = q.Get("state")
		f.challenge = q.Get("code_challenge")
		if f.state == "" || q.Get("code_challenge_method") != "S256" || q.Get("redirect_uri") != "http://localhost:1455/auth/callback" {
			f.t.Error("missing PKCE/state")
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`<script>"session_id\",\"us_isolatedsession\"</script>`))}, nil
	case "auth.openai.com/api/accounts/session/select":
		if payload["session_id"] != "us_isolatedsession" {
			f.t.Error("session selection mismatch")
		}
		return response(map[string]any{"continue_url": "https://auth.openai.com/workspace", "oai-client-auth-session": map[string]any{"workspaces": []any{map[string]any{"id": "personal", "kind": "personal"}, map[string]any{"id": "run-workspace", "kind": "organization"}}}}), nil
	case "auth.openai.com/api/accounts/workspace/select":
		if payload["workspace_id"] != "run-workspace" {
			f.t.Error("wrong workspace selection")
		}
		state := f.state
		if f.wrongState {
			state = "foreign-state"
		}
		return response(map[string]any{"continue_url": "https://auth.openai.com/oauth/complete?state=" + url.QueryEscape(state)}), nil
	case "auth.openai.com/oauth/complete":
		return redirect("http://localhost:1455/auth/callback?code=test-once-code&state=" + url.QueryEscape(request.URL.Query().Get("state"))), nil
	}
	return nil, errors.New("unexpected isolated protocol endpoint")
}
func (f *protocolFixture) verifySecurity(request *http.Request, flow string) {
	var header map[string]any
	if json.Unmarshal([]byte(request.Header.Get("Openai-Sentinel-Token")), &header) != nil || header["c"] != "test-sentinel-token" || header["flow"] != flow || header["id"] != "test-device" {
		f.t.Error("missing real SDK security header")
	}
}
func TestProtocolLoginUsesRecognizedDetailsAndExchangesFreshPKCEOnce(t *testing.T) {
	for _, tc := range []struct {
		name, input                     string
		mail, mfa, wrongState, redirect bool
	}{
		{name: "password and totp", input: "run@example.test----test-password----JBSWY3DPEHPK3PXP", mfa: true},
		{name: "mail uses only new code", input: "run@example.test----https://mail.example.test/inbox", mail: true},
		{name: "foreign state never exchanged", input: "run@example.test----test-password", wrongState: true},
		{name: "foreign redirect never fetched", input: "run@example.test----test-password", redirect: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service, store := fixture(t, `{}`)
			owner, _ := previewOwner(t, store)
			runner, _ := runTasks(t, service)
			protocol := &protocolFixture{t: t, emailCode: tc.mail, withMFA: tc.mfa, wrongState: tc.wrongState, maliciousRedirect: tc.redirect}
			exchanges := 0
			signedRunInputWithProtocol(t, service, protocol.response, func(request *http.Request) error {
				exchanges++
				if err := request.ParseForm(); err != nil {
					return err
				}
				challenge := sha256.Sum256([]byte(request.Form.Get("code_verifier")))
				if request.Form.Get("code") != "test-once-code" || request.Form.Get("grant_type") != "authorization_code" || protocol.challenge != base64.RawURLEncoding.EncodeToString(challenge[:]) {
					return errors.New("PKCE binding mismatch")
				}
				return nil
			})
			reads := 0
			service.UseMailTransport(transportFunc(func(request *http.Request) (*http.Response, error) {
				if request.URL.String() != "https://mail.example.test/inbox" {
					return nil, errors.New("wrong mailbox")
				}
				reads++
				items := []any{map[string]any{"id": "old", "otp": "123456"}}
				if reads > 1 {
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
			if tc.wrongState || tc.redirect {
				if exchanges != 0 || result.Status == "completed" {
					t.Fatal("invalid authorization accepted")
				}
				return
			}
			if result.Status != "completed" || exchanges != 1 {
				t.Fatalf("protocol authorization failed: %+v", result.Items)
			}
			if tc.mfa && (protocol.passwords != 1 || protocol.totps != 1) {
				t.Fatal("password or TOTP replayed")
			}
			if tc.mail && protocol.codes != 1 {
				t.Fatal("email code replayed")
			}
			raw, _ := json.Marshal(result)
			for _, secret := range []string{"test-password", "JBSWY3DPEHPK3PXP", "234567", "rt_run-private", "test-sentinel-token"} {
				if strings.Contains(string(raw), secret) {
					t.Fatal("login secret exposed")
				}
			}
		})
	}
}

func TestProtocolManualCodeIsBoundToOwnerItemAndOnePrompt(t *testing.T) {
	service, store := fixture(t, `{}`)
	owner, _ := previewOwner(t, store)
	runner, _ := runTasks(t, service)
	protocol := &protocolFixture{t: t, emailCode: true, inputNeeded: make(chan struct{}, 1)}
	signedRunInputWithProtocol(t, service, protocol.response, func(*http.Request) error { return nil })
	ctx := context.Background()
	preview, err := service.Preview(ctx, owner, accountworkbench.PreviewInput{Action: "export", Content: "run@example.test"})
	if err != nil {
		t.Fatal(err)
	}
	run, err := service.Start(ctx, owner, accountworkbench.RunConfirmation{ID: preview.ID, Revision: preview.Revision})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-protocol.inputNeeded:
	case <-time.After(5 * time.Second):
		t.Fatal("protocol did not reach OTP")
	}
	// Query until the durable prompt is available; no artificial sleeps are used.
	deadline := time.After(5 * time.Second)
	var prompt *accountworkbench.LoginPrompt
	for prompt == nil {
		select {
		case <-deadline:
			t.Fatal("OTP prompt did not appear")
		default:
		}
		run, err = service.Run(ctx, owner, run.ID)
		if err != nil {
			t.Fatal(err)
		}
		prompt = run.Items[0].LoginPrompt
	}
	input := accountworkbench.LoginInput{PromptID: prompt.ID, Value: "234567"}
	if err := service.SubmitLoginInput(ctx, "foreign-owner", run.ID, run.Items[0].ID, input); err == nil {
		t.Fatal("foreign owner accepted")
	}
	if err := service.SubmitLoginInput(ctx, owner, run.ID, "wrong-item", input); err == nil {
		t.Fatal("foreign item accepted")
	}
	if err := service.SubmitLoginInput(ctx, owner, run.ID, run.Items[0].ID, accountworkbench.LoginInput{PromptID: prompt.ID, Value: "bad"}); err == nil {
		t.Fatal("invalid OTP accepted")
	}
	if err := service.SubmitLoginInput(ctx, owner, run.ID, run.Items[0].ID, input); err != nil {
		t.Fatal(err)
	}
	if err := service.SubmitLoginInput(ctx, owner, run.ID, run.Items[0].ID, input); err == nil {
		t.Fatal("OTP replay accepted")
	}
	awaitRun(t, runner)
	run, err = service.Run(ctx, owner, run.ID)
	if err != nil || run.Status != "completed" || run.Items[0].LoginPrompt != nil {
		t.Fatal("manual protocol OTP did not complete")
	}
}
