package browserlogin_test

import (
	"context"
	"errors"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskrunner"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

type browserFixture struct {
	closed chan struct{}
	once   sync.Once
	access string
}

func (b *browserFixture) Screenshot(context.Context) ([]byte, error)      { return []byte("frame"), nil }
func (b *browserFixture) Input(context.Context, browserlogin.Input) error { return nil }
func (b *browserFixture) Credentials(context.Context) (configstore.AuthRecord, error) {
	return configstore.AuthRecord{Host: "login.example", AccessToken: &b.access}, nil
}
func (b *browserFixture) Close() { b.once.Do(func() { close(b.closed) }) }

type factoryFixture struct {
	browser *browserFixture
	open    chan struct{}
	proceed chan struct{}
}

func (f *factoryFixture) Open(ctx context.Context, _ configstore.AuthRecord) (browserlogin.Browser, error) {
	close(f.open)
	select {
	case <-f.proceed:
		return f.browser, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
func setupManager(t *testing.T) (*browserlogin.Manager, *factoryFixture, *taskstore.Store, *taskrunner.Group) {
	t.Helper()
	store, err := taskstore.Open(filepath.Join(t.TempDir(), "tasks.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	runner := taskrunner.New(context.Background())
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = runner.Shutdown(ctx)
		store.Close()
	})
	f := &factoryFixture{browser: &browserFixture{closed: make(chan struct{}), access: "private-session-token"}, open: make(chan struct{}), proceed: make(chan struct{})}
	return browserlogin.New(f, store, runner), f, store, runner
}
func await(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatal("expected lifecycle event did not arrive")
	}
}
func waitView(t *testing.T, m *browserlogin.Manager, id, status string) browserlogin.View {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for {
		v, err := m.Read(ctx, "owner", id)
		if err != nil {
			t.Fatal(err)
		}
		if v.Status == status {
			return v
		}
		if ctx.Err() != nil {
			t.Fatalf("expected %s, got %s", status, v.Status)
		}
		select {
		case <-ctx.Done():
		default:
			runtime.Gosched()
		}
	}
}
func TestNewSessionRejectsOtherOwnersAndConcurrentBrowsers(t *testing.T) {
	m, f, _, _ := setupManager(t)
	v, err := m.Start(context.Background(), "owner", configstore.AuthRecord{Host: "login.example"}, func(context.Context, configstore.AuthRecord) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	await(t, f.open)
	if _, err = m.Read(context.Background(), "other", v.ID); !errors.Is(err, browserlogin.ErrSession) {
		t.Fatal("other session could read browser")
	}
	if err = m.Finish("other", v.ID); !errors.Is(err, browserlogin.ErrSession) {
		t.Fatal("other session could submit")
	}
	if err = m.Cancel("other", v.ID); !errors.Is(err, browserlogin.ErrSession) {
		t.Fatal("other session could cancel")
	}
	if _, err = m.Start(context.Background(), "owner", configstore.AuthRecord{Host: "second.example"}, func(context.Context, configstore.AuthRecord) error { return nil }); err == nil {
		t.Fatal("concurrent browser accepted")
	}
	if err = m.Cancel("owner", v.ID); err != nil {
		t.Fatal(err)
	}
	waitView(t, m, v.ID, "cancelled")
}
func TestLoginCommitsOnlyAfterExplicitCompletionAndClearsBrowser(t *testing.T) {
	m, f, store, _ := setupManager(t)
	committed := make(chan struct{}, 1)
	v, err := m.Start(context.Background(), "owner", configstore.AuthRecord{Host: "login.example"}, func(_ context.Context, r configstore.AuthRecord) error {
		if r.AccessToken == nil || *r.AccessToken != "private-session-token" {
			return errors.New("credentials missing")
		}
		committed <- struct{}{}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	await(t, f.open)
	close(f.proceed)
	view := waitView(t, m, v.ID, "waiting")
	if view.Image == "" {
		t.Fatal("waiting browser lacks image")
	}
	select {
	case <-committed:
		t.Fatal("committed without operator action")
	default:
	}
	if err = m.Finish("owner", v.ID); err != nil {
		t.Fatal(err)
	}
	await(t, committed)
	await(t, f.browser.closed)
	view = waitView(t, m, v.ID, "succeeded")
	if view.Image != "" {
		t.Fatal("retained screenshot")
	}
	task, err := store.Get(context.Background(), v.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	if task.Status != "succeeded" || task.Result["credentials_persisted"] != true {
		t.Fatalf("task result: %#v", task)
	}
	if strings.Contains(task.Message, f.browser.access) {
		t.Fatal("token leaked")
	}
}
func TestVerificationFailureDoesNotReportCredentialsSaved(t *testing.T) {
	m, f, store, _ := setupManager(t)
	v, err := m.Start(context.Background(), "owner", configstore.AuthRecord{Host: "login.example"}, func(context.Context, configstore.AuthRecord) error { return errors.New("SESSION_BINDING_MISMATCH") })
	if err != nil {
		t.Fatal(err)
	}
	await(t, f.open)
	close(f.proceed)
	waitView(t, m, v.ID, "waiting")
	if err = m.Finish("owner", v.ID); err != nil {
		t.Fatal(err)
	}
	await(t, f.browser.closed)
	view := waitView(t, m, v.ID, "failed")
	if !strings.Contains(view.Message, "SESSION_BINDING_MISMATCH") {
		t.Fatal("missing actionable reason")
	}
	task, err := store.Get(context.Background(), v.ID)
	if err != nil {
		t.Fatal(err)
	}
	if task.Result["credentials_persisted"] == true {
		t.Fatal("failed verification reported success")
	}
}
func TestCancellationClosesWaitingBrowserWithoutCommit(t *testing.T) {
	m, f, _, _ := setupManager(t)
	committed := make(chan struct{}, 1)
	v, err := m.Start(context.Background(), "owner", configstore.AuthRecord{Host: "login.example"}, func(context.Context, configstore.AuthRecord) error { committed <- struct{}{}; return nil })
	if err != nil {
		t.Fatal(err)
	}
	await(t, f.open)
	close(f.proceed)
	waitView(t, m, v.ID, "waiting")
	if err = m.Cancel("owner", v.ID); err != nil {
		t.Fatal(err)
	}
	await(t, f.browser.closed)
	select {
	case <-committed:
		t.Fatal("cancelled session committed")
	default:
	}
}
