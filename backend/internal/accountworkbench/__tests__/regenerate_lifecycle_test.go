package accountworkbench_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

func TestRegenerationRequiresConfirmationOwnerCorrectKindAndUnconsumedPreview(t *testing.T) {
	f, _ := exportFixture(t)
	regenerationTransport(f, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"code":0,"data":{"access_token":"regenerated-access-private","chatgpt_user_id":"user-101","chatgpt_account_id":"workspace-101"}}`))
	})
	view := previewRegeneration(t, f, "regenerate-owner", accountworkbench.RegenerationInput{AccountIDs: []string{"101"}})
	if _, err := f.service.Regenerate(context.Background(), "regenerate-owner", view.ID, false); err == nil {
		t.Fatal("unconfirmed regeneration started")
	}
	if _, err := f.service.Regenerate(context.Background(), "another-owner", view.ID, true); !errors.Is(err, accountworkbench.ErrExportPreview) {
		t.Fatalf("other session = %v", err)
	}
	if _, err := f.service.Export(context.Background(), "regenerate-owner", view.ID, true); !errors.Is(err, accountworkbench.ErrExportPreview) {
		t.Fatalf("ordinary export accepted regeneration preview = %v", err)
	}
	f.service.DeleteExportPreview("another-owner", view.ID)
	if task := executeRegeneration(t, f, "regenerate-owner", view); task.Status != "succeeded" {
		t.Fatal("rejected attempts consumed the authorized preview")
	}
	if _, err := f.service.Regenerate(context.Background(), "regenerate-owner", view.ID, true); !errors.Is(err, accountworkbench.ErrExportPreview) {
		t.Fatalf("consumed preview = %v", err)
	}
}

func TestRegenerationRejectsDiscardedReplacedAndExpiredPreviewWithoutRefreshing(t *testing.T) {
	f, directory := exportFixture(t)
	var refreshes atomic.Int32
	regenerationTransport(f, func(w http.ResponseWriter, _ *http.Request) { refreshes.Add(1); w.WriteHeader(503) })
	input := accountworkbench.RegenerationInput{AccountIDs: []string{"101"}}
	first := previewRegeneration(t, f, "regenerate-owner", input)
	second := previewRegeneration(t, f, "regenerate-owner", input)
	if _, err := f.service.Regenerate(context.Background(), "regenerate-owner", first.ID, true); !errors.Is(err, accountworkbench.ErrExportPreview) {
		t.Fatalf("replaced preview = %v", err)
	}
	f.service.DeleteExportPreview("regenerate-owner", second.ID)
	if _, err := f.service.Regenerate(context.Background(), "regenerate-owner", second.ID, true); !errors.Is(err, accountworkbench.ErrExportPreview) {
		t.Fatalf("discarded preview = %v", err)
	}
	synctest.Test(t, func(t *testing.T) {
		view := previewRegeneration(t, f, "regenerate-owner", input)
		time.Sleep(10*time.Minute + time.Second)
		if _, err := f.service.Regenerate(context.Background(), "regenerate-owner", view.ID, true); !errors.Is(err, accountworkbench.ErrExportPreview) {
			t.Fatalf("expired preview = %v", err)
		}
	})
	entries, err := os.ReadDir(directory)
	if err != nil || len(entries) != 0 || refreshes.Load() != 0 {
		t.Fatalf("expired scope wrote files or refreshed: files=%d, refreshes=%d, err=%v", len(entries), refreshes.Load(), err)
	}
}

func TestRegenerationRejectsQueuedAccountTargetAndHistoryChangesBeforeRefresh(t *testing.T) {
	for _, change := range []string{"account-config", "account-identity", "target", "deleted-task", "deleted-artifact", "closed-directory"} {
		t.Run(change, func(t *testing.T) {
			f, directory := exportFixture(t)
			input := accountworkbench.RegenerationInput{AccountIDs: []string{"101"}}
			var source taskstore.Task
			var artifact accountworkbench.ExportMetadata
			if change == "deleted-task" || change == "deleted-artifact" {
				view := previewExport(t, f, "101")
				if _, err := f.service.Export(context.Background(), "export-owner", view.ID, true); err != nil {
					t.Fatal(err)
				}
				source = f.await(t)
				list, err := f.service.Exports(context.Background(), "export-owner")
				if err != nil || len(list) != 1 {
					t.Fatalf("source artifact = %+v, %v", list, err)
				}
				artifact = list[0]
				input = accountworkbench.RegenerationInput{SourceTaskID: source.ID}
			}
			if err := f.service.CloseExports(); err != nil {
				t.Fatal(err)
			}
			gate := make(chan struct{})
			var once sync.Once
			release := func() { once.Do(func() { close(gate) }) }
			t.Cleanup(release)
			f.service = accountworkbench.New(f.private, f.tasks, f.business, nil, &batchRunner{group: f.runner, gate: gate, done: make(chan string, 1)})
			if err := f.service.UseExportDirectory(directory); err != nil {
				t.Fatal(err)
			}
			var refreshes atomic.Int32
			regenerationTransport(f, func(w http.ResponseWriter, _ *http.Request) { refreshes.Add(1); w.WriteHeader(503) })
			view := previewRegeneration(t, f, "export-owner", input)
			if _, err := f.service.Regenerate(context.Background(), "export-owner", view.ID, true); err != nil {
				t.Fatal(err)
			}
			switch change {
			case "account-config":
				f.remote.accounts["101"]["name"] = "changed account"
			case "account-identity":
				f.remote.accounts["101"]["credentials"].(map[string]any)["chatgpt_user_id"] = "other-user"
			case "target":
				if err := f.private.ConfigureTarget(context.Background(), f.server.URL, "changed-target-key", 3); err != nil {
					t.Fatal(err)
				}
			case "deleted-task":
				if err := f.service.DeleteHistory(context.Background(), accountworkbench.HistoryActionInput{Confirmed: true, Items: []taskstore.HistorySelection{{ID: source.ID, UpdatedAt: source.UpdatedAt}}}); err != nil {
					t.Fatal(err)
				}
			case "deleted-artifact":
				if err := f.service.DeleteExport(context.Background(), "export-owner", artifact.ID); err != nil {
					t.Fatal(err)
				}
			case "closed-directory":
				if err := f.service.CloseExports(); err != nil {
					t.Fatal(err)
				}
			}
			release()
			result := f.await(t)
			rows := result.Result["items"].([]accountworkbench.ResultItem)
			if result.Status != "failed" || len(rows) != 1 || rows[0].Status != "failed" || rows[0].Report != nil || refreshes.Load() != 0 {
				t.Fatalf("changed queued source = %+v, refreshes=%d", result, refreshes.Load())
			}
		})
	}
}

func TestRegenerationSourceIndexesPreserveSuccessfulOriginalRowAfterPartialResult(t *testing.T) {
	f, _ := exportFixture(t)
	var calls atomic.Int32
	regenerationTransport(f, func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"code":1}`))
			return
		}
		_, _ = w.Write([]byte(`{"code":0,"data":{"access_token":"second-private","refresh_token":"rt_second_private","chatgpt_user_id":"user-102","chatgpt_account_id":"workspace-102"}}`))
	})
	view := previewRegeneration(t, f, "regenerate-owner", accountworkbench.RegenerationInput{AccountIDs: []string{"101", "102"}})
	task := executeRegeneration(t, f, "regenerate-owner", view)
	if task.Status != "partial" {
		t.Fatal("fixture did not produce a partial regeneration result")
	}
	if _, err := f.service.PreviewRegeneration(context.Background(), "regenerate-owner", accountworkbench.RegenerationInput{SourceTaskID: task.ID, Indexes: []int{0}}); err == nil {
		t.Fatal("failed original index matched another row's successful artifact")
	}
	preview := previewRegeneration(t, f, "regenerate-owner", accountworkbench.RegenerationInput{SourceTaskID: task.ID, Indexes: []int{1}})
	if len(preview.Items) != 1 || preview.Items[0].Index != 1 || preview.Items[0].UserID != "user-102" {
		t.Fatalf("regenerated task index = %+v", preview.Items)
	}
}

func TestRegenerationCancellationDuringLaterRefreshPreservesCompletedPrivateArtifact(t *testing.T) {
	f, directory := exportFixture(t)
	started := make(chan struct{})
	var calls atomic.Int32
	f.service.UseTransport(oauthTransportFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodPost {
			response := httptest.NewRecorder()
			f.remote.ServeHTTP(response, r)
			return response.Result(), nil
		}
		if calls.Add(1) == 1 {
			return oauthResponse(`{"code":0,"data":{"access_token":"first-private","refresh_token":"rt_first_private","chatgpt_user_id":"user-101","chatgpt_account_id":"workspace-101"}}`), nil
		}
		close(started)
		<-r.Context().Done()
		return nil, r.Context().Err()
	}))
	view := previewRegeneration(t, f, "regenerate-owner", accountworkbench.RegenerationInput{AccountIDs: []string{"101", "102"}})
	task, err := f.service.Regenerate(context.Background(), "regenerate-owner", view.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("second refresh did not start")
	}
	if !f.runner.CancelTask(task.ID) {
		t.Fatal("regeneration task could not be cancelled")
	}
	result := f.await(t)
	rows := result.Result["items"].([]accountworkbench.ResultItem)
	if result.Status != "cancelled" || rows[0].Status != "succeeded" || rows[1].Status != "cancelled" || calls.Load() != 2 {
		t.Fatalf("cancelled regeneration = %+v, refreshes=%d", result, calls.Load())
	}
	entries, err := os.ReadDir(directory)
	if err != nil || len(entries) != 2 {
		t.Fatal("cancellation discarded the completed account artifact")
	}
	public, _ := json.Marshal(result)
	if strings.Contains(string(public), "rt_first_private") {
		t.Fatal("cancellation response exposed a rotated token")
	}
}

func TestRegenerationCancellationAfterRefreshReturnsStillRetainsRotatedCredentials(t *testing.T) {
	f, directory := exportFixture(t)
	started, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	f.service.UseTransport(oauthTransportFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodPost {
			response := httptest.NewRecorder()
			f.remote.ServeHTTP(response, r)
			return response.Result(), nil
		}
		close(started)
		<-release
		return oauthResponse(`{"code":0,"data":{"access_token":"completed-private","refresh_token":"rt_completed_private","chatgpt_user_id":"user-101","chatgpt_account_id":"workspace-101"}}`), nil
	}))
	view := previewRegeneration(t, f, "regenerate-owner", accountworkbench.RegenerationInput{AccountIDs: []string{"101"}})
	task, err := f.service.Regenerate(context.Background(), "regenerate-owner", view.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("refresh did not start")
	}
	if !f.runner.CancelTask(task.ID) {
		t.Fatal("regeneration task could not be cancelled")
	}
	unblock()
	result := f.await(t)
	rows := result.Result["items"].([]accountworkbench.ResultItem)
	if result.Status != "cancelled" || len(rows) != 1 || rows[0].Status != "succeeded" || rows[0].Report == nil {
		t.Fatalf("completed rotation after cancellation = %+v", result)
	}
	entries, err := os.ReadDir(directory)
	if err != nil || len(entries) != 2 {
		t.Fatal("confirmed rotated credentials were discarded due to cancellation")
	}
}
