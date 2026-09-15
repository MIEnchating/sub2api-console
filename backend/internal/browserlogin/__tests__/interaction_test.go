package browserlogin_test

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskrunner"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

type delayedFrameBrowser struct {
	block   atomic.Bool
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (b *delayedFrameBrowser) Screenshot(ctx context.Context) ([]byte, error) {
	if b.block.Load() {
		b.once.Do(func() { close(b.started) })
		select {
		case <-b.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return []byte("frame"), nil
}
func (*delayedFrameBrowser) Input(ctx context.Context, _ browserlogin.Input) error {
	return ctx.Err()
}
func (*delayedFrameBrowser) Credentials(context.Context) (configstore.AuthRecord, error) {
	return configstore.AuthRecord{}, errors.New("fixture has no credentials")
}
func (*delayedFrameBrowser) Close() {}

type interactionFactory struct{ browser browserlogin.Browser }

func (f interactionFactory) Open(context.Context, configstore.AuthRecord) (browserlogin.Browser, error) {
	return f.browser, nil
}

func interactionManager(t *testing.T, browser browserlogin.Browser) (*browserlogin.Manager, string) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	socket := filepath.Join(t.TempDir(), "worker.sock")
	workerDone := make(chan error, 1)
	go func() { workerDone <- browserlogin.RunWorker(ctx, socket, interactionFactory{browser: browser}) }()
	store, err := taskstore.Open(filepath.Join(t.TempDir(), "tasks.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	runner := taskrunner.New(ctx)
	t.Cleanup(func() {
		cancel()
		shutdown, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		_ = runner.Shutdown(shutdown)
		store.Close()
		select {
		case <-workerDone:
		case <-shutdown.Done():
			t.Error("isolated worker did not stop")
		}
	})
	waitSocket(t, socket)
	manager := browserlogin.New(browserlogin.NewRemote(socket), store, runner)
	view, err := manager.Start(ctx, "owner", configstore.AuthRecord{Host: "login.example", BaseURL: "https://login.example"}, func(context.Context, configstore.AuthRecord) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	waitView(t, manager, view.ID, "waiting")
	return manager, view.ID
}

func TestSlowFrameDoesNotBlockKeyboardInputAcrossManagerAndWorker(t *testing.T) {
	browser := &delayedFrameBrowser{started: make(chan struct{}), release: make(chan struct{})}
	manager, id := interactionManager(t, browser)
	t.Cleanup(func() { close(browser.release) })
	browser.block.Store(true)
	frameDone := make(chan error, 1)
	go func() { _, err := manager.Read(context.Background(), "owner", id); frameDone <- err }()
	await(t, browser.started)
	inputDone := make(chan error, 1)
	go func() {
		inputDone <- manager.Input(context.Background(), "owner", id, browserlogin.Input{Kind: "text", Text: "a"})
	}()
	select {
	case err := <-inputDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("keyboard input is waiting for an unrelated screenshot")
	}
}

func TestCancelledSessionDoesNotReturnAnInFlightFrame(t *testing.T) {
	browser := &delayedFrameBrowser{started: make(chan struct{}), release: make(chan struct{})}
	manager, id := interactionManager(t, browser)
	browser.block.Store(true)
	frameDone := make(chan browserlogin.View, 1)
	go func() { view, _ := manager.Read(context.Background(), "owner", id); frameDone <- view }()
	await(t, browser.started)
	if err := manager.Cancel("owner", id); err != nil {
		t.Fatal(err)
	}
	close(browser.release)
	select {
	case view := <-frameDone:
		if view.Image != "" || view.Status == "waiting" {
			t.Fatal("cancelled session returned an active screenshot")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cancelled frame did not finish")
	}
}
