package accountworkbench_test

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func TestRunPreservesReturnedRotationEvenWhenResponseMetadataIsIncomplete(t *testing.T) {
	service, store := fixture(t, `{}`)
	owner, _ := previewOwner(t, store)
	runner, _ := runTasks(t, service)
	requests := 0
	service.UseOfficialTransport(transportFunc(func(*http.Request) (*http.Response, error) {
		requests++
		return upstreamResponse(map[string]any{"refresh_token": "rt_returned-rotation"}), nil
	}))
	ctx := context.Background()
	preview, err := service.Preview(ctx, owner, accountworkbench.PreviewInput{Action: "export", Content: "rt_original"})
	if err != nil {
		t.Fatal(err)
	}
	run, err := service.Start(ctx, owner, accountworkbench.RunConfirmation{ID: preview.ID, Revision: preview.Revision})
	if err != nil {
		t.Fatal(err)
	}
	awaitRun(t, runner)
	private, _, err := store.WorkbenchDocument(ctx, "run:"+owner+":"+run.ID)
	if err != nil || !strings.Contains(string(private), "rt_returned-rotation") || requests != 1 {
		t.Fatal("received rotation was lost or replayed")
	}
	run, err = service.Run(ctx, owner, run.ID)
	if err != nil || run.Status == "completed" {
		t.Fatal("incomplete credentials reported successful")
	}
	raw, _ := json.Marshal(run)
	if strings.Contains(string(raw), "rt_returned-rotation") {
		t.Fatal("rotation exposed publicly")
	}
}

func completedExport(t *testing.T) (*accountworkbench.Service, *configstore.Store, string, string, accountworkbench.Run, string) {
	t.Helper()
	service, store := fixture(t, `{}`)
	owner, token := previewOwner(t, store)
	runner, tasks := runTasks(t, service)
	directory := t.TempDir()
	service.UseExecution(tasks, runner, nil, directory)
	input := signedRunInput(t, service)
	preview, err := service.Preview(context.Background(), owner, accountworkbench.PreviewInput{Action: "export", Content: input})
	if err != nil {
		t.Fatal(err)
	}
	run, err := service.Start(context.Background(), owner, accountworkbench.RunConfirmation{ID: preview.ID, Revision: preview.Revision})
	if err != nil {
		t.Fatal(err)
	}
	awaitRun(t, runner)
	run, err = service.Run(context.Background(), owner, run.ID)
	if err != nil || run.Status != "completed" {
		t.Fatalf("export failed: %v %+v", err, run)
	}
	return service, store, owner, token, run, directory
}

func TestRepeatedExportReusesPrivateFileForUnchangedRun(t *testing.T) {
	service, _, owner, _, run, directory := completedExport(t)
	confirmation := accountworkbench.RunConfirmation{ID: run.ID, Revision: run.Revision}
	first, err := service.Export(context.Background(), owner, confirmation)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Export(context.Background(), owner, confirmation)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID {
		t.Fatal("unchanged run created another private file")
	}
	entries, err := os.ReadDir(directory)
	if err != nil || len(entries) != 1 {
		t.Fatal("export retained duplicate files")
	}
	info, err := os.Stat(filepath.Join(directory, first.ID+".json"))
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("private file permissions changed")
	}
}

func TestRecoveryRemovesExpiredOrRevokedPrivateData(t *testing.T) {
	for _, reason := range []string{"expired", "revoked"} {
		t.Run(reason, func(t *testing.T) {
			service, store, owner, token, run, directory := completedExport(t)
			ctx := context.Background()
			if _, err := service.Export(ctx, owner, accountworkbench.RunConfirmation{ID: run.ID, Revision: run.Revision}); err != nil {
				t.Fatal(err)
			}
			if _, err := service.Preview(ctx, owner, accountworkbench.PreviewInput{Action: "export", Content: "rt_pending-private"}); err != nil {
				t.Fatal(err)
			}
			if reason == "revoked" {
				if err := store.RevokeSession(ctx, token); err != nil {
					t.Fatal(err)
				}
			} else {
				for _, key := range []string{"run:" + owner + ":" + run.ID, "preview:" + owner} {
					raw, rev, err := store.WorkbenchDocument(ctx, key)
					if err != nil {
						t.Fatal(err)
					}
					var value map[string]any
					if err = json.Unmarshal(raw, &value); err != nil {
						t.Fatal(err)
					}
					value["public"].(map[string]any)["expires_at"] = time.Now().Add(-time.Minute).UTC().Format(time.RFC3339Nano)
					raw, _ = json.Marshal(value)
					if _, err = store.SaveWorkbenchDocument(ctx, key, rev, raw); err != nil {
						t.Fatal(err)
					}
				}
			}
			if err := service.Recover(ctx); err != nil {
				t.Fatal(err)
			}
			private, _, err := store.WorkbenchDocument(ctx, "run:"+owner+":"+run.ID)
			if err != nil || strings.Contains(string(private), "rt_run-private") || strings.Contains(string(private), "access_token") {
				t.Fatal("terminal run retained credentials after expiry or logout")
			}
			preview, _, err := store.WorkbenchDocument(ctx, "preview:"+owner)
			if err != nil || len(preview) > 0 {
				t.Fatal("expired or revoked preview retained")
			}
			entries, err := os.ReadDir(directory)
			if err != nil || len(entries) > 0 {
				t.Fatal("expired or revoked private artifact retained")
			}
		})
	}
}
