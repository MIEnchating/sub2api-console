package workbenchprovider_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/workbenchprovider"
)

func TestSMSLubanUsesExactOrderIDAndPrefersExplicitOTPOverOtherNumbers(t *testing.T) {
	polls, releases := 0, 0
	client := workbenchprovider.NewHTTP(transportFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Host != "lubansms.com" || request.URL.Query().Get("apikey") != "isolated-key" {
			t.Fatal("Luban request changed endpoint or credentials")
		}
		switch request.URL.Path {
		case "/v2/api/getNumber":
			if request.URL.Query().Get("service_id") != "121949" {
				t.Fatal("Luban purchase changed service ID")
			}
			return smsResponse(request, `{"code":0,"number":17005550123,"request_id":9007199254740993}`), nil
		case "/v2/api/getSms":
			if request.URL.Query().Get("request_id") != "9007199254740993" {
				t.Fatal("provider request ID lost numeric precision")
			}
			polls++
			if polls == 1 {
				return smsResponse(request, `{"code":0,"msg":"wait"}`), nil
			}
			return smsResponse(request, `{"code":0,"sms_code":{"request_id":"999999","verification_code":"012345"},"sms_msg":"OpenAI 654321","msg":"success"}`), nil
		case "/v2/api/setStatus":
			releases++
			if request.URL.Query().Get("request_id") != "9007199254740993" || request.URL.Query().Get("status") != "reject" {
				t.Fatal("release changed stable order ID or provider status")
			}
			return smsResponse(request, `{"code":0}`), nil
		default:
			t.Fatal("unsupported Luban endpoint")
			return nil, context.Canceled
		}
	}))
	provider, err := workbenchprovider.NewSMS(workbenchprovider.SMSConfig{Provider: "luban", APIKey: "isolated-key", ServiceID: "121949"}, client)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	number, err := provider.Acquire(ctx, "one-purchase")
	if err != nil || number.Phone != "+17005550123" || number.RequestID != "9007199254740993" {
		t.Fatalf("Luban acquisition rejected valid exact ID/phone: %v", err)
	}
	if err := provider.MarkReady(ctx, number.RequestID); err != nil {
		t.Fatal(err)
	}
	if message, err := provider.Poll(ctx, number.RequestID); err != nil || !message.Pending {
		t.Fatal("Luban wait response was accepted as a code")
	}
	if message, err := provider.Poll(ctx, number.RequestID); err != nil || message.Code != "012345" || message.Pending {
		t.Fatalf("Luban explicit code parsing failed: %v", err)
	}
	if err := provider.Release(ctx, number.RequestID); err != nil {
		t.Fatal(err)
	}
	if err := provider.Release(ctx, number.RequestID); err != nil || releases != 1 {
		t.Fatal("Luban release was replayed")
	}
}

func TestSMSLubanBusinessFailureNeverReturnsSecretProviderMessage(t *testing.T) {
	client := workbenchprovider.NewHTTP(transportFunc(func(request *http.Request) (*http.Response, error) {
		return smsResponse(request, `{"code":401,"msg":"isolated-key +17005550123 private provider detail"}`), nil
	}))
	provider, err := workbenchprovider.NewSMS(workbenchprovider.SMSConfig{Provider: "luban", APIKey: "isolated-key", ServiceID: "121949"}, client)
	if err != nil {
		t.Fatal(err)
	}
	_, err = provider.Acquire(context.Background(), "one-purchase")
	if err == nil || strings.Contains(err.Error(), "isolated-key") || strings.Contains(err.Error(), "17005550123") {
		t.Fatal("Luban business failure accepted or private details returned")
	}
}

func TestSMSLubanSixDigitOrderIDIsNotMistakenForOTP(t *testing.T) {
	client := workbenchprovider.NewHTTP(transportFunc(func(request *http.Request) (*http.Response, error) {
		if strings.HasSuffix(request.URL.Path, "getNumber") {
			return smsResponse(request, `{"code":0,"number":"+17005550123","request_id":"111111"}`), nil
		}
		return smsResponse(request, `{"code":0,"sms_code":{"request_id":"999999","phone":"123456"},"msg":"success"}`), nil
	}))
	provider, err := workbenchprovider.NewSMS(workbenchprovider.SMSConfig{Provider: "luban", APIKey: "isolated-key", ServiceID: "121949"}, client)
	if err != nil {
		t.Fatal(err)
	}
	number, err := provider.Acquire(context.Background(), "one-purchase")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Poll(context.Background(), number.RequestID); err == nil {
		t.Fatal("provider metadata was treated as a verification code")
	}
}
