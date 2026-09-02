package upstreamdelete

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

type boundedDeleteClient struct {
	active  atomic.Int32
	maximum atomic.Int32
	started chan struct{}
	release chan struct{}
}

type deleteTaskObserver struct {
	updates chan taskstore.Task
}

func (observer *deleteTaskObserver) Save(_ context.Context, task taskstore.Task) error {
	if task.Status == "succeeded" || task.Status == "failed" {
		observer.updates <- task
	}
	return nil
}

type deferredDeleteRunner struct {
	run func(context.Context)
}

func (runner *deferredDeleteRunner) Go(run func(context.Context)) error {
	runner.run = run
	return nil
}

func (runner *deferredDeleteRunner) Run(ctx context.Context) {
	if runner.run == nil {
		panic("upstream delete task was not scheduled")
	}
	runner.run(ctx)
}

func (c *boundedDeleteClient) DeleteAccountWithVerification(ctx context.Context, _ string, _ bool) (map[string]any, error) {
	current := c.active.Add(1)
	for {
		previous := c.maximum.Load()
		if current <= previous || c.maximum.CompareAndSwap(previous, current) {
			break
		}
	}
	c.started <- struct{}{}
	select {
	case <-c.release:
	case <-ctx.Done():
	}
	c.active.Add(-1)
	return map[string]any{"confirmed_absent": true}, nil
}

func TestDeleteAccountsUsesBoundedConcurrency(t *testing.T) {
	client := &boundedDeleteClient{started: make(chan struct{}, 5), release: make(chan struct{}, 5)}
	done := make(chan []error, 1)
	go func() {
		done <- deleteAccounts(context.Background(), client, []string{"1", "2", "3", "4", "5"}, true, 2)
	}()
	for range 2 {
		<-client.started
	}
	select {
	case <-client.started:
		t.Fatal("delete exceeded configured concurrency")
	case <-time.After(50 * time.Millisecond):
	}
	for range 5 {
		client.release <- struct{}{}
	}
	for _, err := range <-done {
		if err != nil {
			t.Fatal(err)
		}
	}
	if client.maximum.Load() != 2 {
		t.Fatalf("maximum concurrency=%d", client.maximum.Load())
	}
}

func TestQueuedDeleteRejectsManagementTargetChangeBeforeRemoteWrite(t *testing.T) {
	serverA := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("queued delete accessed its obsolete management target")
	}))
	defer serverA.Close()
	var targetBRequests atomic.Int32
	serverB := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		targetBRequests.Add(1)
		http.Error(response, "unexpected replacement target access", http.StatusInternalServerError)
	}))
	defer serverB.Close()
	repository := &deleteRepository{preview: business.UpstreamDeletePreview{Host: "api.example", AccountIDs: []string{"41"}}}
	private := &deletePrivateStore{target: configstore.TargetSettings{BaseURL: serverA.URL, AdminKey: "target-a", TimeoutSeconds: 2}}
	tasks := &deleteTaskObserver{updates: make(chan taskstore.Task, 1)}
	runner := &deferredDeleteRunner{}
	service := New(repository, private, tasks)
	service.UseTaskRunner(runner)
	if _, err := service.Enqueue(context.Background(), "api.example", []string{"41"}, "operator"); err != nil {
		t.Fatal(err)
	}
	private.target = configstore.TargetSettings{BaseURL: serverB.URL, AdminKey: "target-b", TimeoutSeconds: 2}
	runner.Run(context.Background())
	finished := <-tasks.updates
	if finished.Status != "failed" || !strings.Contains(fmt.Sprint(finished.Result["error"]), "管理目标") || finished.Result["remote_write"] != false {
		t.Fatalf("target-drift delete task=%#v", finished)
	}
	if requests := targetBRequests.Load(); requests != 0 {
		t.Fatalf("queued delete accessed replacement target %d times", requests)
	}
}

type deleteRepository struct {
	preview        business.UpstreamDeletePreview
	projection     business.UpstreamDeleteProjection
	previewErr     error
	projectionErr  error
	deleteCalls    int
	audit          business.UpstreamDeleteAudit
	manualControls map[string]business.ManualPriorityControl
	protections    map[string]business.AccountMutationProtection
}

func (r *deleteRepository) AccountMutationProtections(_ context.Context, accountIDs []string) (map[string]business.AccountMutationProtection, error) {
	result := make(map[string]business.AccountMutationProtection, len(accountIDs))
	for _, accountID := range accountIDs {
		protection := r.protections[accountID]
		if _, found := r.manualControls[accountID]; found {
			protection.ManualPriority = true
		}
		result[accountID] = protection
	}
	return result, nil
}

func (r *deleteRepository) Mode(context.Context) (string, error) { return "完全模式", nil }

func (r *deleteRepository) ManualPriorityControls(_ context.Context, accountIDs []string) (map[string]business.ManualPriorityControl, error) {
	result := make(map[string]business.ManualPriorityControl)
	for _, accountID := range accountIDs {
		if control, found := r.manualControls[accountID]; found {
			result[accountID] = control
		}
	}
	return result, nil
}

func (r *deleteRepository) UpstreamDeletePreview(context.Context, string) (business.UpstreamDeletePreview, error) {
	return r.preview, r.previewErr
}

func (r *deleteRepository) DeleteUpstreamProjection(_ context.Context, _ string, _ []string, audit business.UpstreamDeleteAudit) (business.UpstreamDeleteProjection, error) {
	r.deleteCalls++
	r.audit = audit
	return r.projection, r.projectionErr
}

type deletePrivateStore struct {
	target       configstore.TargetSettings
	targetErr    error
	deleteValue  bool
	deleteErr    error
	deleteCalls  int
	deletedHosts []string
}

func (s *deletePrivateStore) TargetSettings(context.Context) (configstore.TargetSettings, error) {
	return s.target, s.targetErr
}

func (s *deletePrivateStore) DeleteAuthRecord(_ context.Context, host string) (bool, error) {
	s.deleteCalls++
	s.deletedHosts = append(s.deletedHosts, host)
	return s.deleteValue, s.deleteErr
}

func deletionServer(t *testing.T, deleteStatus int) *httptest.Server {
	t.Helper()
	deleted := false
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Header.Get("X-API-Key") != "test-admin-key" {
			http.Error(response, `{"message":"missing key"}`, http.StatusUnauthorized)
			return
		}
		switch {
		case request.Method == http.MethodDelete && request.URL.Path == "/api/v1/admin/accounts/41":
			if deleteStatus >= http.StatusBadRequest {
				response.WriteHeader(deleteStatus)
				_, _ = response.Write([]byte(`{"message":"delete rejected"}`))
				return
			}
			deleted = true
			response.Header().Set("Content-Type", "application/json")
			_, _ = response.Write([]byte(`{"success":true,"data":{"id":41}}`))
		case request.Method == http.MethodGet && request.URL.Path == "/api/v1/admin/accounts/41" && deleted:
			response.WriteHeader(http.StatusNotFound)
			_, _ = response.Write([]byte(`{"message":"not found"}`))
		case request.Method == http.MethodGet && request.URL.Path == "/api/v1/admin/accounts/41":
			response.Header().Set("Content-Type", "application/json")
			_, _ = response.Write([]byte(`{"data":{"id":41}}`))
		default:
			http.NotFound(response, request)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func newDeleteService(t *testing.T, deleteStatus int) (*Service, *deleteRepository, *deletePrivateStore) {
	t.Helper()
	server := deletionServer(t, deleteStatus)
	repository := &deleteRepository{
		preview:    business.UpstreamDeletePreview{Host: "api.example", AccountIDs: []string{"41"}},
		projection: business.UpstreamDeleteProjection{Host: "api.example", DeletedAccounts: 1, DeletedGroups: 2, EventID: -7},
	}
	private := &deletePrivateStore{
		target:      configstore.TargetSettings{BaseURL: server.URL, AdminKey: "test-admin-key", TimeoutSeconds: 2},
		deleteValue: true,
	}
	return New(repository, private, nil), repository, private
}

func TestDeleteConfirmsRemoteAbsenceBeforeCommittingLocalProjection(t *testing.T) {
	service, repository, private := newDeleteService(t, http.StatusOK)
	result, err := service.Delete(context.Background(), "api.example", []string{"41"}, "admin")
	if err != nil {
		t.Fatal(err)
	}
	if repository.deleteCalls != 1 || private.deleteCalls != 1 {
		t.Fatalf("projection calls=%d private calls=%d", repository.deleteCalls, private.deleteCalls)
	}
	if repository.audit.Actor != "admin" || repository.audit.RemoteDeletedAccounts != 1 || !repository.audit.PrivateAuthDeleted || !repository.audit.ReadbackConfirmed {
		t.Fatalf("audit=%#v", repository.audit)
	}
	if result.EventID != -7 || result.RemoteDeletedAccounts != 1 || !result.RemoteWrite || !result.ReadbackConfirmed {
		t.Fatalf("result=%#v", result)
	}
}

func TestDeleteLocksAndRemovesEveryStableIdentityAuthAlias(t *testing.T) {
	service, repository, private := newDeleteService(t, http.StatusOK)
	repository.preview.IdentityHosts = []string{"HTTPS://API.EXAMPLE/", "relay.example"}
	result, err := service.Delete(context.Background(), "api.example", []string{"41"}, "admin")
	if err != nil {
		t.Fatal(err)
	}
	if !result.PrivateAuthDeleted || private.deleteCalls != 2 || strings.Join(private.deletedHosts, ",") != "api.example,relay.example" {
		t.Fatalf("result=%#v deleteCalls=%d hosts=%#v", result, private.deleteCalls, private.deletedHosts)
	}
}

func TestManualDeleteAlwaysConfirmsRemoteAbsence(t *testing.T) {
	var deletes, reads int
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		switch request.Method {
		case http.MethodDelete:
			deletes++
			_, _ = response.Write([]byte(`{"success":true}`))
		case http.MethodGet:
			reads++
			http.Error(response, `{"message":"not found"}`, http.StatusNotFound)
		}
	}))
	defer server.Close()
	repository := &deleteRepository{
		preview:    business.UpstreamDeletePreview{Host: "api.example", AccountIDs: []string{"41"}},
		projection: business.UpstreamDeleteProjection{Host: "api.example", DeletedAccounts: 1},
	}
	private := &deletePrivateStore{
		target:      configstore.TargetSettings{BaseURL: server.URL, AdminKey: "test-admin-key", TimeoutSeconds: 2},
		deleteValue: true,
	}
	result, err := New(repository, private, nil).Delete(context.Background(), "api.example", []string{"41"}, "admin")
	if err != nil {
		t.Fatal(err)
	}
	if deletes != 1 || reads != 1 || !result.ReadbackConfirmed || !repository.audit.ReadbackConfirmed {
		t.Fatalf("deletes=%d reads=%d result=%#v audit=%#v", deletes, reads, result, repository.audit)
	}
}

func TestDeleteRejectsChangedPreviewBeforeAnyRemoteOrPrivateWrite(t *testing.T) {
	service, repository, private := newDeleteService(t, http.StatusOK)
	_, err := service.Delete(context.Background(), "api.example", []string{"99"}, "admin")
	if err == nil || !strings.Contains(err.Error(), "范围已变化") {
		t.Fatalf("err=%v", err)
	}
	if repository.deleteCalls != 0 || private.deleteCalls != 0 {
		t.Fatalf("projection calls=%d private calls=%d", repository.deleteCalls, private.deleteCalls)
	}
}

func TestDeleteRejectsUpstreamContainingManualPriorityAccount(t *testing.T) {
	repository := &deleteRepository{
		preview: business.UpstreamDeletePreview{Host: "api.example", AccountIDs: []string{"41"}},
		manualControls: map[string]business.ManualPriorityControl{
			"41": {AccountID: "41"},
		},
	}
	private := &deletePrivateStore{targetErr: errors.New("target must not be read")}
	service := New(repository, private, nil)
	_, err := service.Delete(context.Background(), "api.example", []string{"41"}, "operator")
	if err == nil || !strings.Contains(err.Error(), "人工优先位") {
		t.Fatalf("manual priority delete error=%v", err)
	}
	if private.deleteCalls != 0 || repository.deleteCalls != 0 {
		t.Fatalf("manual account deletion reached writes: private=%d projection=%d", private.deleteCalls, repository.deleteCalls)
	}
}

func TestDeleteRechecksManualPauseAfterAcquiringReservations(t *testing.T) {
	repository := &deleteRepository{
		preview:     business.UpstreamDeletePreview{Host: "https://API.EXAMPLE/", AccountIDs: []string{"41"}},
		protections: map[string]business.AccountMutationProtection{"41": {Paused: true}},
	}
	private := &deletePrivateStore{targetErr: errors.New("target must not be read")}
	_, err := New(repository, private, nil).Delete(context.Background(), "api.example", []string{"41"}, "operator")
	if err == nil || !strings.Contains(err.Error(), "人工暂停") {
		t.Fatalf("manual pause delete error=%v", err)
	}
	if private.deleteCalls != 0 || repository.deleteCalls != 0 {
		t.Fatalf("protected account deletion reached writes: private=%d projection=%d", private.deleteCalls, repository.deleteCalls)
	}
}

func TestDeleteRemoteFailureLeavesPrivateAndLocalStateUntouched(t *testing.T) {
	service, repository, private := newDeleteService(t, http.StatusBadRequest)
	_, err := service.Delete(context.Background(), "api.example", []string{"41"}, "admin")
	if err == nil || !strings.Contains(err.Error(), "远程删除失败") {
		t.Fatalf("err=%v", err)
	}
	if repository.deleteCalls != 0 || private.deleteCalls != 0 {
		t.Fatalf("projection calls=%d private calls=%d", repository.deleteCalls, private.deleteCalls)
	}
}

func TestDeleteReportsAccountsRemovedBeforeALaterRemoteFailure(t *testing.T) {
	deleted := map[string]bool{}
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		accountID := strings.TrimPrefix(request.URL.Path, "/api/v1/admin/accounts/")
		if request.Method == http.MethodDelete && accountID == "42" {
			response.WriteHeader(http.StatusBadRequest)
			_, _ = response.Write([]byte(`{"message":"delete rejected"}`))
			return
		}
		if request.Method == http.MethodDelete && accountID == "41" {
			deleted[accountID] = true
			_, _ = response.Write([]byte(`{"success":true}`))
			return
		}
		if request.Method == http.MethodGet && deleted[accountID] {
			response.WriteHeader(http.StatusNotFound)
			_, _ = response.Write([]byte(`{"message":"not found"}`))
			return
		}
		if request.Method == http.MethodGet && accountID == "42" {
			_, _ = response.Write([]byte(`{"data":{"id":42}}`))
			return
		}
		http.NotFound(response, request)
	}))
	t.Cleanup(server.Close)
	repository := &deleteRepository{preview: business.UpstreamDeletePreview{
		Host: "api.example", AccountIDs: []string{"41", "42"},
	}}
	private := &deletePrivateStore{target: configstore.TargetSettings{
		BaseURL: server.URL, AdminKey: "test-admin-key", TimeoutSeconds: 2,
	}}
	result, err := New(repository, private, nil).Delete(context.Background(), "api.example", []string{"41", "42"}, "admin")
	if err == nil || !strings.Contains(err.Error(), "账号 42 远程删除失败") {
		t.Fatalf("err=%v", err)
	}
	if result.RemoteDeletedAccounts != 1 || !result.RemoteWrite {
		t.Fatalf("result=%#v", result)
	}
	if repository.deleteCalls != 0 || private.deleteCalls != 0 {
		t.Fatalf("projection calls=%d private calls=%d", repository.deleteCalls, private.deleteCalls)
	}
}

func TestDeleteKeepsLocalProjectionForRetryWhenPrivateCleanupFails(t *testing.T) {
	service, repository, private := newDeleteService(t, http.StatusOK)
	private.deleteValue = false
	private.deleteErr = errors.New("database busy")
	result, err := service.Delete(context.Background(), "api.example", []string{"41"}, "admin")
	if err == nil || !strings.Contains(err.Error(), "远端账号已删除") || !strings.Contains(err.Error(), "database busy") {
		t.Fatalf("err=%v", err)
	}
	if !result.RemoteWrite || result.RemoteDeletedAccounts != 1 || result.PrivateAuthDeleted {
		t.Fatalf("result=%#v", result)
	}
	if repository.deleteCalls != 0 {
		t.Fatalf("local projection was removed before private cleanup: calls=%d", repository.deleteCalls)
	}
}

func TestDeleteReportsPartialFailureWhenAtomicLocalCommitFails(t *testing.T) {
	service, repository, _ := newDeleteService(t, http.StatusOK)
	repository.projectionErr = errors.New("event rejected")
	result, err := service.Delete(context.Background(), "api.example", []string{"41"}, "admin")
	if err == nil || !strings.Contains(err.Error(), "远端账号已删除") || !strings.Contains(err.Error(), "event rejected") {
		t.Fatalf("err=%v", err)
	}
	if !result.RemoteWrite || !result.PrivateAuthDeleted || result.RemoteDeletedAccounts != 1 {
		t.Fatalf("result=%#v", result)
	}
}
