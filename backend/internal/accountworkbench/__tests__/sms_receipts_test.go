package accountworkbench_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
)

func TestSMSReceiptExplicitInspectionUsesOriginalOrderAndReturnsNoCode(t *testing.T) {
	f, browser := newAssistFixture(t, "phone", "sms_code")
	ticks := make(chan time.Time)
	f.service.UseOAuthAssistTicks(ticks)
	finished := installSMSFixture(t, f, "")
	startAssisted(t, f, smsLoginInput())
	ticks <- time.Now()
	awaitAssistAction(t, browser)
	ctx := context.Background()
	receipts, err := f.service.SMSReceipts(ctx, "owner")
	if err != nil || len(receipts) != 1 || receipts[0].OrderID != "order-1" || !receipts[0].CanInspect {
		t.Fatalf("receipt unavailable: %v", err)
	}
	input := accountworkbench.OAuthSMSInput{Provider: "smsbower", APIKey: "private-sms-key"}
	result, err := f.service.InspectSMSReceipt(ctx, "owner", receipts[0].ID, input)
	if err != nil || !result.CodeAvailable {
		t.Fatalf("read-only inspection failed: %v", err)
	}
	raw, _ := json.Marshal(struct {
		Receipts any
		Result   any
	}{receipts, result})
	for _, secret := range []string{"private-sms-key", "654321", "config_hash"} {
		if strings.Contains(string(raw), secret) {
			t.Fatal("receipt response exposed private supplier data")
		}
	}
	if err := f.service.CancelOAuth("owner", f.view.ID); err != nil {
		t.Fatal(err)
	}
	f.awaitPhase(t, "cancelled")
	awaitSMSFinish(t, finished)
}

func TestSMSReceiptDifferentOwnerOrSupplierKeyCannotInspectOriginalOrder(t *testing.T) {
	f, browser := newAssistFixture(t, "phone", "sms_code")
	ticks := make(chan time.Time)
	f.service.UseOAuthAssistTicks(ticks)
	finished := installSMSFixture(t, f, "")
	startAssisted(t, f, smsLoginInput())
	ticks <- time.Now()
	awaitAssistAction(t, browser)
	ctx := context.Background()
	receipts, err := f.service.SMSReceipts(ctx, "owner")
	if err != nil || len(receipts) != 1 {
		t.Fatalf("receipt missing: %v", err)
	}
	input := accountworkbench.OAuthSMSInput{Provider: "smsbower", APIKey: "private-sms-key"}
	if _, err := f.service.InspectSMSReceipt(ctx, "other-owner", receipts[0].ID, input); err == nil {
		t.Fatal("cross-owner inspection accepted")
	}
	input.APIKey = "different-private-key"
	if _, err := f.service.InspectSMSReceipt(ctx, "owner", receipts[0].ID, input); err == nil {
		t.Fatal("different supplier credential accepted")
	}
	if err := f.service.CancelOAuth("owner", f.view.ID); err != nil {
		t.Fatal(err)
	}
	f.awaitPhase(t, "cancelled")
	awaitSMSFinish(t, finished)
}
