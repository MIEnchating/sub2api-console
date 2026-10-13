package accountworkbench_test

import (
	"context"
	"encoding/json"
	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"strings"
	"testing"
)

func TestImportModelSettingPersistsAndBecomesPreviewDefault(t *testing.T) {
	service, private := fixture(t, `{"data":{"items":[],"total":0}}`)
	ctx := context.Background()
	initial, err := service.ImportModel(ctx)
	if err != nil || initial.Model != "gpt-6-astra" || initial.Revision != 0 {
		t.Fatalf("default=%+v err=%v", initial, err)
	}
	saved, err := service.SaveImportModel(ctx, accountworkbench.ImportModelSettings{Model: " custom-model ", Revision: initial.Revision})
	if err != nil {
		t.Fatal(err)
	}
	reopened := accountworkbench.New(private)
	loaded, err := reopened.ImportModel(ctx)
	if err != nil || loaded.Model != "custom-model" || loaded.Revision != saved.Revision {
		t.Fatalf("saved setting lost: %+v %v", loaded, err)
	}
	owner, _ := previewOwner(t, private)
	for _, model := range []string{"", "explicit-model"} {
		preview, err := service.Preview(ctx, owner, accountworkbench.PreviewInput{Action: "import", Content: "rt_test-input", Model: model})
		if err != nil {
			t.Fatal(err)
		}
		raw, _, err := private.WorkbenchDocument(ctx, "preview:"+owner)
		if err != nil {
			t.Fatal(err)
		}
		var stored struct {
			Settings accountworkbench.PreviewInput `json:"settings"`
		}
		if err := json.Unmarshal(raw, &stored); err != nil {
			t.Fatal(err)
		}
		expected := model
		if expected == "" {
			expected = "custom-model"
		}
		if preview.ID == "" || stored.Settings.Model != expected {
			t.Fatalf("preview model=%s want=%s", stored.Settings.Model, expected)
		}
	}
}

func TestImportModelSettingRejectsInvalidModelsAndStaleWrites(t *testing.T) {
	service, _ := fixture(t, `{}`)
	ctx := context.Background()
	saved, err := service.SaveImportModel(ctx, accountworkbench.ImportModelSettings{Model: "chosen-model"})
	if err != nil {
		t.Fatal(err)
	}
	for _, model := range []string{"", "  ", "model\nheader", strings.Repeat("x", 201)} {
		if _, err := service.SaveImportModel(ctx, accountworkbench.ImportModelSettings{Model: model, Revision: saved.Revision}); err == nil {
			t.Fatalf("accepted invalid model %q", model)
		}
	}
	if _, err := service.SaveImportModel(ctx, accountworkbench.ImportModelSettings{Model: "stale-model"}); err == nil {
		t.Fatal("stale write accepted")
	}
	loaded, err := service.ImportModel(ctx)
	if err != nil || loaded != saved {
		t.Fatalf("rejected write changed setting: %+v %v", loaded, err)
	}
}
