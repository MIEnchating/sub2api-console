package accountworkbench_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

func regenerationTransport(f *importFixture, refresh http.HandlerFunc) {
	f.service.UseTransport(oauthTransportFunc(func(r *http.Request) (*http.Response, error) {
		response := httptest.NewRecorder()
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/openai/refresh-token") {
			refresh(response, r)
		} else {
			f.remote.ServeHTTP(response, r)
		}
		return response.Result(), nil
	}))
}

func previewRegeneration(t *testing.T, f *importFixture, owner string, input accountworkbench.RegenerationInput) accountworkbench.RegenerationPreview {
	t.Helper()
	view, err := f.service.PreviewRegeneration(context.Background(), owner, input)
	if err != nil || view.ID == "" || len(view.Items) == 0 {
		t.Fatalf("regeneration preview = %+v, %v", view, err)
	}
	return view
}

func executeRegeneration(t *testing.T, f *importFixture, owner string, view accountworkbench.RegenerationPreview) taskstore.Task {
	t.Helper()
	if _, err := f.service.Regenerate(context.Background(), owner, view.ID, true); err != nil {
		t.Fatal(err)
	}
	return f.await(t)
}

func TestRegenerationUsesRTAfterConfirmationAndPreservesConfigWithoutOnlineWrites(t *testing.T) {
	f, directory := exportFixture(t)
	f.remote.accounts["101"]["credentials"].(map[string]any)["id_token"] = "old-private-id-token"
	var refreshes atomic.Int32
	regenerationTransport(f, func(w http.ResponseWriter, r *http.Request) {
		refreshes.Add(1)
		var input map[string]string
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil || len(input) != 1 || input["refresh_token"] != "rt_export_private" {
			t.Error("regeneration did not submit exactly the reviewed refresh token")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"data":{"access_token":"regenerated-access-private","refresh_token":"rt_regenerated_private","chatgpt_user_id":"user-101","chatgpt_account_id":"workspace-101","email":"owner@example.com"}}`))
	})
	view := previewRegeneration(t, f, "regenerate-owner", accountworkbench.RegenerationInput{AccountIDs: []string{"101"}})
	if refreshes.Load() != 0 || view.Items[0].AccountID != "101" || view.Items[0].UserID != "user-101" || view.Items[0].WorkspaceID != "workspace-101" || view.Items[0].Revision == "" {
		t.Fatalf("read-only scope = %+v, refreshes=%d", view, refreshes.Load())
	}
	task := executeRegeneration(t, f, "regenerate-owner", view)
	if task.Status != "succeeded" || task.Operation != "account-workbench-regenerate" || refreshes.Load() != 1 {
		t.Fatalf("regeneration result = %+v, refreshes=%d", task, refreshes.Load())
	}
	rows, ok := task.Result["items"].([]accountworkbench.ResultItem)
	if !ok || len(rows) != 1 || rows[0].Report["kind"] != accountworkbench.ExportAccounts {
		t.Fatalf("private result metadata = %+v", task.Result)
	}
	artifactID, _ := rows[0].Report["artifact_id"].(string)
	raw, err := os.ReadFile(filepath.Join(directory, artifactID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"regenerated-access-private", "rt_regenerated_private", "0.123456789012345678901"} {
		if !strings.Contains(string(raw), expected) {
			t.Fatalf("private regenerated account did not preserve %s", expected)
		}
	}
	if strings.Contains(string(raw), "old-private-id-token") || strings.Contains(string(raw), "export-access-private") {
		t.Fatal("regenerated account retained stale authorization tokens")
	}
	public, _ := json.Marshal([]any{view, task})
	for _, secret := range []string{"regenerated-access-private", "rt_regenerated_private", "rt_export_private", "test-admin-key"} {
		if strings.Contains(string(public), secret) {
			t.Fatalf("regeneration metadata exposed %s", secret)
		}
	}
	f.remote.mu.Lock()
	defer f.remote.mu.Unlock()
	if f.remote.created != 0 || f.remote.updates != 0 || f.remote.accounts["101"]["credentials"].(map[string]any)["access_token"] != "export-access-private" {
		t.Fatal("private regeneration changed an online account")
	}
}

func TestRegenerationSourceTaskRestoresPrivateArtifactAfterRestartAndRejectsOtherOwner(t *testing.T) {
	f, directory := exportFixture(t)
	view := previewExport(t, f, "101")
	if _, err := f.service.Export(context.Background(), "export-owner", view.ID, true); err != nil {
		t.Fatal(err)
	}
	sourceTask := f.await(t)
	if sourceTask.Status != "succeeded" {
		t.Fatal(sourceTask.Message)
	}
	if err := reloadExportService(t, f, directory); err != nil {
		t.Fatal(err)
	}
	var requests atomic.Int32
	f.service.UseTransport(oauthTransportFunc(func(r *http.Request) (*http.Response, error) {
		requests.Add(1)
		if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/openai/refresh-token") {
			return nil, errors.New("private source must not read or write online accounts")
		}
		return oauthResponse(`{"code":0,"data":{"access_token":"regenerated-access-private","refresh_token":"rt_regenerated_private","chatgpt_user_id":"user-101","chatgpt_account_id":"workspace-101"}}`), nil
	}))
	input := accountworkbench.RegenerationInput{SourceTaskID: sourceTask.ID, Indexes: []int{0}}
	if _, err := f.service.PreviewRegeneration(context.Background(), "another-owner", input); err == nil {
		t.Fatal("another session could reuse private task artifacts")
	}
	preview := previewRegeneration(t, f, "export-owner", input)
	if requests.Load() != 0 || preview.SourceTaskID != sourceTask.ID || preview.Items[0].AccountID != "" {
		t.Fatal("private source preview contacted upstream or inferred an account ID")
	}
	if result := executeRegeneration(t, f, "export-owner", preview); result.Status != "succeeded" || requests.Load() != 1 {
		t.Fatalf("private source regeneration = %+v, requests=%d", result, requests.Load())
	}
	list, err := f.service.Exports(context.Background(), "export-owner")
	if err != nil || len(list) != 2 {
		t.Fatalf("original and regenerated files = %+v, %v", list, err)
	}
}

func TestRegenerationRejectsWrongOfficialIdentityAndNeverReplaysRefreshFailure(t *testing.T) {
	for _, mode := range []string{"user", "workspace", "missing-identity", "business-error", "server-error", "transport-error"} {
		t.Run(mode, func(t *testing.T) {
			f, directory := exportFixture(t)
			view := previewRegeneration(t, f, "regenerate-owner", accountworkbench.RegenerationInput{AccountIDs: []string{"101"}})
			var refreshes atomic.Int32
			f.service.UseTransport(oauthTransportFunc(func(r *http.Request) (*http.Response, error) {
				if r.Method != http.MethodPost {
					response := httptest.NewRecorder()
					f.remote.ServeHTTP(response, r)
					return response.Result(), nil
				}
				refreshes.Add(1)
				switch mode {
				case "transport-error":
					return nil, errors.New("rt_export_private transport failure")
				case "server-error":
					response := oauthResponse(`{"code":1,"message":"rt_export_private"}`)
					response.StatusCode = 503
					return response, nil
				case "business-error":
					return oauthResponse(`{"code":7,"message":"rt_export_private"}`), nil
				}
				user, workspace := "user-101", "workspace-101"
				if mode == "user" {
					user = "other-user"
				}
				if mode == "workspace" {
					workspace = "other-workspace"
				}
				if mode == "missing-identity" {
					user, workspace = "", ""
				}
				payload, _ := json.Marshal(map[string]any{"code": 0, "data": map[string]any{"access_token": "regenerated-access-private", "chatgpt_user_id": user, "chatgpt_account_id": workspace}})
				return oauthResponse(string(payload)), nil
			}))
			task := executeRegeneration(t, f, "regenerate-owner", view)
			rows := task.Result["items"].([]accountworkbench.ResultItem)
			if task.Status != "partial" || len(rows) != 1 || rows[0].Status != "failed" || rows[0].Report != nil || refreshes.Load() != 1 {
				t.Fatalf("failed regeneration = %+v, refreshes=%d", task, refreshes.Load())
			}
			raw, _ := json.Marshal(task)
			if strings.Contains(string(raw), "rt_export_private") || strings.Contains(string(raw), "regenerated-access-private") {
				t.Fatal("refresh failure leaked credential content")
			}
			entries, err := os.ReadDir(directory)
			if err != nil || len(entries) != 0 {
				t.Fatalf("rejected refresh artifacts = %d, %v", len(entries), err)
			}
		})
	}
}

func TestRegenerationKeepsEarlierRotatedTokenWhenLaterAccountFails(t *testing.T) {
	f, directory := exportFixture(t)
	var calls atomic.Int32
	regenerationTransport(f, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if calls.Add(1) == 1 {
			_, _ = w.Write([]byte(`{"code":0,"data":{"access_token":"rotated-first-private","refresh_token":"rt_first_rotated","chatgpt_user_id":"user-101","chatgpt_account_id":"workspace-101"}}`))
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"code":1,"message":"unavailable"}`))
		}
	})
	view := previewRegeneration(t, f, "regenerate-owner", accountworkbench.RegenerationInput{AccountIDs: []string{"101", "102"}})
	task := executeRegeneration(t, f, "regenerate-owner", view)
	rows := task.Result["items"].([]accountworkbench.ResultItem)
	if task.Status != "partial" || calls.Load() != 2 || len(rows) != 2 || rows[0].Status != "succeeded" || rows[1].Status != "failed" {
		t.Fatalf("partial regeneration = %+v, refreshes=%d", task, calls.Load())
	}
	artifactID, _ := rows[0].Report["artifact_id"].(string)
	data, err := os.ReadFile(filepath.Join(directory, artifactID+".json"))
	if err != nil || !strings.Contains(string(data), "rt_first_rotated") {
		t.Fatal("later refresh failure lost previously rotated RT")
	}
}
