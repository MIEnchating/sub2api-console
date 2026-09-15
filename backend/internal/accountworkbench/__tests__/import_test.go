package accountworkbench_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskrunner"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

const importedJSON = `{"name":"Imported","credentials":{"access_token":"access-new-private","refresh_token":"rt_new_private","chatgpt_account_id":"workspace-1","chatgpt_user_id":"user-1"}}`

type importTaskStore struct {
	*taskstore.Store
	terminal chan taskstore.Task
}

func (s *importTaskStore) Save(ctx context.Context, task taskstore.Task) error {
	if err := s.Store.Save(ctx, task); err != nil {
		return err
	}
	switch task.Status {
	case "succeeded", "partial", "failed", "cancelled":
		s.terminal <- task
	}
	return nil
}

type checkerBoundary func(context.Context, string, string, map[string]any, string, int) (map[string]any, error)

func (check checkerBoundary) CheckOAuth(ctx context.Context, id, name string, credentials map[string]any, model string, timeout int) (map[string]any, error) {
	return check(ctx, id, name, credentials, model, timeout)
}

type upstreamFixture struct {
	mu                          sync.Mutex
	accounts                    map[string]map[string]any
	created, updates, refreshes int
	unknownCreate               bool
	mutated                     func(string, map[string]any, map[string]any)
}

func (f *upstreamFixture) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	if r.Header.Get("x-api-key") != "test-admin-key" {
		http.Error(w, "invalid test credentials", 401)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/admin")
	var body map[string]any
	if r.Body != nil && r.Method != http.MethodGet {
		decoder := json.NewDecoder(r.Body)
		decoder.UseNumber()
		_ = decoder.Decode(&body)
	}
	respond := func(data any) { _ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": data}) }
	if path == "/openai/refresh-token" {
		f.refreshes++
		respond(map[string]any{"access_token": "access-new-private", "refresh_token": "rt_new_private", "chatgpt_account_id": "workspace-1", "chatgpt_user_id": "user-1", "email": "owner@example.com"})
		return
	}
	if strings.HasPrefix(path, "/openai/accounts/") && strings.HasSuffix(path, "/refresh") {
		id := strings.TrimSuffix(strings.TrimPrefix(path, "/openai/accounts/"), "/refresh")
		account := f.accounts[id]
		if account == nil {
			http.Error(w, "not found", 404)
			return
		}
		f.refreshes++
		credentials := account["credentials"].(map[string]any)
		credentials["access_token"], credentials["refresh_token"] = "access-refreshed-private", "rt_refreshed_private"
		if f.mutated != nil {
			f.mutated(path, account, body)
		}
		respond(account)
		return
	}
	if path == "/accounts" && r.Method == http.MethodGet {
		items := []map[string]any{}
		for _, account := range f.accounts {
			items = append(items, account)
		}
		respond(map[string]any{"items": items, "total": len(items)})
		return
	}
	if path == "/accounts" && r.Method == http.MethodPost {
		f.created++
		if f.unknownCreate {
			_, _ = w.Write([]byte(`{"code":0,"data":{}}`))
			return
		}
		body["id"] = json.Number("101")
		f.accounts["101"] = body
		if f.mutated != nil {
			f.mutated(path, body, body)
		}
		respond(body)
		return
	}
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) >= 2 && parts[0] == "accounts" {
		account := f.accounts[parts[1]]
		if account == nil {
			http.Error(w, "not found", 404)
			return
		}
		if r.Method != http.MethodGet {
			f.updates++
			for key, value := range body {
				account[key] = value
			}
			if len(parts) == 3 && parts[2] == "clear-error" {
				account["error_message"] = ""
				account["status"] = "active"
			}
			if len(parts) == 3 && parts[2] == "recover-state" {
				account["status"] = "active"
				for _, key := range []string{"error_message", "error", "temp_unschedulable_reason", "temp_unschedulable_until", "rate_limit_reset_at", "overload_until"} {
					delete(account, key)
				}
			}
			if f.mutated != nil {
				f.mutated(path, account, body)
			}
		}
		respond(account)
		return
	}
	http.Error(w, "unexpected isolated endpoint", 404)
}

type importFixture struct {
	service  *accountworkbench.Service
	private  *configstore.Store
	business *business.Store
	tasks    *importTaskStore
	runner   *taskrunner.Group
	remote   *upstreamFixture
	server   *httptest.Server
}

func newImportFixture(t *testing.T, checker checkerBoundary) *importFixture {
	t.Helper()
	f := &importFixture{remote: &upstreamFixture{accounts: map[string]map[string]any{}}}
	f.server = httptest.NewServer(f.remote)
	t.Cleanup(f.server.Close)
	var err error
	f.private, err = configstore.Open(filepath.Join(t.TempDir(), "config.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.private.Close() })
	if err = f.private.ConfigureTarget(context.Background(), f.server.URL, "test-admin-key", 3); err != nil {
		t.Fatal(err)
	}
	f.business, err = business.Open(filepath.Join(t.TempDir(), "business.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.business.Close() })
	if err = f.business.Bootstrap(context.Background()); err != nil {
		t.Fatal(err)
	}
	store, err := taskstore.Open(filepath.Join(t.TempDir(), "tasks.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	f.tasks = &importTaskStore{Store: store, terminal: make(chan taskstore.Task, 10)}
	f.runner = taskrunner.NewBounded(context.Background(), 3)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := f.runner.Shutdown(ctx); err != nil {
			t.Error(err)
		}
	})
	var oauthChecker accountworkbench.OAuthChecker
	if checker != nil {
		oauthChecker = checker
	}
	f.service = accountworkbench.New(f.private, f.tasks, f.business, oauthChecker, f.runner)
	return f
}
func (f *importFixture) await(t *testing.T) taskstore.Task {
	t.Helper()
	select {
	case task := <-f.tasks.terminal:
		return task
	case <-time.After(5 * time.Second):
		t.Fatal("isolated import task did not finish")
		return taskstore.Task{}
	}
}
func (f *importFixture) preview(t *testing.T, content string, check bool) accountworkbench.Preview {
	t.Helper()
	preview, err := f.service.Preview(context.Background(), "owner", accountworkbench.PreviewInput{Content: content, CheckAfterImport: check})
	if err != nil || len(preview.Errors) > 0 {
		t.Fatalf("preview = %v, %v", preview.Errors, err)
	}
	return preview
}
func (f *importFixture) start(t *testing.T, preview accountworkbench.Preview) taskstore.Task {
	t.Helper()
	task, err := f.service.Import(context.Background(), "owner", preview.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	return task
}
func (f *importFixture) existing() {
	f.remote.accounts["101"] = map[string]any{"id": json.Number("101"), "name": "Original", "platform": "openai", "type": "oauth", "status": "active", "schedulable": true, "group_ids": []any{json.Number("7")}, "credentials": map[string]any{"access_token": "access-old-private", "refresh_token": "rt_old_private", "chatgpt_account_id": "workspace-1", "chatgpt_user_id": "user-1", "model_mapping": map[string]any{"gpt-5": "gpt-5.6"}, "custom_pool_flag": true}}
}

func TestImportRefreshTokenPreviewFindsStableExistingIdentityBeforeConfirmation(t *testing.T) {
	f := newImportFixture(t, nil)
	f.existing()
	preview := f.preview(t, "rt_new_private", false)
	if !preview.Items[0].Duplicate || preview.Items[0].AccountID != "101" {
		t.Fatalf("refresh preview missed existing identity: %+v", preview.Items[0])
	}
	f.start(t, preview)
	task := f.await(t)
	if task.Status != "partial" {
		t.Fatalf("unchecked import status = %s", task.Status)
	}
	if f.remote.created != 0 || f.remote.refreshes != 1 {
		t.Fatalf("refresh should occur once and not duplicate account: creates=%d refreshes=%d", f.remote.created, f.remote.refreshes)
	}
}

func TestImportExistingAccountPreservesCredentialExtras(t *testing.T) {
	f := newImportFixture(t, nil)
	f.existing()
	preview := f.preview(t, importedJSON, false)
	f.start(t, preview)
	f.await(t)
	credentials := f.remote.accounts["101"]["credentials"].(map[string]any)
	if credentials["custom_pool_flag"] != true || credentials["model_mapping"] == nil {
		t.Fatal("existing credential extras were dropped")
	}
	if credentials["access_token"] != "access-new-private" {
		t.Fatal("new token was not applied")
	}
}

func TestImportWithoutDetectionCreatesInactiveUnschedulableAccount(t *testing.T) {
	f := newImportFixture(t, nil)
	preview := f.preview(t, importedJSON, false)
	f.start(t, preview)
	task := f.await(t)
	if task.Status != "partial" || f.remote.accounts["101"]["schedulable"] != false || f.remote.accounts["101"]["status"] != "inactive" {
		raw, _ := json.Marshal(task)
		t.Fatalf("unchecked account became usable: task=%s remote=%v", raw, f.remote.accounts["101"])
	}
}

func TestImportDetectionPassAppliesTemplateAndEnablesAfterReadback(t *testing.T) {
	f := newImportFixture(t, func(_ context.Context, _, _ string, credentials map[string]any, _ string, _ int) (map[string]any, error) {
		if credentials["access_token"] != "access-new-private" {
			return nil, errors.New("incorrect credentials")
		}
		return map[string]any{"verdict": "SOL_CONSISTENT"}, nil
	})
	template, err := f.service.SaveTemplate(context.Background(), "", accountworkbench.TemplateInput{Name: "Default", Config: configstore.WorkbenchTemplateConfig{"group_ids": json.RawMessage(`["7"]`), "concurrency": json.RawMessage(`4`), "rate_multiplier": json.RawMessage(`"0.125"`)}})
	if err != nil {
		t.Fatal(err)
	}
	preview := f.preview(t, importedJSON, true)
	if preview.Items[0].TemplateID != template.ID {
		t.Fatal("default template was not matched")
	}
	f.start(t, preview)
	task := f.await(t)
	account := f.remote.accounts["101"]
	if task.Status != "succeeded" || account["schedulable"] != true || account["status"] != "active" || account["rate_multiplier"] != json.Number("0.125") {
		t.Fatalf("promotion failed: task=%s account=%v", task.Status, account["status"])
	}
}

func TestImportDetectionFailureNeverEnablesAndOmitsCredentialsFromTask(t *testing.T) {
	f := newImportFixture(t, func(context.Context, string, string, map[string]any, string, int) (map[string]any, error) {
		return map[string]any{"verdict": "INCONCLUSIVE", "access_token": "access-new-private", "reason": "echo access-new-private"}, nil
	})
	preview := f.preview(t, importedJSON, true)
	f.start(t, preview)
	task := f.await(t)
	if f.remote.accounts["101"]["schedulable"] != false {
		t.Fatal("failed check enabled account")
	}
	raw, _ := json.Marshal(task)
	if strings.Contains(string(raw), "access-new-private") {
		t.Fatal("checker report leaked credentials into task")
	}
}

func TestImportRejectsForeignSessionAndChangedTemplate(t *testing.T) {
	f := newImportFixture(t, nil)
	template, err := f.service.SaveTemplate(context.Background(), "", accountworkbench.TemplateInput{Name: "Default"})
	if err != nil {
		t.Fatal(err)
	}
	preview := f.preview(t, importedJSON, false)
	if _, err = f.service.Import(context.Background(), "other", preview.ID, true); !errors.Is(err, accountworkbench.ErrPreview) {
		t.Fatalf("foreign session import = %v", err)
	}
	if _, err = f.service.SaveTemplate(context.Background(), template.ID, accountworkbench.TemplateInput{Name: "Changed", Revision: template.Revision}); err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.Import(context.Background(), "owner", preview.ID, true); err == nil {
		t.Fatal("stale template preview executed")
	}
	if f.remote.created != 0 {
		t.Fatal("conflict mutated remote")
	}
}

func TestImportRejectsChangedTargetWithoutContactingNewTarget(t *testing.T) {
	f := newImportFixture(t, nil)
	preview := f.preview(t, importedJSON, false)
	if err := f.private.ConfigureTarget(context.Background(), "http://127.0.0.1:1", "different-private-key", 3); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Import(context.Background(), "owner", preview.ID, true); err == nil {
		t.Fatal("changed target accepted")
	}
	if f.remote.created != 0 {
		t.Fatal("changed target mutated previous remote")
	}
}

func TestImportCancellationWhileCheckingKeepsAccountDisabled(t *testing.T) {
	started := make(chan struct{})
	f := newImportFixture(t, func(ctx context.Context, _, _ string, _ map[string]any, _ string, _ int) (map[string]any, error) {
		close(started)
		<-ctx.Done()
		return nil, ctx.Err()
	})
	preview := f.preview(t, importedJSON, true)
	queued := f.start(t, preview)
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("checker did not start")
	}
	if !f.runner.CancelTask(queued.ID) {
		t.Fatal("runner could not cancel task")
	}
	task := f.await(t)
	if task.Status != "cancelled" || f.remote.accounts["101"]["schedulable"] != false {
		t.Fatalf("cancellation state=%s", task.Status)
	}
}

func TestImportUnknownCreateDoesNotReplayCreateRequest(t *testing.T) {
	f := newImportFixture(t, nil)
	f.remote.unknownCreate = true
	preview := f.preview(t, importedJSON, false)
	f.start(t, preview)
	task := f.await(t)
	if task.Status == "succeeded" || f.remote.created != 1 {
		t.Fatalf("unknown commit replay: status=%s creates=%d", task.Status, f.remote.created)
	}
	if _, err := f.service.Import(context.Background(), "owner", preview.ID, true); err == nil {
		t.Fatal("consumed preview replayed")
	}
}

func TestPreviewRejectsCredentialCopiedIntoVisibleName(t *testing.T) {
	f := newImportFixture(t, nil)
	input := strings.Replace(importedJSON, `"Imported"`, `"access-new-private"`, 1)
	preview, err := f.service.Preview(context.Background(), "owner", accountworkbench.PreviewInput{Content: input})
	if err == nil && len(preview.Errors) == 0 {
		raw, _ := json.Marshal(preview)
		t.Fatalf("credential metadata accepted: %s", raw)
	}
}
