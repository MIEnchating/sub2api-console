package accountworkbench_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func bindMaintenance(t *testing.T, f *batchFixture, revision int64) (string, string) {
	t.Helper()
	token, err := f.private.CreateSession(context.Background(), "admin", time.Hour, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256([]byte(token))
	owner := hex.EncodeToString(hash[:])
	if err := f.service.BindMaintenanceOwner(context.Background(), owner, revision); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.private.RevokeSession(context.Background(), token) })
	return owner, token
}

func TestMaintenanceReauthorizationRequiresEnabledConfigAndNeverUsesSavedSMSProvider(t *testing.T) {
	f := newProfileFixture(t)
	_, err := f.service.SaveLoginProfile(context.Background(), "owner", accountworkbench.LoginProfileSaveInput{AccountID: "101", Confirmed: true, Login: accountworkbench.OAuthLoginInput{Email: "owner@example.com", SMS: &accountworkbench.OAuthSMSInput{Provider: "smsbower", APIKey: "private-bower-key", Service: "dr", Country: "0"}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.StartMaintenanceReauthorization(context.Background(), "owner", []string{"101"}, 0); err == nil {
		t.Fatal("maintenance relogin started without explicit config")
	}
	config := configstore.WorkbenchMaintenance{Enabled: true, ReauthorizeWithProfiles: true, IntervalMinutes: 5, CooldownMinutes: 10, Model: "gpt-5.6-sol"}
	if err := f.private.SaveWorkbenchMaintenance(context.Background(), f.server.URL, config); err != nil {
		t.Fatal(err)
	}
	owner, _ := bindMaintenance(t, f, 1)
	var providerCalls atomic.Int32
	f.service.UseProviderTransport(oauthTransportFunc(func(*http.Request) (*http.Response, error) { providerCalls.Add(1); return oauthResponse(`{}`), nil }))
	view, err := f.service.StartMaintenanceReauthorization(context.Background(), owner, []string{"101"}, 1)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.service.CancelOAuthBatch(owner, view.ID) })
	if !view.FreshLogin || view.Items[0].SMSProvider != "" {
		t.Fatal("maintenance retained SMS purchase config")
	}
	child := f.awaitTask(t, "account-workbench-oauth", "waiting")
	config.Revision, config.ReauthorizeWithProfiles = 1, false
	if err := f.private.SaveWorkbenchMaintenance(context.Background(), f.server.URL, config); err != nil {
		t.Fatal(err)
	}
	if err := f.service.FinishOAuth(owner, child.ID); err != nil {
		t.Fatal(err)
	}
	f.awaitDone(t, view.ID)
	stored, err := f.service.ReadOAuthBatch(owner, view.ID)
	if err == nil && (stored.Available != 0 || stored.Status != "failed" && stored.Status != "cancelled") {
		t.Fatalf("disabled maintenance completed relogin: %+v %v", stored, err)
	}
	if providerCalls.Load() != 0 {
		t.Fatal("automatic relogin called SMS provider")
	}
}
