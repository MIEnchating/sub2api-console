package accountworkbench_test

import (
	"context"
	"encoding/json"
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

func TestLocalRegenerationRejectsRemovedAndChangedSourceBeforeRefreshing(t *testing.T) {
	for _, change := range []string{"removed", "changed", "expired"} {
		t.Run(change, func(t *testing.T) {
			f, directory, management := localExportFixture(t)
			source, _ := localRegenerationSource(t, f, importedJSON)
			var calls atomic.Int32
			f.service.UseOAuthTransport(oauthTransportFunc(func(*http.Request) (*http.Response, error) {
				calls.Add(1)
				return localRefreshResponse("user-1", "workspace-1"), nil
			}))
			input := accountworkbench.RegenerationInput{Scope: accountworkbench.ScopeLocalExport, ArtifactID: source.ID}
			if change == "expired" {
				if err := f.service.CloseExports(); err != nil {
					t.Fatal(err)
				}
				path := filepath.Join(directory, source.ID+".meta.json")
				raw, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				var sidecar map[string]any
				if err := json.Unmarshal(raw, &sidecar); err != nil {
					t.Fatal(err)
				}
				metadata := sidecar["metadata"].(map[string]any)
				expired := time.Now().Add(-time.Hour)
				metadata["created_at"] = expired.Add(-24 * time.Hour).UTC().Format(time.RFC3339Nano)
				metadata["expires_at"] = expired.UTC().Format(time.RFC3339Nano)
				raw, err = json.Marshal(sidecar)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, raw, 0600); err != nil {
					t.Fatal(err)
				}
				if err := reloadExportService(t, f, directory); err != nil {
					t.Fatal(err)
				}
				if _, err := f.service.PreviewRegeneration(context.Background(), "local-owner", input); err == nil {
					t.Fatal("expired source could be previewed")
				}
			} else {
				view := previewRegeneration(t, f, "local-owner", input)
				if change == "removed" {
					if err := f.service.DeleteLocalExport(context.Background(), "local-owner", source.ID); err != nil {
						t.Fatal(err)
					}
				} else {
					path := filepath.Join(directory, source.ID+".json")
					raw, err := os.ReadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(path, []byte(strings.Replace(string(raw), "Imported", "Renamed account", 1)), 0600); err != nil {
						t.Fatal(err)
					}
				}
				if result := executeRegeneration(t, f, "local-owner", view); result.Status != "failed" {
					t.Fatalf("changed source result = %+v", result)
				}
			}
			if calls.Load() != 0 || management.Load() != 0 {
				t.Fatal("invalid local source made a network request")
			}
		})
	}
}

func TestLocalRegenerationCancellationAfterValidRefreshPreservesRotationAndStopsRemainingItems(t *testing.T) {
	f, directory, management := localExportFixture(t)
	second := strings.NewReplacer("Imported", "Second", "user-1", "user-2", "workspace-1", "workspace-2", "rt_new_private", "rt_second_private").Replace(importedJSON)
	source, _ := localRegenerationSource(t, f, "["+importedJSON+","+second+"]")
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
	view := previewRegeneration(t, f, "local-owner", accountworkbench.RegenerationInput{Scope: accountworkbench.ScopeLocalExport, ArtifactID: source.ID})
	task, err := f.service.Regenerate(context.Background(), "local-owner", view.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("official refresh did not start")
	}
	if !f.runner.CancelTask(task.ID) {
		t.Fatal("local regeneration task could not be cancelled")
	}
	unblock()
	result := f.await(t)
	rows := result.Result["items"].([]accountworkbench.ResultItem)
	if result.Status != "cancelled" || rows[0].Status != "succeeded" || rows[1].Status != "cancelled" || calls.Load() != 1 || management.Load() != 0 {
		t.Fatalf("cancelled local regeneration = %+v, calls=%d", result, calls.Load())
	}
	id := rows[0].Report["artifact_id"].(string)
	raw, err := os.ReadFile(filepath.Join(directory, id+".json"))
	if err != nil || !strings.Contains(string(raw), "rt_local_rotated_private") {
		t.Fatal("cancellation discarded a valid rotated refresh token")
	}
}
