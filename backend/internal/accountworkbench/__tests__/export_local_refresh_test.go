package accountworkbench_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
)

func TestLocalRefreshInputWaitsForConfirmationAndPersistsEachSuccessfulRotation(t *testing.T) {
	f, directory, management := localExportFixture(t)
	var calls atomic.Int32
	f.service.UseOAuthTransport(oauthTransportFunc(func(request *http.Request) (*http.Response, error) {
		calls.Add(1)
		if request.URL.String() != "https://auth.openai.com/oauth/token" || request.Header.Get("Authorization") != "" || request.GetBody != nil {
			t.Error("local RT conversion used a non-official or replayable request")
		}
		if err := request.ParseForm(); err != nil || request.Form.Get("refresh_token") != "rt_bare_private" || request.Form.Get("grant_type") != "refresh_token" {
			t.Error("local RT conversion did not submit the reviewed token")
		}
		return localRefreshResponse("user-bare", "workspace-bare"), nil
	}))
	view, err := f.service.Preview(context.Background(), "local-owner", accountworkbench.PreviewInput{Scope: accountworkbench.ScopeLocalExport, ExportOnly: true, Content: `["rt_bare_private",` + importedJSON + `]`})
	if err != nil || view.ID == "" || len(view.Items) != 2 || !view.Items[0].RefreshRequired || view.Items[1].RefreshRequired || calls.Load() != 0 {
		t.Fatalf("read-only local RT preview = %+v, %v", view, err)
	}
	if _, err := f.service.ExportInput(context.Background(), "local-owner", view.ID, false); err == nil || calls.Load() != 0 {
		t.Fatal("local RT conversion refreshed before confirmation")
	}
	if _, err := f.service.ExportInput(context.Background(), "local-owner", view.ID, true); err != nil {
		t.Fatal(err)
	}
	task := f.await(t)
	rows := task.Result["items"].([]accountworkbench.ResultItem)
	if task.Status != "succeeded" || len(rows) != 2 || calls.Load() != 1 || management.Load() != 0 {
		t.Fatalf("local RT conversion = %+v", task)
	}
	for index, row := range rows {
		if row.Index != index || row.Status != "succeeded" || row.Report["scope"] != accountworkbench.ScopeLocalExport {
			t.Fatalf("local RT item lost original index or scope: %+v", row)
		}
		if _, err := os.Stat(filepath.Join(directory, row.Report["artifact_id"].(string)+".json")); err != nil {
			t.Fatal(err)
		}
	}
	public, _ := json.Marshal([]any{view, task})
	if strings.Contains(string(public), "rt_bare_private") || strings.Contains(string(public), "rt_local_rotated_private") {
		t.Fatal("local RT conversion exposed credentials in public metadata")
	}
	if _, err := f.service.ExportInput(context.Background(), "local-owner", view.ID, true); err == nil || calls.Load() != 1 {
		t.Fatal("local RT conversion preview could replay")
	}
}

func TestLocalRefreshInputFailureIsNotRetriedAndLaterJSONRetainsSourceIndex(t *testing.T) {
	for _, mode := range []string{"transport", "missing-identity", "changed-identity"} {
		t.Run(mode, func(t *testing.T) {
			f, _, management := localExportFixture(t)
			var calls atomic.Int32
			f.service.UseOAuthTransport(oauthTransportFunc(func(*http.Request) (*http.Response, error) {
				calls.Add(1)
				if mode == "transport" {
					return nil, errors.New("rt_failure_private network error")
				}
				if mode == "missing-identity" {
					return localRefreshResponse("", ""), nil
				}
				return localRefreshResponse("unexpected-user", "workspace-1"), nil
			}))
			content := `"rt_failure_private"`
			if mode == "changed-identity" {
				content = `{"credentials":{"refresh_token":"rt_failure_private","chatgpt_user_id":"expected-user","chatgpt_account_id":"workspace-1"}}`
			}
			view, err := f.service.Preview(context.Background(), "local-owner", accountworkbench.PreviewInput{Scope: accountworkbench.ScopeLocalExport, ExportOnly: true, Content: "[" + content + "," + importedJSON + "]"})
			if err != nil || view.ID == "" {
				t.Fatalf("local failure preview = %+v, %v", view, err)
			}
			if _, err := f.service.ExportInput(context.Background(), "local-owner", view.ID, true); err != nil {
				t.Fatal(err)
			}
			task := f.await(t)
			rows := task.Result["items"].([]accountworkbench.ResultItem)
			if task.Status != "partial" || rows[0].Status != "failed" || rows[1].Status != "succeeded" || calls.Load() != 1 || management.Load() != 0 {
				t.Fatalf("failed local refresh = %+v", task)
			}
			regeneration := previewRegeneration(t, f, "local-owner", accountworkbench.RegenerationInput{Scope: accountworkbench.ScopeLocalExport, SourceTaskID: task.ID, Indexes: []int{1}})
			if len(regeneration.Items) != 1 || regeneration.Items[0].Index != 1 || regeneration.Items[0].UserID != "user-1" {
				t.Fatal("local conversion task source lost the surviving original index")
			}
			public, _ := json.Marshal(task)
			if strings.Contains(string(public), "rt_failure_private") {
				t.Fatal("failed official refresh leaked original RT")
			}
		})
	}
}

func TestLocalRefreshInputCancelledAfterOfficialResponseKeepsRotationAndSkipsNextRT(t *testing.T) {
	f, directory, _ := localExportFixture(t)
	started, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	var calls atomic.Int32
	f.service.UseOAuthTransport(oauthTransportFunc(func(*http.Request) (*http.Response, error) {
		if calls.Add(1) == 1 {
			close(started)
		}
		<-release
		return localRefreshResponse("user-1", "workspace-1"), nil
	}))
	view, err := f.service.Preview(context.Background(), "local-owner", accountworkbench.PreviewInput{Scope: accountworkbench.ScopeLocalExport, ExportOnly: true, Content: "rt_first_private\nrt_second_private"})
	if err != nil || view.ID == "" {
		t.Fatalf("local RT preview = %+v, %v", view, err)
	}
	task, err := f.service.ExportInput(context.Background(), "local-owner", view.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("local RT refresh did not start")
	}
	if !f.runner.CancelTask(task.ID) {
		t.Fatal("local conversion task could not be cancelled")
	}
	unblock()
	result := f.await(t)
	rows := result.Result["items"].([]accountworkbench.ResultItem)
	if result.Status != "cancelled" || rows[0].Status != "succeeded" || rows[1].Status != "cancelled" || calls.Load() != 1 {
		t.Fatalf("cancelled local conversion = %+v", result)
	}
	raw, err := os.ReadFile(filepath.Join(directory, rows[0].Report["artifact_id"].(string)+".json"))
	if err != nil || !strings.Contains(string(raw), "rt_local_rotated_private") {
		t.Fatal("cancelled local conversion lost rotated credentials")
	}
}
