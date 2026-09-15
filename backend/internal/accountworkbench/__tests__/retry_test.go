package accountworkbench_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func TestRetryStagedImportAfterServiceRestartDetectsAndPromotesOriginalAccount(t *testing.T) {
	f := newImportFixture(t, nil)
	if err := f.private.SaveWorkbenchTemplate(context.Background(), configstore.WorkbenchTemplate{ID: "default", Name: "Default", TargetURL: f.server.URL, Config: configstore.WorkbenchTemplateConfig{"group_ids": json.RawMessage(`[7]`)}}); err != nil {
		t.Fatal(err)
	}
	task := f.start(t, f.preview(t, importedJSON, false))
	f.await(t)
	record, err := f.private.WorkbenchExecution(context.Background(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	privateJSON, _ := json.Marshal(record)
	if strings.Contains(string(privateJSON), "access-new-private") || strings.Contains(string(privateJSON), "rt_new_private") {
		t.Fatal("confirmed online credentials must be removed from retry persistence")
	}
	restarted := accountworkbench.New(f.private, f.tasks, f.business, checkerBoundary(func(context.Context, string, string, map[string]any, string, int) (map[string]any, error) {
		return map[string]any{"verdict": "SOL_CONSISTENT"}, nil
	}), f.runner)
	preview, err := restarted.RetryPreview(context.Background(), "owner", accountworkbench.RetryPreviewInput{TaskID: task.ID, Indexes: []int{0}, Model: "gpt-5.6-sol"})
	if err != nil || len(preview.Items) != 1 || preview.Items[0].AccountID != "101" || preview.Items[0].Action != "check" || !preview.CheckAfterImport {
		t.Fatalf("retry preview = %+v, %v", preview, err)
	}
	if _, err := restarted.Import(context.Background(), "owner", preview.ID, true); err != nil {
		t.Fatal(err)
	}
	completed := f.await(t)
	if completed.Status != "succeeded" {
		t.Fatalf("retry result = %+v", completed)
	}
	f.remote.mu.Lock()
	defer f.remote.mu.Unlock()
	if f.remote.created != 1 || f.remote.accounts["101"]["schedulable"] != true {
		t.Fatalf("retry duplicated or did not promote account: %d, %+v", f.remote.created, f.remote.accounts["101"])
	}
	groups, _ := json.Marshal(f.remote.accounts["101"]["group_ids"])
	if string(groups) != `[7]` {
		t.Fatalf("original new-account template lost: %s", groups)
	}
	raw, _ := json.Marshal(completed)
	if strings.Contains(string(raw), "access-new-private") || strings.Contains(string(raw), "rt_new_private") {
		t.Fatal("retry task exposes private credentials")
	}
}

func TestRetryUncertainCreateReconcilesExactMarkerBeforeUsingAccount(t *testing.T) {
	f := newImportFixture(t, checkerBoundary(func(context.Context, string, string, map[string]any, string, int) (map[string]any, error) {
		return map[string]any{"verdict": "SOL_CONSISTENT"}, nil
	}))
	f.remote.unknownCreate = true
	task := f.start(t, f.preview(t, importedJSON, false))
	f.await(t)
	record, err := f.private.WorkbenchExecution(context.Background(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	var credentials map[string]any
	if err := json.Unmarshal(record.Items[0].Credentials, &credentials); err != nil {
		t.Fatal(err)
	}
	f.remote.mu.Lock()
	f.remote.accounts["101"] = map[string]any{"id": json.Number("101"), "platform": "openai", "type": "oauth", "name": "Imported", "status": "inactive", "schedulable": false, "group_ids": []any{}, "notes": record.Items[0].Marker, "credentials": credentials}
	f.remote.mu.Unlock()
	preview, err := f.service.RetryPreview(context.Background(), "owner", accountworkbench.RetryPreviewInput{TaskID: task.ID})
	if err != nil || len(preview.Items) != 1 || preview.Items[0].AccountID != "101" {
		t.Fatalf("marker reconciliation preview = %+v, %v", preview, err)
	}
	f.start(t, preview)
	if result := f.await(t); result.Status != "succeeded" {
		t.Fatalf("reconciled import = %+v", result)
	}
	f.remote.mu.Lock()
	defer f.remote.mu.Unlock()
	if f.remote.created != 1 {
		t.Fatal("reconciliation reissued create")
	}
}

func TestRetryExistingCredentialResponseLossReusesConfirmedOnlineCredential(t *testing.T) {
	f := newImportFixture(t, checkerBoundary(func(context.Context, string, string, map[string]any, string, int) (map[string]any, error) {
		return map[string]any{"verdict": "SOL_CONSISTENT"}, nil
	}))
	f.existing()
	f.service.UseTransport(&lostMutationResponse{match: func(request *http.Request, payload map[string]any) bool {
		_, changesCredentials := payload["credentials"]
		return request.Method == http.MethodPut && changesCredentials
	}})
	task := f.start(t, f.preview(t, importedJSON, true))
	if result := f.await(t); result.Status != "partial" {
		t.Fatalf("expected credential response loss, got %+v", result)
	}
	preview, err := f.service.RetryPreview(context.Background(), "owner", accountworkbench.RetryPreviewInput{TaskID: task.ID})
	if err != nil {
		t.Fatal(err)
	}
	if preview.Items[0].Action != "check" {
		t.Fatalf("confirmed credentials must preview a behavior check: %+v", preview.Items[0])
	}
	f.start(t, preview)
	if result := f.await(t); result.Status != "succeeded" {
		t.Fatalf("existing credential retry = %+v", result)
	}
	f.remote.mu.Lock()
	defer f.remote.mu.Unlock()
	groups, _ := json.Marshal(f.remote.accounts["101"]["group_ids"])
	if string(groups) != `[7]` || f.remote.created != 0 || f.remote.accounts["101"]["name"] != "Original" {
		t.Fatal("retry changed existing account identity or configuration")
	}
}

func TestRetryTargetTimeoutChangePreservesOriginalManagementIdentity(t *testing.T) {
	f := newImportFixture(t, nil)
	task := f.start(t, f.preview(t, importedJSON, false))
	f.await(t)
	if err := f.private.ConfigureTarget(context.Background(), f.server.URL, "test-admin-key", 4); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.RetryPreview(context.Background(), "owner", accountworkbench.RetryPreviewInput{TaskID: task.ID}); err != nil {
		t.Fatalf("timeout-only change invalidated management identity: %v", err)
	}
}

type lostMutationResponse struct {
	match func(*http.Request, map[string]any) bool
	lost  atomic.Bool
}

func (fault *lostMutationResponse) RoundTrip(request *http.Request) (*http.Response, error) {
	if request.URL.Hostname() != "127.0.0.1" {
		return nil, errors.New("test refuses non-isolated HTTP endpoint")
	}
	var payload map[string]any
	if request.GetBody != nil {
		body, err := request.GetBody()
		if err != nil {
			return nil, err
		}
		_ = json.NewDecoder(body).Decode(&payload)
		_ = body.Close()
	}
	response, err := http.DefaultTransport.RoundTrip(request)
	if err == nil && fault.match(request, payload) && fault.lost.CompareAndSwap(false, true) {
		_, _ = io.Copy(io.Discard, response.Body)
		_ = response.Body.Close()
		return nil, errors.New("isolated post-commit response loss")
	}
	return response, err
}

func TestRetryAfterPromotionConfigurationResponseLossKeepsTemplateAndResumes(t *testing.T) {
	f := newImportFixture(t, checkerBoundary(func(context.Context, string, string, map[string]any, string, int) (map[string]any, error) {
		return map[string]any{"verdict": "SOL_CONSISTENT"}, nil
	}))
	f.service.UseTransport(&lostMutationResponse{match: func(request *http.Request, payload map[string]any) bool {
		return request.Method == http.MethodPut && strings.HasSuffix(request.URL.Path, "/accounts/101") && payload["status"] == "active"
	}})
	task := f.start(t, f.preview(t, importedJSON, true))
	if result := f.await(t); result.Status != "partial" {
		t.Fatalf("expected uncertain partial promotion, got %+v", result)
	}
	preview, err := f.service.RetryPreview(context.Background(), "owner", accountworkbench.RetryPreviewInput{TaskID: task.ID})
	if err != nil {
		t.Fatalf("own confirmed configuration blocked retry: %v", err)
	}
	f.start(t, preview)
	if result := f.await(t); result.Status != "succeeded" {
		t.Fatalf("promotion retry = %+v", result)
	}
	f.remote.mu.Lock()
	defer f.remote.mu.Unlock()
	if f.remote.created != 1 || f.remote.accounts["101"]["schedulable"] != true {
		t.Fatal("retry must enable the original account")
	}
}

func TestRetryAfterEnableResponseLossReconcilesWithoutAnotherCheckOrWrite(t *testing.T) {
	var checks atomic.Int32
	f := newImportFixture(t, checkerBoundary(func(context.Context, string, string, map[string]any, string, int) (map[string]any, error) {
		checks.Add(1)
		return map[string]any{"verdict": "SOL_CONSISTENT"}, nil
	}))
	f.service.UseTransport(&lostMutationResponse{match: func(_ *http.Request, payload map[string]any) bool {
		return payload["schedulable"] == true
	}})
	task := f.start(t, f.preview(t, importedJSON, true))
	if result := f.await(t); result.Status != "partial" {
		t.Fatalf("expected uncertain enable, got %+v", result)
	}
	f.remote.mu.Lock()
	before := f.remote.updates
	f.remote.mu.Unlock()
	preview, err := f.service.RetryPreview(context.Background(), "owner", accountworkbench.RetryPreviewInput{TaskID: task.ID})
	if err != nil {
		t.Fatal(err)
	}
	if preview.Items[0].Action != "reconcile" {
		t.Fatalf("enabled account must preview read-only reconciliation: %+v", preview.Items[0])
	}
	f.start(t, preview)
	if result := f.await(t); result.Status != "succeeded" {
		t.Fatalf("read-only reconciliation = %+v", result)
	}
	f.remote.mu.Lock()
	defer f.remote.mu.Unlock()
	if f.remote.updates != before || checks.Load() != 1 || f.remote.created != 1 {
		t.Fatal("already-enabled account was checked or mutated again")
	}
}

func TestRetryStagedAccountRejectsExternalConfigurationChange(t *testing.T) {
	f := newImportFixture(t, nil)
	task := f.start(t, f.preview(t, importedJSON, false))
	f.await(t)
	f.remote.mu.Lock()
	f.remote.accounts["101"]["notes"] = "external change"
	f.remote.mu.Unlock()
	if _, err := f.service.RetryPreview(context.Background(), "owner", accountworkbench.RetryPreviewInput{TaskID: task.ID}); err == nil {
		t.Fatal("external changes must require a fresh import preview")
	}
}

func TestRetryEnabledReconciliationShowsOnlineGroupsWhenOriginalTemplateChanged(t *testing.T) {
	f := newImportFixture(t, checkerBoundary(func(context.Context, string, string, map[string]any, string, int) (map[string]any, error) {
		return map[string]any{"verdict": "SOL_CONSISTENT"}, nil
	}))
	ctx := context.Background()
	template := configstore.WorkbenchTemplate{ID: "default", Name: "Default", TargetURL: f.server.URL, Config: configstore.WorkbenchTemplateConfig{"group_ids": json.RawMessage(`[7]`)}}
	if err := f.private.SaveWorkbenchTemplate(ctx, template); err != nil {
		t.Fatal(err)
	}
	f.service.UseTransport(&lostMutationResponse{match: func(_ *http.Request, payload map[string]any) bool {
		return payload["schedulable"] == true
	}})
	task := f.start(t, f.preview(t, importedJSON, true))
	f.await(t)
	template.Revision = 1
	template.Config["group_ids"] = json.RawMessage(`[8]`)
	if err := f.private.SaveWorkbenchTemplate(ctx, template); err != nil {
		t.Fatal(err)
	}
	preview, err := f.service.RetryPreview(ctx, "owner", accountworkbench.RetryPreviewInput{TaskID: task.ID})
	if err != nil {
		t.Fatal(err)
	}
	row := preview.Items[0]
	if row.Action != "reconcile" || row.TemplateID != "" || len(row.GroupIDs) != 1 || row.GroupIDs[0] != "7" {
		t.Fatalf("read-only confirmation must show online applied config, not changed template: %+v", row)
	}
}

func TestRetryConcurrentPreviewsOnlyOneClaimsOriginalExecution(t *testing.T) {
	f := newImportFixture(t, checkerBoundary(func(context.Context, string, string, map[string]any, string, int) (map[string]any, error) {
		return map[string]any{"verdict": "SOL_CONSISTENT"}, nil
	}))
	task := f.start(t, f.preview(t, importedJSON, false))
	f.await(t)
	first, err := f.service.RetryPreview(context.Background(), "first", accountworkbench.RetryPreviewInput{TaskID: task.ID})
	if err != nil {
		t.Fatal(err)
	}
	second, err := f.service.RetryPreview(context.Background(), "second", accountworkbench.RetryPreviewInput{TaskID: task.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Import(context.Background(), "first", first.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Import(context.Background(), "second", second.ID, true); err != nil {
		t.Fatal(err)
	}
	results := []string{f.await(t).Status, f.await(t).Status}
	if !((results[0] == "succeeded" && results[1] == "failed") || (results[0] == "failed" && results[1] == "succeeded")) {
		t.Fatalf("both previews consumed the same execution: %v", results)
	}
}

func TestRetryRejectedCompetingTaskCannotBecomeIndependentRetrySource(t *testing.T) {
	allowCheck := make(chan struct{})
	var release sync.Once
	defer release.Do(func() { close(allowCheck) })
	f := newImportFixture(t, checkerBoundary(func(ctx context.Context, _, _ string, _ map[string]any, _ string, _ int) (map[string]any, error) {
		select {
		case <-allowCheck:
			return map[string]any{"verdict": "SOL_CONSISTENT"}, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}))
	task := f.start(t, f.preview(t, importedJSON, false))
	f.await(t)
	previews := map[string]accountworkbench.Preview{}
	for _, owner := range []string{"first", "second"} {
		preview, err := f.service.RetryPreview(context.Background(), owner, accountworkbench.RetryPreviewInput{TaskID: task.ID})
		if err != nil {
			t.Fatal(err)
		}
		previews[owner] = preview
	}
	for _, owner := range []string{"first", "second"} {
		if _, err := f.service.Import(context.Background(), owner, previews[owner].ID, true); err != nil {
			t.Fatal(err)
		}
	}
	rejected := f.await(t)
	if rejected.Status != "failed" {
		t.Fatalf("expected competing claim to fail: %+v", rejected)
	}
	if _, err := f.service.RetryPreview(context.Background(), "third", accountworkbench.RetryPreviewInput{TaskID: rejected.ID}); err == nil {
		t.Fatal("unclaimed child execution must not bypass its original source claim")
	}
	release.Do(func() { close(allowCheck) })
	f.await(t)
}

func TestRetryUncertainCreateWithoutMarkerNeverReplaysCreation(t *testing.T) {
	f := newImportFixture(t, nil)
	f.remote.unknownCreate = true
	task := f.start(t, f.preview(t, importedJSON, false))
	f.await(t)
	_, err := f.service.RetryPreview(context.Background(), "owner", accountworkbench.RetryPreviewInput{TaskID: task.ID, Indexes: []int{0}})
	if err == nil {
		t.Fatal("unknown creation without its stable marker must require reconciliation")
	}
	f.remote.mu.Lock()
	defer f.remote.mu.Unlock()
	if f.remote.created != 1 {
		t.Fatal("retry reissued a create request")
	}
}

func TestRetryRejectsTargetCredentialChangeAfterOriginalTask(t *testing.T) {
	f := newImportFixture(t, nil)
	task := f.start(t, f.preview(t, importedJSON, false))
	f.await(t)
	if err := f.private.ConfigureTarget(context.Background(), f.server.URL, "different-admin-key", 3); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.RetryPreview(context.Background(), "owner", accountworkbench.RetryPreviewInput{TaskID: task.ID}); err == nil {
		t.Fatal("retry reused private execution state after management credentials changed")
	}
}

func TestRetryFailedSubsetPreservesOriginalInputIndexAndSkipsSuccessfulAccount(t *testing.T) {
	f := newImportFixture(t, checkerBoundary(func(_ context.Context, id, _ string, _ map[string]any, _ string, _ int) (map[string]any, error) {
		if id == "101" {
			return map[string]any{"verdict": "SOL_CONSISTENT"}, nil
		}
		return map[string]any{"verdict": "INCONCLUSIVE"}, nil
	}))
	f.existing()
	f.remote.accounts["202"] = map[string]any{"id": json.Number("202"), "name": "Second", "platform": "openai", "type": "oauth", "status": "active", "schedulable": true, "group_ids": []any{json.Number("7")}, "credentials": map[string]any{"access_token": "second-old-private", "refresh_token": "rt_second_private", "chatgpt_account_id": "workspace-2", "chatgpt_user_id": "user-2"}}
	content := `[` + importedJSON + `,{"name":"Second","credentials":{"access_token":"second-new-private","refresh_token":"rt_second_private","chatgpt_account_id":"workspace-2","chatgpt_user_id":"user-2"}}]`
	task := f.start(t, f.preview(t, content, true))
	if result := f.await(t); result.Status != "partial" {
		t.Fatalf("expected one failed check in original batch, got %+v", result)
	}
	preview, err := f.service.RetryPreview(context.Background(), "owner", accountworkbench.RetryPreviewInput{TaskID: task.ID})
	if err != nil || len(preview.Items) != 1 || preview.Items[0].Index != 1 || preview.Items[0].AccountID != "202" {
		t.Fatalf("retry subset must retain input index 1 only: %+v, %v", preview, err)
	}
}
