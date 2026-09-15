package accountworkbench_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

type checkpointPrivate struct {
	*configstore.Store
	failStatus string
	onSaved    func(configstore.WorkbenchOAuthCheckpoint)
}

func (s *checkpointPrivate) SaveWorkbenchOAuthCheckpoint(ctx context.Context, record configstore.WorkbenchOAuthCheckpoint) (configstore.WorkbenchOAuthCheckpoint, error) {
	if record.Status == s.failStatus {
		return configstore.WorkbenchOAuthCheckpoint{}, errors.New("isolated checkpoint persistence failure")
	}
	saved, err := s.Store.SaveWorkbenchOAuthCheckpoint(ctx, record)
	if err == nil && s.onSaved != nil {
		s.onSaved(saved)
	}
	return saved, err
}

type checkpointBrowser struct {
	mu      sync.Mutex
	factory *checkpointFactory
	options browserlogin.OAuthOptions
	closed  bool
	manual  bool
}

func (b *checkpointBrowser) Screenshot(context.Context) ([]byte, error) {
	return []byte("private-test-frame"), nil
}
func (b *checkpointBrowser) Input(context.Context, browserlogin.Input) error {
	b.mu.Lock()
	b.manual = true
	b.mu.Unlock()
	return nil
}
func (b *checkpointBrowser) AuthorizationCode(context.Context) (browserlogin.OAuthResult, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.manual || b.closed {
		return browserlogin.OAuthResult{}, browserlogin.ErrOAuthPending
	}
	return browserlogin.OAuthResult{Code: "private-checkpoint-code", State: b.options.State}, nil
}
func (b *checkpointBrowser) Close() { b.mu.Lock(); b.closed = true; b.mu.Unlock() }
func (b *checkpointBrowser) SuspendOAuth(context.Context) (browserlogin.OAuthCheckpoint, error) {
	b.factory.suspends.Add(1)
	if b.factory.beforeSuspend != nil {
		b.factory.beforeSuspend(b.options)
	}
	if b.factory.suspendErr != nil {
		return browserlogin.OAuthCheckpoint{}, b.factory.suspendErr
	}
	b.Close()
	return browserlogin.OAuthCheckpoint{ID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Owner: b.options.Recovery.Owner, Lease: b.options.Recovery.Lease, Stage: "email_code", ExpiresAt: b.options.Recovery.ExpiresAt, Revision: 1}, nil
}

type checkpointFactory struct {
	suspends      atomic.Int32
	restores      atomic.Int32
	deletes       atomic.Int32
	suspendErr    error
	restoreErr    error
	beforeSuspend func(browserlogin.OAuthOptions)
	beforeRestore func(browserlogin.OAuthRestoreOptions)
	mu            sync.Mutex
	original      browserlogin.OAuthOptions
}

func (f *checkpointFactory) OpenOAuth(_ context.Context, options browserlogin.OAuthOptions) (browserlogin.OAuthBrowser, error) {
	if err := options.Validate(); err != nil {
		return nil, err
	}
	f.mu.Lock()
	f.original = options
	f.mu.Unlock()
	return &checkpointBrowser{factory: f, options: options}, nil
}
func (f *checkpointFactory) RestoreOAuth(_ context.Context, options browserlogin.OAuthRestoreOptions) (browserlogin.OAuthBrowser, error) {
	f.restores.Add(1)
	if f.beforeRestore != nil {
		f.beforeRestore(options)
	}
	if f.restoreErr != nil {
		return nil, f.restoreErr
	}
	return &checkpointBrowser{factory: f, options: options.Options}, nil
}
func (f *checkpointFactory) DeleteOAuthCheckpoint(context.Context, browserlogin.OAuthCheckpointRef) error {
	f.deletes.Add(1)
	return nil
}

type checkpointFixture struct {
	*importFixture
	state     *checkpointPrivate
	factory   *checkpointFactory
	events    chan taskstore.Task
	completed chan string
	exchanges atomic.Int32
	active    []string
}

func newCheckpointFixture(t *testing.T) *checkpointFixture {
	t.Helper()
	f := &checkpointFixture{importFixture: newImportFixture(t, nil), factory: &checkpointFactory{}, events: make(chan taskstore.Task, 50), completed: make(chan string, 10)}
	f.state = &checkpointPrivate{Store: f.private}
	f.configureService()
	t.Cleanup(func() {
		for _, id := range f.active {
			_ = f.service.CancelOAuth("checkpoint-owner", id)
		}
	})
	return f
}

func (f *checkpointFixture) configureService() {
	f.service = accountworkbench.New(f.state, &oauthTestTasks{Store: f.tasks.Store, events: f.events}, f.business, nil, &batchRunner{group: f.runner, done: f.completed})
	f.service.UseOAuthBrowser(f.factory)
	f.service.UseOAuthAssistTicks(make(chan time.Time))
	f.service.UseOAuthTransport(oauthTransportFunc(func(*http.Request) (*http.Response, error) {
		f.exchanges.Add(1)
		jwt := "e30." + base64.RawURLEncoding.EncodeToString([]byte(`{"email":"owner@example.com","https://api.openai.com/auth":{"chatgpt_user_id":"user-1","chatgpt_account_id":"workspace-1"}}`)) + ".private-test-signature"
		raw, _ := json.Marshal(map[string]any{"access_token": jwt, "refresh_token": "rt_checkpoint_private", "token_type": "Bearer", "expires_in": 3600})
		return oauthResponse(string(raw)), nil
	}))
}

func (f *checkpointFixture) phase(t *testing.T, phase string) taskstore.Task {
	t.Helper()
	for {
		select {
		case task := <-f.events:
			if task.Result["phase"] == phase {
				return task
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("checkpoint task did not reach %s", phase)
			return taskstore.Task{}
		}
	}
}

func (f *checkpointFixture) start(t *testing.T) accountworkbench.OAuthView {
	t.Helper()
	view, err := f.service.StartOAuthWithInput(context.Background(), "checkpoint-owner", accountworkbench.OAuthStartInput{ProxyURL: "http://operator-{session}:private-proxy@proxy.example:8080", Login: &accountworkbench.OAuthLoginInput{Email: "owner@example.com", Password: "private-login-password", WorkspaceID: "workspace-1"}})
	if err != nil {
		t.Fatal(err)
	}
	f.active = append(f.active, view.ID)
	f.phase(t, "waiting")
	return view
}

func (f *checkpointFixture) save(t *testing.T, id string) accountworkbench.OAuthCheckpointView {
	t.Helper()
	view, err := f.service.SaveOAuthCheckpoint(context.Background(), "checkpoint-owner", id, true)
	if err != nil || !view.CanRestore || view.Revision != 2 {
		t.Fatalf("saved checkpoint = %+v, %v", view, err)
	}
	f.phase(t, "checkpointed")
	select {
	case completed := <-f.completed:
		if completed != id {
			t.Fatal("wrong authorization completed")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("suspended authorization browser did not release")
	}
	return view
}
