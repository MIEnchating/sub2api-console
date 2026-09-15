package accountworkbench_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin"
	"github.com/MIEnchating/sub2api-console/backend/internal/targetguard"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskrunner"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

type batchRunner struct {
	group       *taskrunner.Group
	gate        <-chan struct{}
	calls       atomic.Int32
	done        chan string
	rejectChild func()
}

func (r *batchRunner) Go(run func(context.Context)) error { return r.group.Go(run) }
func (r *batchRunner) CancelTask(id string) bool          { return r.group.CancelTask(id) }
func (r *batchRunner) GoTask(id string, run func(context.Context)) error {
	call := r.calls.Add(1)
	if call == 2 && r.rejectChild != nil {
		r.rejectChild()
		return taskrunner.ErrCapacity
	}
	return r.group.GoTask(id, func(ctx context.Context) {
		if call == 1 && r.gate != nil {
			<-r.gate
		}
		run(ctx)
		r.done <- id
	})
}

type batchTasks struct {
	*taskstore.Store
	onSaved           func(taskstore.Task)
	events            chan taskstore.Task
	failFinal         bool
	failSecurityFinal bool
	failImportFinal   bool
	failMixedFinal    bool
}

func (s *batchTasks) Save(ctx context.Context, task taskstore.Task) error {
	if s.failMixedFinal && task.Operation == "account-workbench-mixed" && task.Result["phase"] == "ready" {
		return errors.New("isolated mixed persistence failure")
	}
	if s.failImportFinal && (task.Operation == "account-workbench-import" || task.Operation == "account-workbench-retry") && task.Result["phase"] == "complete" {
		return errors.New("isolated import persistence failure")
	}
	if s.failFinal && task.Operation == "account-workbench-oauth-batch" && task.Status == "succeeded" {
		return errors.New("isolated task persistence failure")
	}
	if s.failSecurityFinal && task.Operation == "account-workbench-security-batch" && task.Status == "succeeded" {
		return errors.New("isolated security task persistence failure")
	}
	if err := s.Store.Save(ctx, task); err != nil {
		return err
	}
	if s.onSaved != nil {
		s.onSaved(task)
	}
	s.events <- task
	return nil
}

type batchBrowserFactory struct {
	mu        sync.Mutex
	opened    int
	previous  *oauthTestBrowser
	closeGate <-chan struct{}
}

func (f *batchBrowserFactory) OpenOAuth(ctx context.Context, options browserlogin.OAuthOptions) (browserlogin.OAuthBrowser, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.previous != nil {
		select {
		case <-f.previous.closed:
		default:
			return nil, errors.New("previous browser still open")
		}
	}
	browser := &oauthTestBrowser{closed: make(chan struct{})}
	if f.opened == 0 {
		browser.closeRelease = f.closeGate
	}
	f.opened++
	f.previous = browser
	return browser.OpenOAuth(ctx, options)
}

type batchFixture struct {
	*importFixture
	taskBoundary *batchTasks
	runBoundary  *batchRunner
	browsers     *batchBrowserFactory
}

func newBatchFixture(t *testing.T, gate <-chan struct{}) *batchFixture {
	t.Helper()
	f := &batchFixture{importFixture: newImportFixture(t, nil), browsers: &batchBrowserFactory{}}
	f.taskBoundary = &batchTasks{Store: f.tasks.Store, events: make(chan taskstore.Task, 200)}
	f.runBoundary = &batchRunner{group: f.runner, gate: gate, done: make(chan string, 30)}
	f.service = accountworkbench.New(f.private, f.taskBoundary, f.business, nil, f.runBoundary)
	f.service.UseOAuthBrowser(f.browsers)
	f.service.UseOAuthAssistTicks(make(chan time.Time))
	f.service.UseOAuthTransport(oauthTransportFunc(func(*http.Request) (*http.Response, error) {
		payload := base64.RawURLEncoding.EncodeToString([]byte(`{"email":"owner@example.com","https://api.openai.com/auth":{"chatgpt_account_id":"workspace-1","chatgpt_user_id":"user-1"}}`))
		return oauthResponse(`{"access_token":"eyJhbGciOiJub25lIn0.` + payload + `.batch-private","refresh_token":"rt_batch_private","token_type":"Bearer"}`), nil
	}))
	return f
}

func (f *batchFixture) startBatch(t *testing.T, content string) accountworkbench.OAuthBatchView {
	t.Helper()
	preview, err := f.service.PreviewOAuthBatch(context.Background(), "owner", accountworkbench.OAuthBatchPreviewInput{Content: content})
	if err != nil || len(preview.Errors) != 0 {
		t.Fatalf("batch preview: %v, %v", err, preview.Errors)
	}
	view, err := f.service.StartOAuthBatch(context.Background(), "owner", preview.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.service.CancelOAuthBatch("owner", view.ID) })
	return view
}

func (f *batchFixture) awaitTask(t *testing.T, operation, phase string) taskstore.Task {
	t.Helper()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	for {
		select {
		case task := <-f.taskBoundary.events:
			if task.Operation == operation && task.Result["phase"] == phase {
				return task
			}
		case <-deadline.C:
			t.Fatalf("batch did not reach %s %s", operation, phase)
			return taskstore.Task{}
		}
	}
}

func (f *batchFixture) awaitDone(t *testing.T, id string) {
	t.Helper()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	for {
		select {
		case done := <-f.runBoundary.done:
			if done == id {
				return
			}
		case <-deadline.C:
			t.Fatal("batch task did not finish")
			return
		}
	}
}

func TestOAuthBatchStartReturnsDetachedRows(t *testing.T) {
	gate := make(chan struct{})
	f := newBatchFixture(t, gate)
	defer close(gate)
	view := f.startBatch(t, "owner@example.com----private-password")
	view.Items[0].Email = "modified@example.com"
	stored, err := f.service.ReadOAuthBatch("owner", view.ID)
	if err != nil || stored.Items[0].Email != "owner@example.com" {
		t.Fatal("returned rows mutate live batch")
	}
}

func TestOAuthBatchCancelledBeforeRunnerStartsPersistsCancellationWithoutOpeningBrowser(t *testing.T) {
	gate := make(chan struct{})
	f := newBatchFixture(t, gate)
	view := f.startBatch(t, "owner@example.com")
	if err := f.service.CancelOAuthBatch("owner", view.ID); err != nil {
		t.Fatal(err)
	}
	close(gate)
	f.awaitDone(t, view.ID)
	task, err := f.tasks.Get(context.Background(), view.ID)
	if err != nil || task.Status != "cancelled" {
		t.Fatalf("pre-start cancellation task status=%s err=%v", task.Status, err)
	}
	f.browsers.mu.Lock()
	defer f.browsers.mu.Unlock()
	if f.browsers.opened != 0 {
		t.Fatal("cancelled batch opened a browser")
	}
}

func TestOAuthStartRejectsChangedCallerTargetBeforeTaskCreation(t *testing.T) {
	f := newBatchFixture(t, nil)
	target, err := f.private.TargetSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := f.private.ConfigureTarget(context.Background(), f.server.URL, "changed-test-key", 3); err != nil {
		t.Fatal(err)
	}
	view, err := f.service.StartOAuth(targetguard.Expect(context.Background(), target), "owner")
	if view.ID != "" {
		_ = f.service.CancelOAuth("owner", view.ID)
	}
	if !errors.Is(err, targetguard.ErrChanged) {
		t.Fatalf("changed target accepted: %v", err)
	}
}

func TestOAuthBatchFinalPersistenceFailureDiscardsImportableCredentials(t *testing.T) {
	f := newBatchFixture(t, nil)
	f.taskBoundary.failFinal = true
	view := f.startBatch(t, "owner@example.com----private-password")
	child := f.awaitTask(t, "account-workbench-oauth", "waiting")
	if err := f.service.FinishOAuth("owner", child.ID); err != nil {
		t.Fatal(err)
	}
	f.awaitDone(t, view.ID)
	stored, err := f.service.ReadOAuthBatch("owner", view.ID)
	if err != nil || stored.Status != "failed" || stored.Available != 0 {
		t.Fatalf("failed persistence left importable batch: %s/%d err=%v", stored.Status, stored.Available, err)
	}
	if _, err := f.service.PreviewOAuthBatchImport(context.Background(), "owner", view.ID, accountworkbench.OAuthPreviewInput{}); err == nil {
		t.Fatal("unrecorded authorization could be imported")
	}
}

func TestOAuthBatchSuccessCreatesPrivateAggregateImportPreviewAndCancelRevokesIt(t *testing.T) {
	f := newBatchFixture(t, nil)
	view := f.startBatch(t, "owner@example.com----private-password----JBSWY3DPEHPK3PXP")
	child := f.awaitTask(t, "account-workbench-oauth", "waiting")
	if err := f.service.FinishOAuth("owner", child.ID); err != nil {
		t.Fatal(err)
	}
	terminal := f.awaitTask(t, "account-workbench-oauth-batch", "authorized")
	f.awaitDone(t, view.ID)
	stored, err := f.service.ReadOAuthBatch("owner", view.ID)
	if err != nil || stored.Status != "authorized" || stored.Available != 1 {
		t.Fatalf("authorization failed: %s err=%v", stored.Status, err)
	}
	preview, err := f.service.PreviewOAuthBatchImport(context.Background(), "owner", view.ID, accountworkbench.OAuthPreviewInput{})
	if err != nil || len(preview.Items) != 1 || preview.Items[0].Email != "owner@example.com" {
		t.Fatalf("aggregate preview unavailable: %v", err)
	}
	for _, value := range []any{view, stored, terminal, preview} {
		raw, _ := json.Marshal(value)
		for _, secret := range []string{"private-password", "JBSWY3DPEHPK3PXP", "batch-private", "rt_batch_private", "private-auth-code", "code_verifier"} {
			if strings.Contains(string(raw), secret) {
				t.Fatal("batch response or task exposed private credentials")
			}
		}
	}
	if err := f.service.CancelOAuthBatch("owner", view.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Import(context.Background(), "owner", preview.ID, true); !errors.Is(err, accountworkbench.ErrPreview) {
		t.Fatalf("cancelled batch retained executable preview: %v", err)
	}
}

func TestOAuthBatchContinuesAfterFailedIdentityAndPreservesSuccessfulSourceIndex(t *testing.T) {
	f := newBatchFixture(t, nil)
	view := f.startBatch(t, "different@example.com\nowner@example.com")
	first := f.awaitTask(t, "account-workbench-oauth", "waiting")
	if err := f.service.FinishOAuth("owner", first.ID); err != nil {
		t.Fatal(err)
	}
	second := f.awaitTask(t, "account-workbench-oauth", "waiting")
	if first.ID == second.ID {
		t.Fatal("batch reused OAuth session")
	}
	if err := f.service.FinishOAuth("owner", second.ID); err != nil {
		t.Fatal(err)
	}
	f.awaitTask(t, "account-workbench-oauth-batch", "authorized")
	f.awaitDone(t, view.ID)
	stored, err := f.service.ReadOAuthBatch("owner", view.ID)
	if err != nil || stored.Available != 1 || stored.Items[0].Status != "failed" || stored.Items[1].Status != "succeeded" {
		t.Fatalf("partial batch result invalid: %+v %v", stored, err)
	}
	preview, err := f.service.PreviewOAuthBatchImport(context.Background(), "owner", view.ID, accountworkbench.OAuthPreviewInput{})
	if err != nil || len(preview.Items) != 1 || preview.Items[0].Index != 1 {
		t.Fatalf("successful source row lost: %+v %v", preview.Items, err)
	}
}

func TestOAuthBatchCancellationWhileChildLaunchIsRejectedFinishesWithoutRetainingResults(t *testing.T) {
	f := newBatchFixture(t, nil)
	entered, release := make(chan struct{}), make(chan struct{})
	f.runBoundary.rejectChild = func() { close(entered); <-release }
	view := f.startBatch(t, "owner@example.com")
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("child launch not reached")
	}
	if err := f.service.CancelOAuthBatch("owner", view.ID); err != nil {
		t.Fatal(err)
	}
	close(release)
	f.awaitDone(t, view.ID)
	task, err := f.tasks.Get(context.Background(), view.ID)
	if err != nil || task.Status != "cancelled" {
		t.Fatalf("launch cancellation invalid: %s %v", task.Status, err)
	}
}

func TestOAuthBatchParentTaskCancellationClosesCurrentBrowserAndSkipsRemainingRows(t *testing.T) {
	f := newBatchFixture(t, nil)
	view := f.startBatch(t, "owner@example.com\nnext@example.com")
	f.awaitTask(t, "account-workbench-oauth", "waiting")
	if !f.runBoundary.CancelTask(view.ID) {
		t.Fatal("batch task was not cancellable")
	}
	f.awaitDone(t, view.ID)
	stored, err := f.service.ReadOAuthBatch("owner", view.ID)
	if err != nil || stored.Status != "cancelled" || stored.Available != 0 || stored.CurrentOAuthID != "" {
		t.Fatalf("cancelled batch retained state: %+v %v", stored, err)
	}
	f.browsers.mu.Lock()
	defer f.browsers.mu.Unlock()
	if f.browsers.opened != 1 {
		t.Fatal("cancelled batch started another browser")
	}
	select {
	case <-f.browsers.previous.closed:
	default:
		t.Fatal("cancelled batch browser stayed open")
	}
}

func TestOAuthBatchRejectsForeignOwnerAndUnconfirmedLaunch(t *testing.T) {
	f := newBatchFixture(t, nil)
	preview, err := f.service.PreviewOAuthBatch(context.Background(), "owner", accountworkbench.OAuthBatchPreviewInput{Content: "owner@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.StartOAuthBatch(context.Background(), "other", preview.ID, true); err == nil {
		t.Fatal("foreign preview accepted")
	}
	if _, err := f.service.StartOAuthBatch(context.Background(), "owner", preview.ID, false); err == nil {
		t.Fatal("unconfirmed preview accepted")
	}
	view, err := f.service.StartOAuthBatch(context.Background(), "owner", preview.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.service.CancelOAuthBatch("owner", view.ID) })
	if _, err := f.service.ReadOAuthBatch("other", view.ID); !errors.Is(err, browserlogin.ErrSession) {
		t.Fatal("foreign batch read accepted")
	}
	if err := f.service.CancelOAuthBatch("other", view.ID); !errors.Is(err, browserlogin.ErrSession) {
		t.Fatal("foreign batch cancellation accepted")
	}
	if _, err := f.service.PreviewOAuthBatchImport(context.Background(), "other", view.ID, accountworkbench.OAuthPreviewInput{}); !errors.Is(err, browserlogin.ErrSession) {
		t.Fatal("foreign batch import accepted")
	}
}

func TestOAuthBatchWaitsForBrowserCleanupBeforeNextAccountAndReservesBrowser(t *testing.T) {
	f := newBatchFixture(t, nil)
	release := make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	f.browsers.closeGate = release
	view := f.startBatch(t, "different@example.com\nowner@example.com")
	first := f.awaitTask(t, "account-workbench-oauth", "waiting")
	if err := f.service.FinishOAuth("owner", first.ID); err != nil {
		t.Fatal(err)
	}
	f.awaitTask(t, "account-workbench-oauth", "failed")
	f.browsers.mu.Lock()
	closing := f.browsers.previous.closed
	f.browsers.mu.Unlock()
	select {
	case <-closing:
	case <-time.After(5 * time.Second):
		t.Fatal("browser cleanup did not start")
	}
	if _, err := f.service.StartOAuth(context.Background(), "owner"); err == nil {
		t.Fatal("single OAuth bypassed batch reservation")
	}
	stored, err := f.service.ReadOAuthBatch("owner", view.ID)
	if err != nil || stored.Items[1].Status != "queued" {
		t.Fatal("next row started before browser cleanup")
	}
	unblock()
	second := f.awaitTask(t, "account-workbench-oauth", "waiting")
	if err := f.service.FinishOAuth("owner", second.ID); err != nil {
		t.Fatal(err)
	}
	f.awaitDone(t, view.ID)
}

func TestOAuthBatchTargetChangeBetweenAccountsStopsQueueAndDiscardsPriorResults(t *testing.T) {
	f := newBatchFixture(t, nil)
	release := make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	f.browsers.closeGate = release
	view := f.startBatch(t, "owner@example.com\nnext@example.com")
	first := f.awaitTask(t, "account-workbench-oauth", "waiting")
	if err := f.service.FinishOAuth("owner", first.ID); err != nil {
		t.Fatal(err)
	}
	f.awaitTask(t, "account-workbench-oauth", "authorized")
	if err := f.private.ConfigureTarget(context.Background(), f.server.URL, "changed-test-key", 3); err != nil {
		t.Fatal(err)
	}
	unblock()
	f.awaitDone(t, view.ID)
	stored, err := f.service.ReadOAuthBatch("owner", view.ID)
	if err != nil || stored.Status != "failed" || stored.Available != 0 {
		t.Fatalf("changed target retained results: %+v %v", stored, err)
	}
	f.browsers.mu.Lock()
	defer f.browsers.mu.Unlock()
	if f.browsers.opened != 1 {
		t.Fatal("changed target started next browser")
	}
}

func TestOAuthBatchRejectsAuthorizedCredentialsForDifferentWorkspace(t *testing.T) {
	f := newBatchFixture(t, nil)
	view := f.startBatch(t, `[{"email":"owner@example.com","workspace_id":"workspace-other"}]`)
	child := f.awaitTask(t, "account-workbench-oauth", "waiting")
	if err := f.service.FinishOAuth("owner", child.ID); err != nil {
		t.Fatal(err)
	}
	f.awaitDone(t, view.ID)
	stored, err := f.service.ReadOAuthBatch("owner", view.ID)
	if err != nil || stored.Status != "failed" || stored.Available != 0 || !strings.Contains(stored.Items[0].Message, "工作区") {
		t.Fatalf("wrong workspace accepted: %+v %v", stored, err)
	}
}

func TestOAuthBatchBusyBrowserPreservesPreviewForRetryAfterCleanup(t *testing.T) {
	f := newBatchFixture(t, nil)
	single, err := f.service.StartOAuth(context.Background(), "owner")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.service.CancelOAuth("owner", single.ID) })
	f.awaitTask(t, "account-workbench-oauth", "waiting")
	preview, err := f.service.PreviewOAuthBatch(context.Background(), "owner", accountworkbench.OAuthBatchPreviewInput{Content: "owner@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.StartOAuthBatch(context.Background(), "owner", preview.ID, true); err == nil {
		t.Fatal("busy browser accepted batch")
	}
	if err := f.service.CancelOAuth("owner", single.ID); err != nil {
		t.Fatal(err)
	}
	f.awaitDone(t, single.ID)
	view, err := f.service.StartOAuthBatch(context.Background(), "owner", preview.ID, true)
	if err != nil {
		t.Fatalf("busy browser consumed confirmed preview: %v", err)
	}
	t.Cleanup(func() { _ = f.service.CancelOAuthBatch("owner", view.ID) })
}
