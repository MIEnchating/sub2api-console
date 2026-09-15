package accountworkbench_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin"
)

type smsRecoveryFactory struct{ *checkpointFactory }
type smsRecoveryBrowser struct{ *checkpointBrowser }

func (f *smsRecoveryFactory) OpenOAuth(ctx context.Context, options browserlogin.OAuthOptions) (browserlogin.OAuthBrowser, error) {
	browser, err := f.checkpointFactory.OpenOAuth(ctx, options)
	if err != nil {
		return nil, err
	}
	return &smsRecoveryBrowser{browser.(*checkpointBrowser)}, nil
}
func (f *smsRecoveryFactory) RestoreOAuth(ctx context.Context, options browserlogin.OAuthRestoreOptions) (browserlogin.OAuthBrowser, error) {
	browser, err := f.checkpointFactory.RestoreOAuth(ctx, options)
	if err != nil {
		return nil, err
	}
	return &smsRecoveryBrowser{browser.(*checkpointBrowser)}, nil
}
func (b *smsRecoveryBrowser) InspectAuth(context.Context) (browserlogin.AuthPage, error) {
	return browserlogin.AuthPage{Stage: "phone", Revision: strings.Repeat("1", 64)}, nil
}
func (b *smsRecoveryBrowser) ApplyAuth(context.Context, browserlogin.AuthAction) error { return nil }

func TestLiveOAuthSMSRestoredCheckpointRetainsOriginalPurchaseAndRejectsAnotherProvider(t *testing.T) {
	f := newCheckpointFixture(t)
	factory := &smsRecoveryFactory{f.factory}
	f.service.UseOAuthBrowser(factory)
	ticks := make(chan time.Time)
	f.service.UseOAuthAssistTicks(ticks)
	acquired := make(chan struct{}, 1)
	f.service.UseProviderTransport(oauthTransportFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Query().Get("action") == "getNumber" {
			acquired <- struct{}{}
			return oauthResponse("ACCESS_NUMBER:original-order:15555550101"), nil
		}
		return oauthResponse("ACCESS_READY"), nil
	}))
	login := smsLoginInput()
	started, err := f.service.StartOAuthWithInput(context.Background(), "checkpoint-owner", accountworkbench.OAuthStartInput{Login: &login})
	if err != nil {
		t.Fatal(err)
	}
	f.active = append(f.active, started.ID)
	f.phase(t, "waiting")
	ticks <- time.Now()
	select {
	case <-acquired:
	case <-time.After(5 * time.Second):
		t.Fatal("original SMS order was not purchased")
	}
	saved := f.save(t, started.ID)
	f.configureService()
	f.service.UseOAuthBrowser(factory)
	restored, err := f.service.RestoreOAuthCheckpoint(context.Background(), "checkpoint-owner", saved.ID, accountworkbench.OAuthCheckpointAction{Revision: saved.Revision, Confirmed: true})
	if err != nil {
		t.Fatal(err)
	}
	f.active = append(f.active, restored.ID)
	f.phase(t, "waiting")
	state, err := f.service.ReadOAuthSMSAttachment(context.Background(), "checkpoint-owner", restored.ID, "")
	if err != nil || !state.Configured || state.CanAttach {
		t.Fatalf("restored SMS lineage = %+v, %v", state, err)
	}
	if err := f.service.AttachOAuthSMS(context.Background(), "checkpoint-owner", restored.ID, attachSMSInput(strings.Repeat("1", 64))); err == nil {
		t.Fatal("restored authorization purchased another number despite its original receipt")
	}
}
