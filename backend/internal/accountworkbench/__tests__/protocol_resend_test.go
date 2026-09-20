package accountworkbench_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"runtime"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
)

func waitProtocolPrompt(t *testing.T, service *accountworkbench.Service, owner, id, previous string) accountworkbench.Run {
	t.Helper()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	for {
		run, err := service.Run(context.Background(), owner, id)
		if err != nil {
			t.Fatal(err)
		}
		if prompt := run.Items[0].LoginPrompt; prompt != nil && prompt.ID != previous {
			return run
		}
		if run.Status != "running" && run.Status != "queued" {
			t.Fatalf("authorization ended before input: %s", run.Status)
		}
		select {
		case <-deadline.C:
			t.Fatal("protocol input prompt missing")
		default:
			runtime.Gosched()
		}
	}
}

func TestManualEmailResendUsesSameSessionAndDoesNotReplayUncertainRequest(t *testing.T) {
	for _, lost := range []bool{false, true} {
		name := "success"
		if lost {
			name = "lost response"
		}
		t.Run(name, func(t *testing.T) {
			service, store := fixture(t, `{}`)
			owner, _ := previewOwner(t, store)
			runner, _ := runTasks(t, service)
			protocol := &protocolFixture{t: t, emailCode: true}
			sends := 0
			signedRunInputWithProtocol(t, service, func(req *http.Request) (*http.Response, error) {
				if req.URL.Path == "/api/accounts/email-otp/resend" {
					sends++
					if req.Method != "POST" || req.URL.Host != "auth.openai.com" {
						t.Error("wrong resend endpoint")
					}
					if lost {
						return nil, errors.New("response lost")
					}
					return upstreamResponse(map[string]any{}), nil
				}
				return protocol.response(req)
			}, func(*http.Request) error { return nil })
			ctx := context.Background()
			preview, err := service.Preview(ctx, owner, accountworkbench.PreviewInput{Action: "export", Content: "run@example.test----test-password"})
			if err != nil {
				t.Fatal(err)
			}
			run, err := service.Start(ctx, owner, accountworkbench.RunConfirmation{ID: preview.ID, Revision: preview.Revision})
			if err != nil {
				t.Fatal(err)
			}
			run = waitProtocolPrompt(t, service, owner, run.ID, "")
			oldPrompt := run.Items[0].LoginPrompt.ID
			var input accountworkbench.LoginInput
			if err := json.Unmarshal([]byte(`{"action":"resend_email","value":""}`), &input); err != nil {
				t.Fatal(err)
			}
			input.PromptID = oldPrompt
			if err := service.SubmitLoginInput(ctx, owner, run.ID, run.Items[0].ID, input); err != nil {
				t.Fatalf("manual resend rejected: %v", err)
			}
			if !lost {
				run = waitProtocolPrompt(t, service, owner, run.ID, oldPrompt)
				if err := service.SubmitLoginInput(ctx, owner, run.ID, run.Items[0].ID, input); err == nil {
					t.Fatal("old prompt permitted duplicate resend")
				}
				if err := service.SubmitLoginInput(ctx, owner, run.ID, run.Items[0].ID, accountworkbench.LoginInput{PromptID: run.Items[0].LoginPrompt.ID, Value: "234567"}); err != nil {
					t.Fatal(err)
				}
			}
			awaitRun(t, runner)
			run, err = service.Run(ctx, owner, run.ID)
			if err != nil {
				t.Fatal(err)
			}
			want := "completed"
			if lost {
				want = "needs_attention"
			}
			if run.Status != want || sends != 1 || protocol.bootstraps != 1 {
				t.Fatalf("status=%s sends=%d bootstraps=%d", run.Status, sends, protocol.bootstraps)
			}
		})
	}
}
