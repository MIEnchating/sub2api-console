package accountworkbench_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/MIEnchating/sub2api-console/backend/internal/targetguard"
)

func TestInputConversionRequiresOwnerAndConfirmationAndConsumesPreviewOnce(t *testing.T) {
	f, _ := exportFixture(t)
	view := previewConversion(t, f, accountworkbench.PreviewInput{Content: importedJSON})
	if _, err := f.service.ExportInput(context.Background(), "convert-owner", view.ID, false); err == nil {
		t.Fatal("unconfirmed conversion created a task")
	}
	if _, err := f.service.ExportInput(context.Background(), "other-owner", view.ID, true); !errors.Is(err, accountworkbench.ErrPreview) {
		t.Fatalf("other session conversion = %v", err)
	}
	if _, err := f.service.Import(context.Background(), "convert-owner", view.ID, true); err == nil {
		t.Fatal("conversion-only preview could write online accounts")
	}
	if _, err := f.service.ExportInput(context.Background(), "convert-owner", view.ID, true); err != nil {
		t.Fatalf("rejected attempts consumed the authorized preview: %v", err)
	}
	if task := f.await(t); task.Status != "succeeded" {
		t.Fatalf("conversion = %+v", task)
	}
	if _, err := f.service.ExportInput(context.Background(), "convert-owner", view.ID, true); !errors.Is(err, accountworkbench.ErrPreview) {
		t.Fatalf("consumed conversion preview = %v", err)
	}
	artifacts, err := f.service.Exports(context.Background(), "other-owner")
	if err != nil || len(artifacts) != 0 {
		t.Fatalf("other session accessed conversion metadata = %+v, %v", artifacts, err)
	}
	f.remote.mu.Lock()
	defer f.remote.mu.Unlock()
	if f.remote.created != 0 || f.remote.updates != 0 || f.remote.refreshes != 0 {
		t.Fatal("conversion-only scope modified an online account")
	}
}

func TestInputConversionAcceptsNormalImportPreviewWithoutChangingOnlineAccounts(t *testing.T) {
	f, _ := exportFixture(t)
	f.existing()
	view := f.preview(t, importedJSON, true)
	if view.ExportOnly || !view.Items[0].Duplicate {
		t.Fatal("fixture did not produce an existing-account import preview")
	}
	if _, err := f.service.ExportInput(context.Background(), "owner", view.ID, true); err != nil {
		t.Fatal(err)
	}
	if task := f.await(t); task.Status != "succeeded" {
		t.Fatalf("conversion = %+v", task)
	}
	f.remote.mu.Lock()
	defer f.remote.mu.Unlock()
	if f.remote.created != 0 || f.remote.updates != 0 || f.remote.refreshes != 0 || f.remote.accounts["101"]["name"] != "Original" {
		t.Fatal("converting an import preview changed the matching online account")
	}
}

func TestInputConversionRejectsDiscardedAndReplacedPreviews(t *testing.T) {
	f, _ := exportFixture(t)
	first := previewConversion(t, f, accountworkbench.PreviewInput{Content: importedJSON})
	second := previewConversion(t, f, accountworkbench.PreviewInput{Content: importedJSON})
	if _, err := f.service.ExportInput(context.Background(), "convert-owner", first.ID, true); !errors.Is(err, accountworkbench.ErrPreview) {
		t.Fatalf("replaced preview = %v", err)
	}
	f.service.DeletePreview("convert-owner", second.ID)
	if _, err := f.service.ExportInput(context.Background(), "convert-owner", second.ID, true); !errors.Is(err, accountworkbench.ErrPreview) {
		t.Fatalf("discarded preview = %v", err)
	}
}

func TestInputConversionAfterPreviewExpiryRejectsTheExpiredCredentials(t *testing.T) {
	f, directory := exportFixture(t)
	// Only the preview timer lives in the virtual clock. SQLite and HTTP fixture
	// lifetimes stay outside the bubble and no conversion performs network I/O.
	synctest.Test(t, func(t *testing.T) {
		view := previewConversion(t, f, accountworkbench.PreviewInput{Content: importedJSON})
		time.Sleep(10*time.Minute + time.Second)
		if _, err := f.service.ExportInput(context.Background(), "convert-owner", view.ID, true); !errors.Is(err, accountworkbench.ErrPreview) {
			t.Fatalf("expired preview = %v", err)
		}
	})
	entries, err := os.ReadDir(directory)
	if err != nil || len(entries) != 0 {
		t.Fatalf("expired conversion artifacts = %d, %v", len(entries), err)
	}
}

func TestInputConversionRejectsTargetCredentialChangeBeforeTaskCreation(t *testing.T) {
	f, directory := exportFixture(t)
	view := previewConversion(t, f, accountworkbench.PreviewInput{Content: importedJSON})
	if err := f.private.ConfigureTarget(context.Background(), f.server.URL, "replacement-test-key", 3); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.ExportInput(context.Background(), "convert-owner", view.ID, true); !errors.Is(err, targetguard.ErrChanged) {
		t.Fatalf("changed target conversion = %v", err)
	}
	entries, err := os.ReadDir(directory)
	if err != nil || len(entries) != 0 {
		t.Fatalf("changed target artifacts = %d, %v", len(entries), err)
	}
}

func TestInputConversionWhenQueuedScopeChangesFailsWithoutWritingAFile(t *testing.T) {
	for _, change := range []string{"target", "template-update", "template-delete"} {
		t.Run(change, func(t *testing.T) {
			f := newImportFixture(t, nil)
			gate := make(chan struct{})
			var once sync.Once
			release := func() { once.Do(func() { close(gate) }) }
			t.Cleanup(release)
			f.service = accountworkbench.New(f.private, f.tasks, f.business, nil, &batchRunner{group: f.runner, gate: gate, done: make(chan string, 1)})
			directory := filepath.Join(t.TempDir(), "conversion")
			if err := f.service.UseExportDirectory(directory); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = f.service.CloseExports() })
			template, err := f.service.SaveTemplate(context.Background(), "", accountworkbench.TemplateInput{Name: "Reviewed"})
			if err != nil {
				t.Fatal(err)
			}
			view := previewConversion(t, f, accountworkbench.PreviewInput{Content: importedJSON, TemplateID: template.ID})
			if _, err := f.service.ExportInput(context.Background(), "convert-owner", view.ID, true); err != nil {
				t.Fatal(err)
			}
			switch change {
			case "target":
				err = f.private.ConfigureTarget(context.Background(), f.server.URL, "replacement-test-key", 3)
			case "template-update":
				_, err = f.service.SaveTemplate(context.Background(), template.ID, accountworkbench.TemplateInput{Name: "Changed", Revision: template.Revision})
			case "template-delete":
				err = f.service.DeleteTemplate(context.Background(), template.ID, template.Revision)
			}
			if err != nil {
				t.Fatal(err)
			}
			release()
			task := f.await(t)
			rows, ok := task.Result["items"].([]accountworkbench.ResultItem)
			if task.Status != "failed" || !ok || len(rows) != 1 || rows[0].Status != "failed" || rows[0].Report != nil {
				t.Fatalf("changed queued scope = %+v", task)
			}
			entries, err := os.ReadDir(directory)
			if err != nil || len(entries) != 0 {
				t.Fatalf("changed queued scope artifacts = %d, %v", len(entries), err)
			}
		})
	}
}

func TestInputConversionRejectsRetryPreviewAndPreservesItsImportAction(t *testing.T) {
	f, directory := exportFixture(t)
	task := f.start(t, f.preview(t, importedJSON, false))
	f.await(t)
	view, err := f.service.RetryPreview(context.Background(), "owner", accountworkbench.RetryPreviewInput{TaskID: task.ID})
	if err != nil || view.ID == "" {
		t.Fatalf("retry preview = %+v, %v", view, err)
	}
	if _, err := f.service.ExportInput(context.Background(), "owner", view.ID, true); err == nil || !strings.Contains(err.Error(), "重新处理预览") {
		t.Fatalf("retry conversion = %v", err)
	}
	if _, err := f.service.Import(context.Background(), "owner", view.ID, true); err != nil {
		t.Fatalf("conversion rejection consumed retry preview: %v", err)
	}
	f.await(t)
	entries, err := os.ReadDir(directory)
	if err != nil || len(entries) != 0 {
		t.Fatalf("retry conversion artifacts = %d, %v", len(entries), err)
	}
}
