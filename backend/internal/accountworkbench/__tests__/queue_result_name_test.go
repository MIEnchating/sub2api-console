package accountworkbench_test

import (
	"context"
	"testing"
)

func TestMixedQueueRecoveryPreservesCompletedAccountNameInFinalPreview(t *testing.T) {
	f := newBatchFixture(t, nil)
	view := startRecoverableMixed(t, f, mixedJSON)
	f.awaitDone(t, view.ID)

	restartMixedFixture(t, f)
	restored := resumeMixedFixture(t, f)
	f.awaitDone(t, restored.ID)
	preview, err := f.service.PreviewWorkbenchRunResult(context.Background(), "owner", restored.ID)
	if err != nil || len(preview.Items) != 1 {
		t.Fatalf("recovered final preview = %+v, %v", preview, err)
	}
	if preview.Items[0].Name != "JSON account" {
		t.Fatalf("recovered account name = %q, want original JSON account", preview.Items[0].Name)
	}
}
