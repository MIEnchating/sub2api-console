package accountworkbench_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
)

func TestLoginProfileSecurityUpdateRequiresCompletedOwnerBoundResultAndPreservesSecrets(t *testing.T) {
	f := newSecurityBatchFixture(t, nil)
	profile := saveProfile(t, f.batchFixture, "101")
	security, err := f.service.StartSecurity(context.Background(), "owner", accountworkbench.SecurityStartInput{AccountID: "101", Operation: "password", Password: securityPassword, Confirmed: true})
	if err != nil {
		t.Fatal(err)
	}
	f.awaitTask(t, "account-workbench-security-password", "waiting")
	input := accountworkbench.LoginProfileSecurityInput{ProfileID: profile.ID, Revision: profile.Revision, SecurityID: security.ID, Confirmed: true}
	if _, err := f.service.ApplySecurityToLoginProfile(context.Background(), "owner", input); err == nil {
		t.Fatal("pending security result updated profile")
	}
	if err := f.service.ContinueSecurity(context.Background(), "owner", security.ID); err != nil {
		t.Fatal(err)
	}
	f.awaitDone(t, security.ID)
	if _, err := f.service.ApplySecurityToLoginProfile(context.Background(), "foreign", input); err == nil {
		t.Fatal("foreign security result updated profile")
	}
	updated, err := f.service.ApplySecurityToLoginProfile(context.Background(), "owner", input)
	if err != nil || updated.Revision != 2 || !updated.HasPassword || !updated.HasTOTP {
		t.Fatalf("completed security result failed to update profile: %+v %v", updated, err)
	}
	raw, _ := json.Marshal(updated)
	if strings.Contains(string(raw), securityPassword) || strings.Contains(string(raw), securitySecret) {
		t.Fatal("security profile update exposed credentials")
	}
	if _, err := f.service.ApplySecurityToLoginProfile(context.Background(), "owner", input); err == nil {
		t.Fatal("security result bypassed profile CAS")
	}
}

func TestLoginProfileSecurityBatchResultUpdatesAfterChildSessionWasRemoved(t *testing.T) {
	f := newSecurityBatchFixture(t, nil)
	profile := saveProfile(t, f.batchFixture, "101")
	batch := f.startSecurityBatch(t, "totp", "101")
	childID := f.finishSecurityChild(t, "totp")
	f.awaitDone(t, batch.ID)
	if _, err := f.service.Security(context.Background(), "owner", childID, false); err == nil {
		t.Fatal("batch retained completed child session")
	}
	input := accountworkbench.LoginProfileSecurityInput{ProfileID: profile.ID, Revision: profile.Revision, BatchID: batch.ID, AccountID: "101", Confirmed: true}
	if _, err := f.service.ApplySecurityToLoginProfile(context.Background(), "foreign", input); err == nil {
		t.Fatal("foreign batch updated profile")
	}
	updated, err := f.service.ApplySecurityToLoginProfile(context.Background(), "owner", input)
	if err != nil || updated.Revision != 2 || !updated.HasTOTP {
		t.Fatalf("completed batch artifact unavailable: %+v %v", updated, err)
	}
}
