package protocolsdk_test

import (
	"context"
	"encoding/json"
	"github.com/MIEnchating/sub2api-console/backend/internal/protocolsdk"
	"os"
	"testing"
)

func TestOfficialSDKRunsInGoAndBindsReturnedTokenToDeviceAndFlow(t *testing.T) {
	sdk, err := os.ReadFile("testdata/official-sdk-20260219f9f6.js")
	if err != nil {
		t.Fatal(err)
	}
	requests := 0
	tokens, err := protocolsdk.Run(context.Background(), protocolsdk.Options{SDK: string(sdk), SDKURL: "https://sentinel.openai.com/sentinel/20260219f9f6/sdk.js", PageURL: "https://auth.openai.com/log-in/password", DeviceID: "test-device", Flow: "password_verify", Request: func(_ context.Context, req protocolsdk.Request) (protocolsdk.Response, error) {
		requests++
		if req.Method != "POST" || req.URL != "https://sentinel.openai.com/backend-api/sentinel/req" {
			t.Fatal("SDK requested unexpected resource")
		}
		var payload map[string]any
		if json.Unmarshal([]byte(req.Body), &payload) != nil || payload["p"] == "" || payload["id"] != "test-device" || payload["flow"] != "password_verify" {
			t.Fatal("SDK requirements were not bound")
		}
		return protocolsdk.Response{Status: 200, Body: `{"token":"test-server-token","proofofwork":{"required":false},"turnstile":{"required":false}}`}, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if json.Unmarshal([]byte(tokens.Token), &payload) != nil || payload["id"] != "test-device" || payload["flow"] != "password_verify" || payload["c"] != "test-server-token" || requests != 1 {
		t.Fatalf("SDK result not bound (requests=%d)", requests)
	}
}

func TestSDKExecutionRejectsReturnedErrorsAndMismatchedBindings(t *testing.T) {
	for _, tc := range []struct{ name, token string }{
		{"sdk error", `{"e":"failure","id":"test-device","flow":"password_verify","c":"challenge"}`},
		{"different device", `{"id":"other-device","flow":"password_verify","c":"challenge"}`},
		{"different flow", `{"id":"test-device","flow":"email_otp_validate","c":"challenge"}`},
		{"missing challenge", `{"id":"test-device","flow":"password_verify"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			encoded, _ := json.Marshal(tc.token)
			_, err := protocolsdk.Run(context.Background(), protocolsdk.Options{SDK: `var SentinelSDK={token:async function(){return ` + string(encoded) + `;}}`, SDKURL: "https://sentinel.openai.com/sentinel/test/sdk.js", PageURL: "https://auth.openai.com/log-in/password", DeviceID: "test-device", Flow: "password_verify"})
			if err == nil {
				t.Fatal("invalid SDK result accepted")
			}
		})
	}
}
func TestSDKCancellationInterruptsRunningJavaScript(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	entered := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		_, err := protocolsdk.Run(ctx, protocolsdk.Options{SDK: `var SentinelSDK={token:async function(){await fetch('https://sentinel.openai.com/backend-api/sentinel/req');for(;;){};}}`, SDKURL: "https://sentinel.openai.com/sentinel/test/sdk.js", PageURL: "https://auth.openai.com/log-in/password", Request: func(context.Context, protocolsdk.Request) (protocolsdk.Response, error) {
			close(entered)
			return protocolsdk.Response{Status: 200, Body: `{}`}, nil
		}})
		done <- err
	}()
	<-entered
	cancel()
	if err := <-done; err == nil {
		t.Fatal("cancelled SDK completed")
	}
}
