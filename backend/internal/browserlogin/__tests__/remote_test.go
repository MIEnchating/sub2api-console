package browserlogin_test

import (
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func TestRemoteWorkerKeepsCredentialsOffScreenshotsAndCleansOnDelete(t *testing.T) {
	socket := filepath.Join(t.TempDir(), "browser.sock")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f := &factoryFixture{browser: &browserFixture{closed: make(chan struct{}), access: "private-session-token"}, open: make(chan struct{}), proceed: make(chan struct{})}
	close(f.proceed)
	done := make(chan error, 1)
	go func() { done <- browserlogin.RunWorker(ctx, socket, f) }()
	deadline := time.After(5 * time.Second)
	for {
		_, err := os.Stat(socket)
		if err == nil {
			break
		}
		select {
		case <-deadline:
			t.Fatal("worker socket did not start")
		default:
			runtime.Gosched()
		}
	}
	remote := browserlogin.NewRemote(socket)
	browser, err := remote.Open(ctx, configstore.AuthRecord{Host: "login.example", BaseURL: "https://login.example"})
	if err != nil {
		t.Fatal(err)
	}
	image, err := browser.Screenshot(ctx)
	if err != nil || string(image) != "frame" {
		t.Fatalf("frame: %v", err)
	}
	if err = browser.Input(ctx, browserlogin.Input{Kind: "key", Key: "Tab"}); err != nil {
		t.Fatal(err)
	}
	record, err := browser.Credentials(ctx)
	if err != nil || record.AccessToken == nil || *record.AccessToken != f.browser.access {
		t.Fatal("private credential transfer failed")
	}
	client := &http.Client{Transport: &http.Transport{DialContext: func(c context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(c, "unix", socket)
	}}}
	response, err := client.Get("http://worker/sessions/wrong-id")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode == 200 {
		t.Fatal("unknown session accepted")
	}
	browser.Close()
	await(t, f.browser.closed)
	cancel()
	select {
	case err = <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("worker shutdown blocked")
	}
}
