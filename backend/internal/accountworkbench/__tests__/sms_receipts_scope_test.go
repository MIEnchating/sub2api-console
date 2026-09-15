package accountworkbench_test

import (
	"context"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
)

func TestLocalOAuthSMSReceiptRemainsInspectableOnlyWithinItsOwnerAndScope(t *testing.T) {
	f, browser := newAssistFixture(t, "phone", "sms_code")
	ticks := make(chan time.Time)
	f.service.UseOAuthAssistTicks(ticks)
	finished := installSMSFixture(t, f, "")
	login := smsLoginInput()
	ctx := context.Background()
	var err error
	f.view, err = f.service.StartOAuthWithInput(ctx, "owner", accountworkbench.OAuthStartInput{Scope: accountworkbench.ScopeLocalExport, Login: &login})
	if err != nil {
		t.Fatal(err)
	}
	f.awaitPhase(t, "waiting")
	ticks <- time.Now()
	awaitAssistAction(t, browser)

	receipts, err := f.service.SMSReceiptsScoped(ctx, "owner", accountworkbench.ScopeLocalExport)
	if err != nil || len(receipts) != 1 || receipts[0].OrderID != "order-1" || !receipts[0].CanInspect {
		t.Fatalf("local order is not available for inspection: %v", err)
	}
	managed, err := f.service.SMSReceipts(ctx, "owner")
	if err != nil || len(managed) != 0 {
		t.Fatalf("local order appeared in managed scope: %v", err)
	}
	input := accountworkbench.OAuthSMSInput{Provider: "smsbower", APIKey: "private-sms-key"}
	if _, err := f.service.InspectSMSReceipt(ctx, "owner", receipts[0].ID, input); err == nil {
		t.Fatal("managed scope inspected a local order")
	}
	if _, err := f.service.InspectSMSReceiptScoped(ctx, "other-owner", receipts[0].ID, accountworkbench.ScopeLocalExport, input); err == nil {
		t.Fatal("another owner inspected a local order")
	}
	result, err := f.service.InspectSMSReceiptScoped(ctx, "owner", receipts[0].ID, accountworkbench.ScopeLocalExport, input)
	if err != nil || !result.CodeAvailable {
		t.Fatalf("local order inspection failed: %v", err)
	}
	if err := f.service.CancelOAuth("owner", f.view.ID); err != nil {
		t.Fatal(err)
	}
	f.awaitPhase(t, "cancelled")
	awaitSMSFinish(t, finished)
}

func TestSMSReceiptInvalidScopeIsRejectedBeforeReadingOrders(t *testing.T) {
	f := newOAuthFixture(t)
	ctx := context.Background()
	if _, err := f.service.SMSReceiptsScoped(ctx, "owner", "invalid"); err == nil {
		t.Fatal("unknown receipt scope accepted")
	}
	if _, err := f.service.InspectSMSReceiptScoped(ctx, "owner", "order", "invalid", accountworkbench.OAuthSMSInput{}); err == nil {
		t.Fatal("unknown inspection scope accepted")
	}
}
