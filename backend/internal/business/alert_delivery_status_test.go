package business

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestUncertainNotificationDeliveryRecordsFailureRequiringConfirmation(t *testing.T) {
	store := openPolicyStore(t)
	ctx := context.Background()
	record, err := store.RecordAlertEvaluation(ctx, time.Now().UTC().Format(time.RFC3339Nano),
		AlertEvidenceResult{Findings: 1},
		AlertDeliveryResult{Configured: true, Attempted: 1, Uncertain: 1},
	)
	if err != nil {
		t.Fatal(err)
	}
	if record.Status != "failed" || !strings.Contains(record.Summary, "待确认 1 项") {
		t.Fatalf("uncertain delivery was not surfaced as requiring confirmation: %#v", record)
	}
	runs, err := store.RunRecords(ctx, nil)
	if err != nil || len(runs) != 1 || runs[0].Status == nil || *runs[0].Status != "failed" {
		t.Fatalf("uncertain delivery was not persisted as a failed run: runs=%#v err=%v", runs, err)
	}
}
