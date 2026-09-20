package accountworkbench_test

import (
	"context"
	"net/http"
	"runtime"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/pquerna/otp/totp"
)

func TestRejectedLoginFactorWaitsForNewInputInSameProtocolSession(t *testing.T) {
	for _, tc := range []struct {
		name, path, code, kind string
		mail, mfa              bool
	}{
		{name: "password", path: "/api/accounts/password/verify", code: "invalid_password", kind: "password"},
		{name: "email code", path: "/api/accounts/email-otp/validate", code: "wrong_email_otp_code", kind: "email_code", mail: true},
		{name: "TOTP", path: "/api/accounts/mfa/verify", code: "invalid_otp", kind: "totp_code", mfa: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service, store := fixture(t, `{}`)
			owner, _ := previewOwner(t, store)
			runner, _ := runTasks(t, service)
			protocol := &protocolFixture{t: t, emailCode: tc.mail, withMFA: tc.mfa}
			attempts := 0
			signedRunInputWithProtocol(t, service, func(req *http.Request) (*http.Response, error) {
				if req.URL.Path == tc.path {
					attempts++
					if attempts == 1 {
						reply := upstreamResponse(map[string]any{"error": map[string]any{"code": tc.code, "message": "private-server-detail"}})
						reply.StatusCode = 400
						return reply, nil
					}
				}
				return protocol.response(req)
			}, func(*http.Request) error { return nil })
			reads := 0
			service.UseMailTransport(transportFunc(func(*http.Request) (*http.Response, error) {
				reads++
				items := []any{}
				if reads > 1 {
					items = append(items, map[string]any{"id": "new", "otp": "234567"})
				}
				return upstreamResponse(map[string]any{"messages": items}), nil
			}))
			input := "run@example.test----test-password----JBSWY3DPEHPK3PXP"
			if tc.mail {
				input = "run@example.test----https://mail.example.test/inbox"
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
			deadline := time.NewTimer(5 * time.Second)
			defer deadline.Stop()
			for {
				run, err = service.Run(ctx, owner, run.ID)
				if err != nil {
					t.Fatal(err)
				}
				if run.Items[0].LoginPrompt != nil {
					break
				}
				select {
				case <-runner.finished:
					t.Fatalf("rejected %s ended authorization instead of asking for input", tc.name)
				case <-deadline.C:
					t.Fatal("new input prompt missing")
				default:
					runtime.Gosched()
				}
			}
			prompt := run.Items[0].LoginPrompt
			if prompt.Kind != tc.kind {
				t.Fatalf("wrong input requested: %s", prompt.Kind)
			}
			if !tc.mail {
				if err := service.SubmitLoginInput(ctx, owner, run.ID, run.Items[0].ID, accountworkbench.LoginInput{PromptID: prompt.ID, Action: "resend_email"}); err == nil {
					t.Fatal("non-email step accepted email resend")
				}
			}
			replacement := "test-password"
			if tc.mail {
				replacement = "234567"
			}
			if tc.mfa {
				replacement, err = totp.GenerateCode("JBSWY3DPEHPK3PXP", time.Now().UTC())
				if err != nil {
					t.Fatal(err)
				}
			}
			if err = service.SubmitLoginInput(ctx, owner, run.ID, run.Items[0].ID, accountworkbench.LoginInput{PromptID: prompt.ID, Value: replacement}); err != nil {
				t.Fatal(err)
			}
			awaitRun(t, runner)
			run, err = service.Run(ctx, owner, run.ID)
			if err != nil {
				t.Fatal(err)
			}
			if run.Status != "completed" || attempts != 2 || protocol.bootstraps != 1 {
				t.Fatalf("factor correction did not reuse login session: status=%s attempts=%d logins=%d", run.Status, attempts, protocol.bootstraps)
			}
		})
	}
}
