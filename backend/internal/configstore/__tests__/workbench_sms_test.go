package configstore_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func smsReceiptFixture() configstore.WorkbenchSMSReceipt {
	return configstore.WorkbenchSMSReceipt{OperationID: "purchase-1", Action: "acquire", State: "submitted", Owner: strings.Repeat("a", 64), Target: strings.Repeat("b", 64), TaskID: "task-1", Provider: "smsbower", ConfigHash: strings.Repeat("c", 64)}
}

func TestWorkbenchSMSReceiptRestartRejectsResubmissionAndKeepsUncertainOrder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.sqlite3")
	store, err := configstore.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	value := smsReceiptFixture()
	if err := store.RecordWorkbenchSMS(ctx, value); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = configstore.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.RecordWorkbenchSMS(ctx, value); !errors.Is(err, configstore.ErrWorkbenchSMSReceipt) {
		t.Fatalf("restart allowed duplicate purchase: %v", err)
	}
	value.State, value.RequestID, value.Phone = "uncertain", "order-1", "+17005550123"
	if err := store.RecordWorkbenchSMS(ctx, value); err != nil {
		t.Fatal(err)
	}
	records, err := store.WorkbenchSMSReceipts(ctx, value.Owner, value.Target)
	if err != nil || len(records) != 1 || records[0].State != "uncertain" || records[0].RequestID != "order-1" {
		t.Fatalf("uncertain order receipt lost: %v", err)
	}
	value.State = "confirmed"
	if err := store.RecordWorkbenchSMS(ctx, value); !errors.Is(err, configstore.ErrWorkbenchSMSReceipt) {
		t.Fatalf("terminal receipt overwritten: %v", err)
	}
}

func TestWorkbenchSMSReceiptRejectsDifferentOwnerOrTargetAndDoesNotLeakList(t *testing.T) {
	store, err := configstore.Open(filepath.Join(t.TempDir(), "config.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	value := smsReceiptFixture()
	if err := store.RecordWorkbenchSMS(ctx, value); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"owner", "target"} {
		changed := value
		changed.State = "confirmed"
		if field == "owner" {
			changed.Owner = strings.Repeat("d", 64)
		} else {
			changed.Target = strings.Repeat("d", 64)
		}
		if err := store.RecordWorkbenchSMS(ctx, changed); !errors.Is(err, configstore.ErrWorkbenchSMSReceipt) {
			t.Fatalf("cross-scope outcome accepted: %v", err)
		}
		records, err := store.WorkbenchSMSReceipts(ctx, changed.Owner, changed.Target)
		if err != nil || len(records) != 0 {
			t.Fatal("receipt leaked across scope")
		}
	}
}
