package accountworkbench_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

func localRegenerationSource(t *testing.T, f *importFixture, content string) (accountworkbench.ExportMetadata, taskstore.Task) {
	t.Helper()
	view, err := f.service.Preview(context.Background(), "local-owner", accountworkbench.PreviewInput{Scope: accountworkbench.ScopeLocalExport, ExportOnly: true, Content: content})
	if err != nil || view.ID == "" {
		t.Fatalf("local source preview = %+v, %v", view, err)
	}
	if _, err := f.service.ExportInput(context.Background(), "local-owner", view.ID, true); err != nil {
		t.Fatal(err)
	}
	task := f.await(t)
	files, err := f.service.LocalExports(context.Background(), "local-owner")
	if err != nil || task.Status != "succeeded" || len(files) != 1 {
		t.Fatalf("local source files = %+v, task=%+v, %v", files, task, err)
	}
	return files[0], task
}

func localRefreshResponse(user, workspace string) *http.Response {
	claims, _ := json.Marshal(map[string]any{"email": "owner@example.com", "https://api.openai.com/auth": map[string]string{"chatgpt_user_id": user, "chatgpt_account_id": workspace}})
	jwt := "e30." + base64.RawURLEncoding.EncodeToString(claims) + ".private-test-signature"
	body, _ := json.Marshal(map[string]any{"access_token": jwt, "refresh_token": "rt_local_rotated_private", "token_type": "Bearer", "expires_in": 3600})
	return oauthResponse(string(body))
}

func TestLocalRegenerationWithoutManagementRefreshesReviewedArtifactOnlyOnce(t *testing.T) {
	f, directory, management := localExportFixture(t)
	source, _ := localRegenerationSource(t, f, importedJSON)
	sourcePath := filepath.Join(directory, source.ID+".json")
	sourceRaw, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sourcePath, []byte(strings.Replace(string(sourceRaw), `"rate_multiplier":1`, `"rate_multiplier":0.123456789012345678901`, 1)), 0600); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	f.service.UseOAuthTransport(oauthTransportFunc(func(request *http.Request) (*http.Response, error) {
		calls.Add(1)
		if request.Method != http.MethodPost || request.URL.String() != "https://auth.openai.com/oauth/token" || request.Header.Get("Authorization") != "" || request.GetBody != nil || !request.Close {
			t.Error("local regeneration did not use isolated one-use official request")
		}
		if err := request.ParseForm(); err != nil || len(request.Form) != 3 || request.Form.Get("grant_type") != "refresh_token" || request.Form.Get("client_id") != "app_EMoamEEZ73f0CkXaXp7hrann" || request.Form.Get("refresh_token") != "rt_new_private" {
			t.Error("official request did not contain exactly the reviewed refresh inputs")
		}
		return localRefreshResponse("user-1", "workspace-1"), nil
	}))
	view := previewRegeneration(t, f, "local-owner", accountworkbench.RegenerationInput{Scope: accountworkbench.ScopeLocalExport, ArtifactID: source.ID})
	if calls.Load() != 0 || management.Load() != 0 || view.Scope != accountworkbench.ScopeLocalExport || view.Target != "" || view.ArtifactID != source.ID || view.Items[0].AccountID != "" {
		t.Fatalf("local read-only preview = %+v", view)
	}
	if _, err := f.service.Regenerate(context.Background(), "local-owner", view.ID, false); err == nil {
		t.Fatal("unconfirmed local refresh started")
	}
	if _, err := f.service.Regenerate(context.Background(), "other-owner", view.ID, true); err == nil {
		t.Fatal("another owner consumed local refresh credentials")
	}
	task := executeRegeneration(t, f, "local-owner", view)
	if task.Status != "succeeded" || calls.Load() != 1 || management.Load() != 0 {
		t.Fatalf("local regeneration = %+v, calls=%d, management=%d", task, calls.Load(), management.Load())
	}
	rows := task.Result["items"].([]accountworkbench.ResultItem)
	id := rows[0].Report["artifact_id"].(string)
	raw, err := os.ReadFile(filepath.Join(directory, id+".json"))
	if err != nil || !strings.Contains(string(raw), "rt_local_rotated_private") || !strings.Contains(string(raw), "0.123456789012345678901") || strings.Contains(string(raw), "access-new-private") {
		t.Fatal("private refreshed artifact lost configuration or retained stale access credentials")
	}
	public, _ := json.Marshal([]any{view, task})
	for _, secret := range []string{"rt_new_private", "rt_local_rotated_private", "private-test-signature"} {
		if strings.Contains(string(public), secret) {
			t.Fatal("local regeneration exposed private authorization")
		}
	}
	if _, err := f.service.Regenerate(context.Background(), "local-owner", view.ID, true); err == nil || calls.Load() != 1 {
		t.Fatal("local regeneration preview could replay a refresh")
	}
}

func TestLocalRegenerationSourceTaskSurvivesRestartAndRemainsOwnerAndScopeBound(t *testing.T) {
	f, directory, management := localExportFixture(t)
	source, task := localRegenerationSource(t, f, importedJSON)
	if err := reloadExportService(t, f, directory); err != nil {
		t.Fatal(err)
	}
	f.service.UseTransport(oauthTransportFunc(func(*http.Request) (*http.Response, error) {
		management.Add(1)
		return nil, errors.New("management forbidden")
	}))
	var calls atomic.Int32
	f.service.UseOAuthTransport(oauthTransportFunc(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return localRefreshResponse("user-1", "workspace-1"), nil
	}))
	input := accountworkbench.RegenerationInput{Scope: accountworkbench.ScopeLocalExport, SourceTaskID: task.ID, Indexes: []int{0}}
	if _, err := f.service.PreviewRegeneration(context.Background(), "other-owner", input); err == nil {
		t.Fatal("another session reused a local source task")
	}
	for _, invalid := range []accountworkbench.RegenerationInput{
		{ArtifactID: source.ID},
		{Scope: accountworkbench.ScopeLocalExport, AccountIDs: []string{"101"}},
		{Scope: accountworkbench.ScopeLocalExport, SourceTaskID: task.ID, ArtifactID: source.ID},
		{Scope: accountworkbench.ScopeLocalExport, SourceTaskID: task.ID, Indexes: []int{1}},
	} {
		if _, err := f.service.PreviewRegeneration(context.Background(), "local-owner", invalid); err == nil {
			t.Fatalf("invalid local source accepted: %+v", invalid)
		}
	}
	view := previewRegeneration(t, f, "local-owner", input)
	if calls.Load() != 0 || management.Load() != 0 {
		t.Fatal("local source task preview performed a network request")
	}
	if result := executeRegeneration(t, f, "local-owner", view); result.Status != "succeeded" || calls.Load() != 1 || management.Load() != 0 {
		t.Fatalf("restarted local source = %+v", result)
	}
}

func TestLocalRegenerationRejectsInvalidOfficialResponsesWithoutReplayingOrWriting(t *testing.T) {
	for _, mode := range []string{"user", "workspace", "missing-identity", "business-error", "transport-error", "redirect"} {
		t.Run(mode, func(t *testing.T) {
			f, directory, management := localExportFixture(t)
			source, _ := localRegenerationSource(t, f, importedJSON)
			var calls atomic.Int32
			f.service.UseOAuthTransport(oauthTransportFunc(func(*http.Request) (*http.Response, error) {
				calls.Add(1)
				switch mode {
				case "user":
					return localRefreshResponse("other-user", "workspace-1"), nil
				case "workspace":
					return localRefreshResponse("user-1", "other-workspace"), nil
				case "missing-identity":
					return localRefreshResponse("", ""), nil
				case "business-error":
					return oauthResponse(`{"error":"rt_new_private"}`), nil
				case "redirect":
					response := oauthResponse("")
					response.StatusCode = http.StatusTemporaryRedirect
					response.Header.Set("Location", "https://example.invalid/token")
					return response, nil
				default:
					return nil, errors.New("rt_new_private transport error")
				}
			}))
			view := previewRegeneration(t, f, "local-owner", accountworkbench.RegenerationInput{Scope: accountworkbench.ScopeLocalExport, ArtifactID: source.ID})
			task := executeRegeneration(t, f, "local-owner", view)
			rows := task.Result["items"].([]accountworkbench.ResultItem)
			if task.Status != "partial" || rows[0].Status != "failed" || rows[0].Report != nil || calls.Load() != 1 || management.Load() != 0 {
				t.Fatalf("rejected official response = %+v, calls=%d", task, calls.Load())
			}
			entries, err := os.ReadDir(directory)
			if err != nil || len(entries) != 2 {
				t.Fatal("rejected local regeneration wrote an artifact")
			}
			raw, _ := json.Marshal(task)
			if strings.Contains(string(raw), "rt_new_private") || strings.Contains(string(raw), "rt_local_rotated_private") {
				t.Fatal("official error exposed credentials in task history")
			}
		})
	}
}
