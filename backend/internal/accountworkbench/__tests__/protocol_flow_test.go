package accountworkbench_test

import (
	"context"
	"errors"
	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"net/http"
	"strings"
	"testing"
)

func TestProtocolLoginWithoutBrowserStartsAtOfficialHTTPBootstrap(t *testing.T) {
	service, store := fixture(t, `{}`)
	owner, _ := previewOwner(t, store)
	runner, _ := runTasks(t, service)
	requests := 0
	service.UseOfficialTransport(transportFunc(func(request *http.Request) (*http.Response, error) {
		requests++
		if request.URL.String() != "https://chatgpt.com/" {
			t.Errorf("unexpected protocol bootstrap: %s", request.URL.Host)
		}
		return nil, errors.New("isolated connection unavailable")
	}))
	preview, err := service.Preview(context.Background(), owner, accountworkbench.PreviewInput{Action: "export", Content: "run@example.test----password"})
	if err != nil {
		t.Fatal(err)
	}
	run, err := service.Start(context.Background(), owner, accountworkbench.RunConfirmation{ID: preview.ID, Revision: preview.Revision})
	if err != nil {
		t.Fatal(err)
	}
	awaitRun(t, runner)
	result, err := service.Run(context.Background(), owner, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if requests != 1 || result.Items[0].Status != "failed" {
		t.Fatalf("HTTP login did not run without a browser: requests=%d status=%s", requests, result.Items[0].Status)
	}
}

func TestProtocolAuthorizationCodeLostResponseIsNotReplayedOnRetry(t *testing.T) {
	service, store := fixture(t, `{}`)
	owner, _ := previewOwner(t, store)
	runner, _ := runTasks(t, service)
	protocol := &protocolFixture{t: t}
	exchanges := 0
	signedRunInputWithProtocol(t, service, protocol.response, func(*http.Request) error { exchanges++; return errors.New("response lost with private-token") })
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
	if run.Items[0].Status != "interrupted" || exchanges != 1 {
		t.Fatal("uncertain code exchange not retained")
	}
	if _, err = service.Retry(ctx, owner, accountworkbench.RunConfirmation{ID: run.ID, Revision: run.Revision}); err == nil {
		awaitRun(t, runner)
	}
	if exchanges != 1 || protocol.bootstraps != 1 {
		t.Fatal("uncertain authorization replayed")
	}
}

func TestProtocolUpstreamErrorsDoNotExposePrivateResponseFields(t *testing.T) {
	service, store := fixture(t, `{}`)
	owner, _ := previewOwner(t, store)
	runner, _ := runTasks(t, service)
	protocol := &protocolFixture{t: t}
	signedRunInputWithProtocol(t, service, func(req *http.Request) (*http.Response, error) {
		if req.URL.Path == "/api/accounts/password/verify" {
			response := upstreamResponse(map[string]any{"error": "test-password secret-cookie"})
			response.StatusCode = 401
			return response, nil
		}
		return protocol.response(req)
	}, func(*http.Request) error { t.Fatal("rejected login exchanged code"); return nil })
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
	if run.Items[0].Status != "failed" || strings.Contains(run.Items[0].Message, "test-password") || strings.Contains(run.Items[0].Message, "secret-cookie") {
		t.Fatal("upstream error exposed secrets")
	}
}
