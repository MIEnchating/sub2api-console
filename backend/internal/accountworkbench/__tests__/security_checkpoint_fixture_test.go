package accountworkbench_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin"
)

type checkpointSecurityBrowser struct {
	*securityBrowser
	stateMu         sync.Mutex
	options         browserlogin.SecurityCheckpointOptions
	identityPending bool
	confirmed       bool
	opens           int
	openErr         error
}

func (b *checkpointSecurityBrowser) OpenSecurityFromCheckpoint(_ context.Context, options browserlogin.SecurityCheckpointOptions) (browserlogin.SecurityBrowser, error) {
	b.stateMu.Lock()
	defer b.stateMu.Unlock()
	if options.Validate() != nil || b.opens != 0 {
		return nil, browserlogin.ErrOAuthCheckpoint
	}
	b.options, b.opens = options, b.opens+1
	if b.openErr != nil {
		return nil, b.openErr
	}
	return b, nil
}

func (b *checkpointSecurityBrowser) Identity(ctx context.Context) (browserlogin.SecurityIdentity, error) {
	b.stateMu.Lock()
	pending := b.identityPending
	b.stateMu.Unlock()
	if pending {
		return browserlogin.SecurityIdentity{}, browserlogin.ErrSecurityPending
	}
	return b.securityBrowser.Identity(ctx)
}

func (b *checkpointSecurityBrowser) ConfirmSecurityIdentity(ctx context.Context, expected browserlogin.SecurityIdentity) error {
	identity, err := b.Identity(ctx)
	if err != nil {
		return err
	}
	if expected != identity {
		return browserlogin.ErrSecurityIdentity
	}
	b.stateMu.Lock()
	b.confirmed = true
	b.stateMu.Unlock()
	return nil
}

func (b *checkpointSecurityBrowser) BeginPassword(ctx context.Context, identity browserlogin.SecurityIdentity) error {
	b.stateMu.Lock()
	confirmed := b.confirmed
	b.stateMu.Unlock()
	if !confirmed {
		return errors.New("password started before identity confirmation")
	}
	return b.securityBrowser.BeginPassword(ctx, identity)
}

func (b *checkpointSecurityBrowser) EnrollTOTP(ctx context.Context, identity browserlogin.SecurityIdentity) (browserlogin.SecurityEnrollment, error) {
	b.stateMu.Lock()
	confirmed := b.confirmed
	b.stateMu.Unlock()
	if !confirmed {
		return browserlogin.SecurityEnrollment{}, errors.New("TOTP enrolled before identity confirmation")
	}
	return b.securityBrowser.EnrollTOTP(ctx, identity)
}

func checkpointSecurityFixture(t *testing.T, scope accountworkbench.ExportScope) (*checkpointFixture, *checkpointSecurityBrowser, accountworkbench.OAuthCheckpointView) {
	t.Helper()
	f := newCheckpointFixture(t)
	browser := &checkpointSecurityBrowser{securityBrowser: &securityBrowser{identity: browserlogin.SecurityIdentity{Email: "owner@example.com", UserID: "user-1"}, enrollment: browserlogin.SecurityEnrollment{Secret: securitySecret, SessionID: "private-security-enrollment"}, page: browserlogin.AuthPage{Stage: "new_password", Revision: strings.Repeat("a", 64)}, passwordResult: true, closed: make(chan struct{})}}
	f.service.UseSecurityBrowser(browser)
	if err := f.service.UseSecurityDirectory(filepath.Join(t.TempDir(), "security")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.service.CloseSecurity() })
	view, err := f.service.StartOAuthWithInput(context.Background(), "checkpoint-owner", accountworkbench.OAuthStartInput{Scope: scope, ProxyURL: "http://operator-{session}:private-proxy@proxy.example:8080"})
	if err != nil {
		t.Fatal(err)
	}
	f.active = append(f.active, view.ID)
	f.phase(t, "waiting")
	checkpoint := f.save(t, view.ID)
	return f, browser, checkpoint
}

func startCheckpointSecurity(t *testing.T, f *checkpointFixture, checkpoint accountworkbench.OAuthCheckpointView, operation string) accountworkbench.SecurityView {
	t.Helper()
	input := accountworkbench.SecurityCheckpointStartInput{OAuthCheckpointAction: accountworkbench.OAuthCheckpointAction{Scope: checkpoint.Scope, Revision: checkpoint.Revision, CheckpointRevision: checkpoint.CheckpointRevision, Confirmed: true}, Operation: operation}
	if operation == "password" {
		input.Password = securityPassword
	}
	view, err := f.service.StartCheckpointSecurity(context.Background(), "checkpoint-owner", checkpoint.ID, input)
	if err != nil {
		t.Fatal(err)
	}
	return view
}

func confirmCheckpointIdentity(t *testing.T, f *checkpointFixture, id string) {
	t.Helper()
	if err := f.service.ConfirmSecurityIdentity(context.Background(), "checkpoint-owner", id, accountworkbench.SecurityIdentityInput{Email: "owner@example.com", UserID: "user-1", Confirmed: true}); err != nil {
		t.Fatal(err)
	}
	f.phase(t, "waiting")
}
