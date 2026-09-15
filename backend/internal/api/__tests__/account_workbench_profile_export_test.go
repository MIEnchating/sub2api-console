package api_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/MIEnchating/sub2api-console/backend/internal/api"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/config"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskrunner"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

func profileExportRequest(router http.Handler, token, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "http://console.test/api/account-workbench/"+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://console.test")
	if token != "" {
		req.AddCookie(&http.Cookie{Name: "sub2api_console_session", Value: token})
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	return response
}

type profileExportRunner struct {
	group *taskrunner.Group
	done  chan struct{}
}

func (r *profileExportRunner) Go(run func(context.Context)) error {
	return r.group.Go(func(ctx context.Context) {
		defer close(r.done)
		run(ctx)
	})
}

func (r *profileExportRunner) GoTask(id string, run func(context.Context)) error {
	return r.group.GoTask(id, func(ctx context.Context) {
		defer close(r.done)
		run(ctx)
	})
}

func (r *profileExportRunner) CancelTask(id string) bool {
	return r.group.CancelTask(id)
}

func TestWorkbenchProfileExportRoutesRequireAuthentication(t *testing.T) {
	router, _ := workbenchRouter(t)
	for _, path := range []string{"exports/profiles/preview", "exports/profiles"} {
		response := profileExportRequest(router, "", http.MethodPost, path, `{}`)
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("unauthenticated %s status = %d", path, response.Code)
		}
	}
}

func TestWorkbenchProfileExportRejectsInvalidScopeAndUnconfirmedExecutionWithoutCaching(t *testing.T) {
	router, token := workbenchRouter(t)
	for _, attempt := range []struct {
		path, body string
		status     int
	}{
		{"exports/profiles/preview", `{"items":"invalid"}`, http.StatusUnprocessableEntity},
		{"exports/profiles/preview", `{"items":[]}`, http.StatusConflict},
		{"exports/profiles/preview", `{"items":[{"id":"missing","revision":1}]}`, http.StatusConflict},
		{"exports/profiles", `{"preview_id":"missing","confirmed":"true"}`, http.StatusUnprocessableEntity},
		{"exports/profiles", `{"preview_id":"missing","confirmed":false}`, http.StatusConflict},
	} {
		response := profileExportRequest(router, token, http.MethodPost, attempt.path, attempt.body)
		if response.Code != attempt.status || response.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("%s response = %d %s", attempt.path, response.Code, response.Body.String())
		}
	}
}

func TestWorkbenchProfileExportReturnsOnlyMetadataAndHasNoBrowserDownload(t *testing.T) {
	private, err := configstore.Open(filepath.Join(t.TempDir(), "config.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = private.Close() })
	if err := private.Initialize(context.Background(), "tester", "isolated-password", "https://target.example", "test-admin-key"); err != nil {
		t.Fatal(err)
	}
	biz, err := business.Open(filepath.Join(t.TempDir(), "business.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = biz.Close() })
	if err := biz.Bootstrap(context.Background()); err != nil {
		t.Fatal(err)
	}
	tasks, err := taskstore.Open(filepath.Join(t.TempDir(), "tasks.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tasks.Close() })
	runner := taskrunner.NewBounded(context.Background(), 2)
	notifyingRunner := &profileExportRunner{group: runner, done: make(chan struct{})}
	service := accountworkbench.New(private, tasks, biz, nil, notifyingRunner)
	if err := service.UseExportDirectory(filepath.Join(t.TempDir(), "exports")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = service.CloseExports() })
	finish := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := runner.Shutdown(ctx); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(finish)
	router := api.New(config.Config{}, private, biz, api.Dependencies{AccountWorkbench: service, Tasks: tasks, TaskCanceller: runner})
	token, err := private.CreateSession(context.Background(), "tester", time.Hour, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	otherToken, err := private.CreateSession(context.Background(), "tester", time.Hour, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	identity, _ := json.Marshal([]string{"https://target.example", "test-admin-key"})
	digest := sha256.Sum256(identity)
	profile, err := private.SaveWorkbenchLoginProfile(context.Background(), configstore.WorkbenchLoginProfile{ID: "saved-profile-1", TargetURL: "https://target.example", TargetFingerprint: hex.EncodeToString(digest[:]), AccountID: "101", UserID: "user-101", WorkspaceID: "workspace-101", Email: "operator@example.com", HasPassword: true, Login: json.RawMessage(`{"email":"operator@example.com","password":"private-profile-password"}`)})
	if err != nil {
		t.Fatal(err)
	}
	input, _ := json.Marshal(accountworkbench.ProfileExportInput{Items: []accountworkbench.ProfileExportSelection{{ID: profile.ID, Revision: profile.Revision}}})
	previewResponse := profileExportRequest(router, token, http.MethodPost, "exports/profiles/preview", string(input))
	if previewResponse.Code != http.StatusOK || previewResponse.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("profile preview = %d %s", previewResponse.Code, previewResponse.Body.String())
	}
	var view accountworkbench.ProfileExportPreview
	if err := json.Unmarshal(previewResponse.Body.Bytes(), &view); err != nil || view.ID == "" || view.Kind != accountworkbench.ExportLoginProfiles || len(view.Items) != 1 || view.Items[0].ID != profile.ID || !view.Items[0].HasPassword {
		t.Fatalf("profile preview = %+v, %v", view, err)
	}
	body := `{"preview_id":"` + view.ID + `","confirmed":true}`
	if response := profileExportRequest(router, otherToken, http.MethodPost, "exports/profiles", body); response.Code != http.StatusConflict {
		t.Fatalf("other session export = %d", response.Code)
	}
	accepted := profileExportRequest(router, token, http.MethodPost, "exports/profiles", body)
	if accepted.Code != http.StatusAccepted || accepted.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("accepted profile export = %d %s", accepted.Code, accepted.Body.String())
	}
	var task taskstore.Task
	if err := json.Unmarshal(accepted.Body.Bytes(), &task); err != nil || task.ID == "" || task.Operation != "account-workbench-profile-export" || task.Status != "queued" {
		t.Fatalf("profile export task = %+v, %v", task, err)
	}
	select {
	case <-notifyingRunner.done:
	case <-time.After(5 * time.Second):
		t.Fatal("profile export did not finish")
	}
	listResponse := profileExportRequest(router, token, http.MethodGet, "exports", "")
	var list []accountworkbench.ExportMetadata
	if err := json.Unmarshal(listResponse.Body.Bytes(), &list); err != nil || listResponse.Code != http.StatusOK || len(list) != 1 || list[0].Kind != accountworkbench.ExportLoginProfiles || list[0].Count != 1 || listResponse.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("profile list = %d %s, %v", listResponse.Code, listResponse.Body.String(), err)
	}
	for _, response := range []*httptest.ResponseRecorder{previewResponse, accepted, listResponse} {
		if strings.Contains(response.Body.String(), "private-profile-password") || strings.Contains(response.Body.String(), "test-admin-key") || strings.Contains(response.Body.String(), `"login":`) {
			t.Fatal("profile API returned private login contents")
		}
	}
	if response := profileExportRequest(router, token, http.MethodGet, "exports/"+list[0].ID, ""); response.Code != http.StatusNotFound {
		t.Fatalf("private artifact GET = %d", response.Code)
	}
}
