package accountworkbench_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
)

func batchExportFixture(t *testing.T) (*importFixture, accountworkbench.ExportPreviewInput) {
	t.Helper()
	f, _ := exportFixture(t)
	preview, err := f.service.Preview(context.Background(), "owner", accountworkbench.PreviewInput{Content: importedJSON})
	if err != nil {
		t.Fatal(err)
	}
	task, err := f.service.Import(context.Background(), "owner", preview.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	f.await(t)
	raw, _ := json.Marshal(map[string]string{"source_task_id": task.ID})
	var input accountworkbench.ExportPreviewInput
	if err := json.Unmarshal(raw, &input); err != nil {
		t.Fatal(err)
	}
	return f, input
}

func TestBatchExportIncludesOnlyAccountsBoundToOriginalImport(t *testing.T) {
	f, input := batchExportFixture(t)
	preview, err := f.service.PreviewExport(context.Background(), "owner", input)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Items) != 1 || preview.Items[0].AccountID != "101" {
		t.Fatalf("wrong batch export range: %+v", preview.Items)
	}
}

func TestBatchExportRejectsChangedOfficialIdentity(t *testing.T) {
	f, input := batchExportFixture(t)
	f.remote.accounts["101"]["credentials"].(map[string]any)["chatgpt_user_id"] = "different-user"
	if _, err := f.service.PreviewExport(context.Background(), "owner", input); err == nil {
		t.Fatal("batch export accepted changed account identity")
	}
}

func TestBatchExportRejectsExplicitAccountRangeAlongsideSourceTask(t *testing.T) {
	f, input := batchExportFixture(t)
	input.AccountIDs = []string{"102"}
	if _, err := f.service.PreviewExport(context.Background(), "owner", input); err == nil {
		t.Fatal("batch export accepted account outside original task")
	}
}
