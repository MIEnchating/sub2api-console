package accountworkbench_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskrunner"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

const securitySecret = "JBSWY3DPEHPK3PXP"
const securityPassword = "SecurePrivate!923"

type securityBrowser struct {
	mu                                    sync.Mutex
	identity                              browserlogin.SecurityIdentity
	enabled                               bool
	enrollment                            browserlogin.SecurityEnrollment
	page                                  browserlogin.AuthPage
	passwordResult                        bool
	activateError                         error
	submitError                           error
	enrolls, activations, begins, submits int
	onActivate                            func(browserlogin.SecurityEnrollment, string) error
	onSubmit                              func(browserlogin.AuthAction) error
	closed                                chan struct{}
	closeOnce                             sync.Once
	closeRelease                          <-chan struct{}
}

func (b *securityBrowser) OpenSecurity(_ context.Context, options browserlogin.SecurityOptions) (browserlogin.SecurityBrowser, error) {
	if options.Validate() != nil || options.Email != "owner@example.com" {
		return nil, errors.New("unexpected security login identity")
	}
	return b, nil
}
func (b *securityBrowser) Screenshot(context.Context) ([]byte, error) {
	return []byte("security-frame"), nil
}
func (b *securityBrowser) Input(context.Context, browserlogin.Input) error { return nil }
func (b *securityBrowser) InspectAuth(context.Context) (browserlogin.AuthPage, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.page, nil
}
func (b *securityBrowser) ApplyAuth(_ context.Context, action browserlogin.AuthAction) error {
	return action.Validate()
}
func (b *securityBrowser) Identity(context.Context) (browserlogin.SecurityIdentity, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.identity, nil
}
func (b *securityBrowser) BeginPassword(_ context.Context, identity browserlogin.SecurityIdentity) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if identity != b.identity {
		return browserlogin.ErrSecurityIdentity
	}
	b.begins++
	return nil
}
func (b *securityBrowser) SubmitPassword(_ context.Context, action browserlogin.AuthAction) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.submits++
	if b.onSubmit != nil {
		if err := b.onSubmit(action); err != nil {
			return err
		}
	}
	return b.submitError
}
func (b *securityBrowser) PasswordResult(context.Context) (bool, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.passwordResult, nil
}
func (b *securityBrowser) TOTPEnabled(_ context.Context, identity browserlogin.SecurityIdentity) (bool, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if identity != b.identity {
		return false, browserlogin.ErrSecurityIdentity
	}
	return b.enabled, nil
}
func (b *securityBrowser) EnrollTOTP(_ context.Context, identity browserlogin.SecurityIdentity) (browserlogin.SecurityEnrollment, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if identity != b.identity {
		return browserlogin.SecurityEnrollment{}, browserlogin.ErrSecurityIdentity
	}
	b.enrolls++
	return b.enrollment, nil
}
func (b *securityBrowser) ActivateTOTP(_ context.Context, identity browserlogin.SecurityIdentity, enrollment browserlogin.SecurityEnrollment, code string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if identity != b.identity {
		return browserlogin.ErrSecurityIdentity
	}
	b.activations++
	if b.onActivate != nil {
		if err := b.onActivate(enrollment, code); err != nil {
			return err
		}
	}
	if b.activateError != nil {
		return b.activateError
	}
	b.enabled = true
	return nil
}
func (b *securityBrowser) Close() {
	b.closeOnce.Do(func() {
		close(b.closed)
		if b.closeRelease != nil {
			<-b.closeRelease
		}
	})
}

type securityFixture struct {
	*importFixture
	browser   *securityBrowser
	events    chan taskstore.Task
	directory string
	view      accountworkbench.SecurityView
}

func newSecurityFixture(t *testing.T) *securityFixture {
	return securityFixtureWithRunner(t, nil)
}

func securityFixtureWithRunner(t *testing.T, wrap func(*taskrunner.Group) taskrunner.Runner) *securityFixture {
	return securityFixtureWithConfiguration(t, wrap, false)
}

type noSecurityManagement struct{ *configstore.Store }

func (s *noSecurityManagement) TargetSettings(context.Context) (configstore.TargetSettings, error) {
	return configstore.TargetSettings{}, errors.New("local security must not read management settings")
}

func securityFixtureWithConfiguration(t *testing.T, wrap func(*taskrunner.Group) taskrunner.Runner, noManagement bool) *securityFixture {
	t.Helper()
	f := &securityFixture{importFixture: newImportFixture(t, nil), events: make(chan taskstore.Task, 64), directory: filepath.Join(t.TempDir(), "security")}
	f.remote.accounts["101"] = exportAccount("101", "Source")
	f.browser = &securityBrowser{identity: browserlogin.SecurityIdentity{Email: "owner@example.com", UserID: "user-101"}, enrollment: browserlogin.SecurityEnrollment{Secret: securitySecret, SessionID: "private-session-id"}, page: browserlogin.AuthPage{Stage: "new_password", Revision: strings.Repeat("a", 64)}, closed: make(chan struct{})}
	var runner taskrunner.Runner = f.runner
	if wrap != nil {
		runner = wrap(f.runner)
	}
	f.service = accountworkbench.New(f.private, &oauthTestTasks{Store: f.tasks.Store, events: f.events}, f.business, nil, runner)
	if noManagement {
		f.service = accountworkbench.New(&noSecurityManagement{Store: f.private}, &oauthTestTasks{Store: f.tasks.Store, events: f.events}, f.business, nil, runner)
	}
	f.service.UseSecurityBrowser(f.browser)
	if err := f.service.UseSecurityDirectory(f.directory); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.service.CloseSecurity() })
	return f
}

func (f *securityFixture) awaitSecurity(t *testing.T, phase string) taskstore.Task {
	t.Helper()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	for {
		select {
		case task := <-f.events:
			if task.Result["phase"] == phase {
				return task
			}
		case <-deadline.C:
			t.Fatalf("security task did not reach %s", phase)
			return taskstore.Task{}
		}
	}
}

func (f *securityFixture) startSecurity(t *testing.T, operation string) {
	t.Helper()
	input := accountworkbench.SecurityStartInput{AccountID: "101", Operation: operation, Confirmed: true}
	if operation == "password" {
		input.Password = securityPassword
	}
	view, err := f.service.StartSecurity(context.Background(), "security-owner", input)
	if err != nil {
		t.Fatal(err)
	}
	f.view = view
	f.awaitSecurity(t, "waiting")
}

func (f *securityFixture) continueSecurity(t *testing.T, phase string) taskstore.Task {
	t.Helper()
	if err := f.service.ContinueSecurity(context.Background(), "security-owner", f.view.ID); err != nil {
		t.Fatal(err)
	}
	return f.awaitSecurity(t, phase)
}
