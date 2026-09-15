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
)

type securityBatchFactory struct {
	mu       sync.Mutex
	browsers []*securityBrowser
	opened   int
}

func (f *securityBatchFactory) OpenSecurity(_ context.Context, options browserlogin.SecurityOptions) (browserlogin.SecurityBrowser, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.opened >= len(f.browsers) {
		return nil, errors.New("unexpected isolated security browser")
	}
	if f.opened > 0 {
		select {
		case <-f.browsers[f.opened-1].closed:
		default:
			return nil, errors.New("previous security browser still open")
		}
	}
	browser := f.browsers[f.opened]
	if options.Validate() != nil || options.Email != browser.identity.Email {
		return nil, browserlogin.ErrSecurityIdentity
	}
	f.opened++
	return browser, nil
}

type securityBatchFixture struct {
	*batchFixture
	factory   *securityBatchFactory
	directory string
}

func newSecurityBatchFixture(t *testing.T, gate <-chan struct{}) *securityBatchFixture {
	t.Helper()
	f := &securityBatchFixture{batchFixture: newBatchFixture(t, gate), factory: &securityBatchFactory{}, directory: filepath.Join(t.TempDir(), "security")}
	for _, id := range []string{"101", "102"} {
		f.remote.accounts[id] = exportAccount(id, "Same display name")
		f.factory.browsers = append(f.factory.browsers, &securityBrowser{identity: browserlogin.SecurityIdentity{Email: "owner@example.com", UserID: "user-" + id}, enrollment: browserlogin.SecurityEnrollment{Secret: securitySecret, SessionID: "private-session-" + id}, page: browserlogin.AuthPage{Stage: "new_password", Revision: strings.Repeat("a", 64)}, passwordResult: true, closed: make(chan struct{})})
	}
	f.service.UseSecurityBrowser(f.factory)
	if err := f.service.UseSecurityDirectory(f.directory); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := f.service.CloseSecurity(); err != nil {
			t.Error(err)
		}
	})
	return f
}

func (f *securityBatchFixture) previewSecurityBatch(t *testing.T, operation string, ids ...string) accountworkbench.SecurityBatchPreview {
	t.Helper()
	input := accountworkbench.SecurityBatchPreviewInput{AccountIDs: ids, Operation: operation}
	if operation == "password" {
		input.Password = securityPassword
	}
	preview, err := f.service.PreviewSecurityBatch(context.Background(), "owner", input)
	if err != nil || len(preview.Errors) != 0 {
		t.Fatalf("security batch preview failed: %v %v", err, preview.Errors)
	}
	return preview
}

func (f *securityBatchFixture) startSecurityBatch(t *testing.T, operation string, ids ...string) accountworkbench.SecurityBatchView {
	t.Helper()
	preview := f.previewSecurityBatch(t, operation, ids...)
	view, err := f.service.StartSecurityBatch(context.Background(), "owner", preview.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.service.CancelSecurityBatch("owner", view.ID) })
	return view
}

func (f *securityBatchFixture) finishSecurityChild(t *testing.T, operation string) string {
	t.Helper()
	child := f.awaitTask(t, "account-workbench-security-"+operation, "waiting")
	if err := f.service.ContinueSecurity(context.Background(), "owner", child.ID); err != nil {
		t.Fatal(err)
	}
	return child.ID
}

func awaitSecurityClosed(t *testing.T, browser *securityBrowser) {
	t.Helper()
	select {
	case <-browser.closed:
	case <-time.After(5 * time.Second):
		t.Fatal("security browser cleanup did not start")
	}
}
