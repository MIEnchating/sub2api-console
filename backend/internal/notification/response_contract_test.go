package notification

import (
	"context"
	"net/http"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func TestQQBotBusinessFailureWithMessageIDIsNotReportedAsSent(t *testing.T) {
	for _, response := range []string{
		`{"code":11255,"message":"permission denied","id":"request-id"}`,
		`{"success":false,"message":"permission denied","id":"request-id"}`,
		`{"errcode":40001,"message":"permission denied","id":"request-id"}`,
	} {
		t.Run(response, func(t *testing.T) {
			transport := &responseRoundTripper{responses: []string{`{"access_token":"test-token"}`, response}}
			outcomes := NewQQBotSender(&http.Client{Transport: transport}).Send(context.Background(), configstore.NotificationSettings{
				AppID: "test-app", ClientSecret: "test-secret", HomeChannel: "test-target", HomeChannelType: "c2c",
			}, []string{"test message"})
			if len(outcomes) != 1 || outcomes[0].Success || outcomes[0].CommitUnknown || outcomes[0].MessageID != nil {
				t.Fatalf("business failure was treated as delivery success: %#v", outcomes)
			}
		})
	}
}

func TestQQBotMalformedMessageIDDoesNotConfirmDelivery(t *testing.T) {
	for _, response := range []string{`{"id":false}`, `{"id":{"value":"message-id"}}`, `{"id":0}`} {
		t.Run(response, func(t *testing.T) {
			transport := &responseRoundTripper{responses: []string{`{"access_token":"test-token"}`, response}}
			outcomes := NewQQBotSender(&http.Client{Transport: transport}).Send(context.Background(), configstore.NotificationSettings{
				AppID: "test-app", ClientSecret: "test-secret", HomeChannel: "test-target", HomeChannelType: "c2c",
			}, []string{"test message"})
			if len(outcomes) != 1 || outcomes[0].Success || !outcomes[0].CommitUnknown {
				t.Fatalf("malformed message ID confirmed delivery: %#v", outcomes)
			}
		})
	}
}

func TestQQBotNumericMessageIDPreservesStableIdentity(t *testing.T) {
	transport := &responseRoundTripper{responses: []string{`{"access_token":"test-token"}`, `{"id":9007199254740993}`}}
	outcomes := NewQQBotSender(&http.Client{Transport: transport}).Send(context.Background(), configstore.NotificationSettings{
		AppID: "test-app", ClientSecret: "test-secret", HomeChannel: "test-target", HomeChannelType: "c2c",
	}, []string{"test message"})
	if len(outcomes) != 1 || !outcomes[0].Success || outcomes[0].MessageID == nil || *outcomes[0].MessageID != "9007199254740993" {
		t.Fatalf("numeric message ID changed: %#v", outcomes)
	}
}
