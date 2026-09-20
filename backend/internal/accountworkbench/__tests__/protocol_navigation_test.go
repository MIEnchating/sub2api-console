package accountworkbench_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
)

func TestProtocolNavigationNegotiatesHTMLBeforeVerifyingLogin(t *testing.T) {
	for _, tc := range []struct {
		name           string
		emailCode, mfa bool
	}{
		{name: "password"},
		{name: "email code", emailCode: true},
		{name: "password followed by 2FA", mfa: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service, store := fixture(t, `{}`)
			owner, _ := previewOwner(t, store)
			runner, _ := runTasks(t, service)
			protocol := &protocolFixture{t: t, emailCode: tc.emailCode, withMFA: tc.mfa}
			exchanges := 0
			signedRunInputWithProtocol(t, service, func(req *http.Request) (*http.Response, error) {
				if req.URL.Path == "/api/auth/signin/openai" {
					return upstreamResponse(map[string]any{"url": "https://auth.openai.com/authorize?login_hint=run%40example.test"}), nil
				}
				if req.URL.Host == "auth.openai.com" && req.URL.Path == "/authorize" {
					// The authorization endpoint negotiates page navigation versus JSON.
					if !strings.HasPrefix(req.Header.Get("Accept"), "text/html,") {
						return upstreamResponse(map[string]any{"page": map[string]any{"type": "login_password"}}), nil
					}
					path := "/log-in/password"
					if tc.emailCode {
						path = "/email-verification"
					}
					return &http.Response{StatusCode: http.StatusFound, Header: http.Header{"Location": {path}}, Body: io.NopCloser(strings.NewReader(""))}, nil
				}
				if req.URL.Path == "/api/accounts/password/verify" || req.URL.Path == "/api/accounts/email-otp/validate" {
					if req.Header.Get("Accept") != "application/json" {
						t.Error("factor submission did not request JSON")
					}
				}
				return protocol.response(req)
			}, func(*http.Request) error { exchanges++; return nil })
			reads := 0
			service.UseMailTransport(transportFunc(func(*http.Request) (*http.Response, error) {
				reads++
				messages := []any{}
				if reads > 1 {
					messages = append(messages, map[string]any{"id": "new", "otp": "234567"})
				}
				return upstreamResponse(map[string]any{"messages": messages}), nil
			}))
			input := "run@example.test----test-password"
			if tc.emailCode {
				input = "run@example.test----https://mail.example.test/inbox"
			}
			if tc.mfa {
				input += "----JBSWY3DPEHPK3PXP"
			}
			ctx := context.Background()
			preview, err := service.Preview(ctx, owner, accountworkbench.PreviewInput{Action: "export", Content: input})
			if err != nil {
				t.Fatal(err)
			}
			run, err := service.Start(ctx, owner, accountworkbench.RunConfirmation{ID: preview.ID, Revision: preview.Revision})
			if err != nil {
				t.Fatal(err)
			}
			awaitRun(t, runner)
			run, err = service.Run(ctx, owner, run.ID)
			if err != nil {
				t.Fatal(err)
			}
			if run.Status != "completed" || exchanges != 1 {
				t.Fatalf("negotiated login did not complete: status=%s exchanges=%d message=%s", run.Status, exchanges, run.Items[0].Message)
			}
			if tc.mfa && (protocol.passwords != 1 || protocol.totps != 1) {
				t.Fatal("password and 2FA must each be verified exactly once before authorization completes")
			}
		})
	}
}

func TestProtocolExistingSessionHomeWithQueryCompletesOnlyWithSessionCookie(t *testing.T) {
	for _, sessionCookie := range []bool{true, false} {
		name := "session cookie present"
		if !sessionCookie {
			name = "session cookie missing"
		}
		t.Run(name, func(t *testing.T) {
			service, store := fixture(t, `{}`)
			owner, _ := previewOwner(t, store)
			runner, _ := runTasks(t, service)
			protocol := &protocolFixture{t: t}
			exchanges := 0
			signedRunInputWithProtocol(t, service, func(req *http.Request) (*http.Response, error) {
				if req.URL.Path == "/api/auth/signin/openai" {
					return upstreamResponse(map[string]any{"url": "https://chatgpt.com/?auth=complete"}), nil
				}
				if req.URL.Host == "chatgpt.com" && req.URL.RawQuery == "auth=complete" {
					response := upstreamResponse(map[string]any{})
					if sessionCookie {
						response.Header.Add("Set-Cookie", "__Secure-next-auth.session-token=private-cookie; Secure; Path=/")
					}
					return response, nil
				}
				return protocol.response(req)
			}, func(*http.Request) error { exchanges++; return nil })
			ctx := context.Background()
			preview, err := service.Preview(ctx, owner, accountworkbench.PreviewInput{Action: "export", Content: "run@example.test----test-password"})
			if err != nil {
				t.Fatal(err)
			}
			run, err := service.Start(ctx, owner, accountworkbench.RunConfirmation{ID: preview.ID, Revision: preview.Revision})
			if err != nil {
				t.Fatal(err)
			}
			awaitRun(t, runner)
			run, err = service.Run(ctx, owner, run.ID)
			if err != nil {
				t.Fatal(err)
			}
			if sessionCookie {
				if run.Status != "completed" || exchanges != 1 {
					t.Fatalf("existing session was rejected: %s", run.Items[0].Message)
				}
			} else if run.Items[0].Status != "failed" || exchanges != 0 {
				t.Fatal("home page without a session cookie was accepted")
			}
		})
	}
}

func TestProtocolUnexpectedPageReportsSafeReasonWithoutReplaying(t *testing.T) {
	for _, tc := range []struct {
		name, path, body, reason string
		status                   int
		header                   http.Header
	}{
		{name: "login entry", path: "/log-in", status: 200, body: `<html>private-page-body</html>`, reason: "账号登录入口"},
		{name: "authorization entry", path: "/authorize", status: 200, body: `{}`, reason: "授权入口"},
		{name: "challenge with success status", path: "/authorize", status: 200, header: http.Header{"Cf-Mitigated": {"challenge"}}, body: `<html>private-page-body</html>`, reason: "安全验证"},
		{name: "forbidden HTML", path: "/authorize", status: 403, header: http.Header{"Content-Type": {"text/html"}}, body: `<html>private-page-body</html>`, reason: "安全验证"},
		{name: "unrecognized private path", path: "/private-path-value", status: 200, body: `<html>private-page-body</html>`, reason: "未识别的登录页面"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service, store := fixture(t, `{}`)
			owner, _ := previewOwner(t, store)
			runner, _ := runTasks(t, service)
			protocol := &protocolFixture{t: t}
			visits := 0
			signedRunInputWithProtocol(t, service, func(req *http.Request) (*http.Response, error) {
				if req.URL.Path == "/api/auth/signin/openai" {
					return upstreamResponse(map[string]any{"url": "https://auth.openai.com" + tc.path + "?state=private-state&login_hint=private-email"}), nil
				}
				if req.URL.Host == "auth.openai.com" && req.URL.Path == tc.path {
					visits++
					return &http.Response{StatusCode: tc.status, Header: tc.header, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
				}
				return protocol.response(req)
			}, func(*http.Request) error {
				t.Error("unexpected page exchanged code")
				return errors.New("unexpected exchange")
			})
			ctx := context.Background()
			preview, err := service.Preview(ctx, owner, accountworkbench.PreviewInput{Action: "export", Content: "run@example.test----test-password"})
			if err != nil {
				t.Fatal(err)
			}
			run, err := service.Start(ctx, owner, accountworkbench.RunConfirmation{ID: preview.ID, Revision: preview.Revision})
			if err != nil {
				t.Fatal(err)
			}
			awaitRun(t, runner)
			run, err = service.Run(ctx, owner, run.ID)
			if err != nil {
				t.Fatal(err)
			}
			if run.Items[0].Status != "failed" || !strings.Contains(run.Items[0].Message, tc.reason) || visits != 1 {
				t.Fatalf("unexpected page reason lost or replayed: visits=%d message=%s", visits, run.Items[0].Message)
			}
			if strings.Contains(run.Items[0].Message, "private-") {
				t.Fatal("page error exposed private path, query, or body")
			}
		})
	}
}
