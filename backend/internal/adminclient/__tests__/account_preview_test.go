package adminclient_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/adminclient"
)

func previewInput() adminclient.AccountPreviewRequest {
	return adminclient.AccountPreviewRequest{ModelID: "gpt-6-astra", Prompt: "Draw an SVG", ReasoningEffort: "low", RequestID: "preview-request", TimeoutSeconds: 5}
}

func TestAccountPreviewRejectsFailedMalformedAndMismatchedResultsWithoutReplay(t *testing.T) {
	for name, body := range map[string]string{
		"business failure":      `{"code":1,"message":"failed"}`,
		"different account":     `{"code":0,"data":{"account_id":2,"request_id":"preview-request","text":"<svg/>"}}`,
		"different request":     `{"code":0,"data":{"account_id":1,"request_id":"other","text":"<svg/>"}}`,
		"empty result":          `{"code":0,"data":{"account_id":1,"request_id":"preview-request","text":""}}`,
		"invalid JSON":          `{"code":0`,
		"echoed management key": `{"code":0,"data":{"account_id":1,"request_id":"preview-request","text":"private-management-secret"}}`,
	} {
		t.Run(name, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; _, _ = io.WriteString(w, body) }))
			defer server.Close()
			client, err := adminclient.New(adminclient.Config{BaseURL: server.URL, AdminKey: "private-management-secret", Attempts: 3}, nil)
			if err != nil {
				t.Fatal(err)
			}
			result, err := client.GenerateAccountPreview(context.Background(), "1", previewInput())
			if err == nil || result != nil || calls != 1 {
				t.Fatalf("failed preview accepted or replayed: calls=%d error=%v", calls, err)
			}
			if strings.Contains(err.Error(), "private-management-secret") {
				t.Fatal("preview error leaked management key")
			}
		})
	}
}

func TestAccountPreviewRefusesRedirectBeforeSendingManagementKey(t *testing.T) {
	called := false
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true }))
	defer target.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer server.Close()
	client, err := adminclient.New(adminclient.Config{BaseURL: server.URL, AdminKey: "private-management-secret"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.GenerateAccountPreview(context.Background(), "1", previewInput()); err == nil || called {
		t.Fatal("preview followed redirect or accepted redirect response")
	}
}

func TestAccountPreviewCancellationStopsRequestWithoutReplay(t *testing.T) {
	started := make(chan struct{})
	stopped := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		close(started)
		<-r.Context().Done()
		close(stopped)
	}))
	defer server.Close()
	client, err := adminclient.New(adminclient.Config{BaseURL: server.URL, AdminKey: "private-management-secret"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() { _, err := client.GenerateAccountPreview(ctx, "1", previewInput()); result <- err }()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("preview never started")
	}
	cancel()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation not preserved: %v", err)
	}
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("upstream preview was not cancelled")
	}
}
