package upstreamdelete

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

type boundedDeleteClient struct {
	active  atomic.Int32
	maximum atomic.Int32
	started chan struct{}
	release chan struct{}
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

type deleteRepository struct {
	preview       business.UpstreamDeletePreview
	projection    business.UpstreamDeleteProjection
	previewErr    error
	projectionErr error
	deleteCalls   int
	audit         business.UpstreamDeleteAudit
}

func (r *deleteRepository) Mode(context.Context) (string, error) { return "full", nil }

func (r *deleteRepository) UpstreamDeletePreview(context.Context, string) (business.UpstreamDeletePreview, error) {
	return r.preview, r.previewErr
}

func (r *deleteRepository) DeleteUpstreamProjection(_ context.Context, _ string, _ []string, audit business.UpstreamDeleteAudit) (business.UpstreamDeleteProjection, error) {
	r.deleteCalls++
	r.audit = audit
	return r.projection, r.projectionErr
}

type deletePrivateStore struct {
	target      configstore.TargetSettings
	targetErr   error
	deleteValue bool
	deleteErr   error
	deleteCalls int
}

func (s *deletePrivateStore) TargetSettings(context.Context) (configstore.TargetSettings, error) {
	return s.target, s.targetErr
}

func (s *deletePrivateStore) DeleteAuthRecord(context.Context, string) (bool, error) {
	s.deleteCalls++
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
