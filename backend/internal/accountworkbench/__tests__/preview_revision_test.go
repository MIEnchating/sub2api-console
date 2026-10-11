package accountworkbench_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
)

func TestPreviewReplacesLargePriorPrivateDocumentAtItsCurrentVersion(t *testing.T) {
	service, store := fixture(t, `{}`)
	owner, _ := previewOwner(t, store)
	key := "preview:" + owner
	previous := json.RawMessage(`{"private":"` + strings.Repeat("old-private", 128<<10) + `"}`)
	if _, err := store.SaveWorkbenchDocument(t.Context(), key, 0, previous); err != nil {
		t.Fatal(err)
	}
	preview, err := service.Preview(t.Context(), owner, accountworkbench.PreviewInput{Action: "export", Content: "rt_new-private"})
	if err != nil {
		t.Fatal(err)
	}
	raw, revision, err := store.WorkbenchDocument(t.Context(), key)
	if err != nil || revision != 2 || preview.Revision != revision {
		t.Fatalf("replacement version=%d preview=%d err=%v", revision, preview.Revision, err)
	}
	if strings.Contains(string(raw), "old-private") || !strings.Contains(string(raw), "rt_new-private") {
		t.Fatal("replacement retained previous credentials or lost new private input")
	}
	public, err := json.Marshal(preview)
	if err != nil || strings.Contains(string(public), "rt_new-private") {
		t.Fatal("replacement leaked private input")
	}
}

func TestMaintenancePreviewReplacementRejectsOldConfirmationAndAcceptsCurrent(t *testing.T) {
	service, store := fixture(t, `{"data":{"items":[],"total":0}}`)
	owner, _ := previewOwner(t, store)
	input := accountworkbench.MaintenanceSettings{IntervalMinutes: 5, CooldownMinutes: 10, GroupIDs: []string{}}
	first, err := service.PreviewMaintenance(t.Context(), owner, input)
	if err != nil {
		t.Fatal(err)
	}
	input.IntervalMinutes = 20
	second, err := service.PreviewMaintenance(t.Context(), owner, input)
	if err != nil {
		t.Fatal(err)
	}
	if second.Revision != first.Revision+1 || second.ID == first.ID {
		t.Fatal("replacement did not advance preview identity and version")
	}
	if _, err := service.ConfigureMaintenance(t.Context(), owner, accountworkbench.RunConfirmation{ID: first.ID, Revision: first.Revision}); err == nil {
		t.Fatal("superseded preview accepted")
	}
	configured, err := service.ConfigureMaintenance(t.Context(), owner, accountworkbench.RunConfirmation{ID: second.ID, Revision: second.Revision})
	if err != nil || configured.IntervalMinutes != 20 {
		t.Fatalf("current preview interval=%d err=%v", configured.IntervalMinutes, err)
	}
}
