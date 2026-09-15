package workbenchprovider_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/workbenchprovider"
)

func smsResponse(request *http.Request, text string) *http.Response {
	return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(text)), Request: request}
}

func TestSMSBowerOptionsPreserveExactDecimalPricesAndSortWithoutFloatRounding(t *testing.T) {
	client := workbenchprovider.NewHTTP(transportFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Scheme != "https" || request.URL.Host != "smsbower.page" || request.URL.Path != "/stubs/handler_api.php" || request.URL.Query().Get("api_key") != "isolated-key" {
			t.Fatal("request escaped the fixed provider endpoint or lost private credentials")
		}
		if request.URL.Query().Get("action") == "getPrices" {
			return smsResponse(request, `{"1001":{"dr":{"cost":0.100000000000000001,"count":3}},"187":{"dr":{"cost":0.100000000000000000,"count":2}},"7":{"dr":{"cost":0.01,"count":0}}}`), nil
		}
		return smsResponse(request, `{"data":[{"id":1001,"chn":"日本","iso":"jp","prefix":81},{"id":187,"chn":"美国","iso":"us","prefix":1}]}`), nil
	}))
	provider, err := workbenchprovider.NewSMS(workbenchprovider.SMSConfig{Provider: "smsbower", APIKey: "isolated-key"}, client)
	if err != nil {
		t.Fatal(err)
	}
	options, err := provider.Options(context.Background())
	if err != nil || len(options) != 2 || options[0].Country != "187" || options[0].Price != "0.100000000000000000" || options[1].Price != "0.100000000000000001" {
		t.Fatalf("exact prices/order lost: %+v, %v", options, err)
	}
}

func TestSMSBowerActivationUsesStableIDAndValidatesEveryStateResponse(t *testing.T) {
	counts := map[string]int{}
	client := workbenchprovider.NewHTTP(transportFunc(func(request *http.Request) (*http.Response, error) {
		query := request.URL.Query()
		action := query.Get("action")
		counts[action+query.Get("status")]++
		switch action {
		case "getNumber":
			if query.Get("service") != "dr" || query.Get("country") != "1001" || query.Get("maxPrice") != "0.123456789012345678" {
				t.Fatal("purchase changed selected service/country/exact price")
			}
			return smsResponse(request, "ACCESS_NUMBER:activation-1:0017005550123"), nil
		case "getStatus":
			if query.Get("id") != "activation-1" {
				t.Fatal("poll lost stable provider order ID")
			}
			if counts[action] == 1 {
				return smsResponse(request, "STATUS_WAIT_CODE"), nil
			}
			return smsResponse(request, "STATUS_OK:Verification 012345"), nil
		case "setStatus":
			if query.Get("status") == "1" {
				return smsResponse(request, "ACCESS_READY"), nil
			}
			return smsResponse(request, "ACCESS_ACTIVATION"), nil
		default:
			t.Fatalf("unexpected request: %s", action)
			return nil, errors.New("unexpected test request")
		}
	}))
	config := workbenchprovider.SMSConfig{Provider: "smsbower", APIKey: "private-test-key", Country: "1001", MaxPrice: "0.123456789012345678"}
	provider, err := workbenchprovider.NewSMS(config, client)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	number, err := provider.Acquire(ctx, "operation-1")
	if err != nil || number.RequestID != "activation-1" || number.Phone != "+17005550123" {
		t.Fatalf("number normalization/stable ID failed: %v", err)
	}
	if again, err := provider.Acquire(ctx, "operation-1"); err != nil || again != number || counts["getNumber"] != 1 {
		t.Fatal("same operation acquired another number")
	}
	if err := provider.MarkReady(ctx, number.RequestID); err != nil {
		t.Fatal(err)
	}
	if err := provider.MarkReady(ctx, number.RequestID); err != nil || counts["setStatus1"] != 1 {
		t.Fatal("ready action replayed")
	}
	if waiting, err := provider.Poll(ctx, number.RequestID); err != nil || !waiting.Pending || waiting.Code != "" {
		t.Fatal("waiting state was treated as a received code")
	}
	message, err := provider.Poll(ctx, number.RequestID)
	if err != nil || message.Pending || message.Code != "012345" {
		t.Fatalf("valid OTP not parsed: %v", err)
	}
	if err := provider.Complete(ctx, number.RequestID); err != nil {
		t.Fatal(err)
	}
	if err := provider.Complete(ctx, number.RequestID); err != nil || counts["setStatus6"] != 1 {
		t.Fatal("complete action replayed")
	}
	if err := provider.Release(ctx, number.RequestID); err == nil {
		t.Fatal("completed order was cancelled")
	}
	raw, _ := json.Marshal([]any{config, number, message})
	if strings.Contains(string(raw), "private-test-key") || strings.Contains(string(raw), "17005550123") || strings.Contains(string(raw), "012345") {
		t.Fatal("SMS private config, phone, or code serialized")
	}
}

func TestSMSBowerSuccessfulHTTPBusinessErrorDoesNotEchoProviderSecrets(t *testing.T) {
	for _, response := range []string{"NO_BALANCE", "private-test-key provider echoed secret", ""} {
		t.Run(response, func(t *testing.T) {
			requests := 0
			client := workbenchprovider.NewHTTP(transportFunc(func(request *http.Request) (*http.Response, error) {
				requests++
				return smsResponse(request, response), nil
			}))
			provider, err := workbenchprovider.NewSMS(workbenchprovider.SMSConfig{Provider: "smsbower", APIKey: "private-test-key", Country: "1001"}, client)
			if err != nil {
				t.Fatal(err)
			}
			_, err = provider.Acquire(context.Background(), "once")
			if err == nil || strings.Contains(err.Error(), "private-test-key") {
				t.Fatal("business error was accepted or exposed credentials")
			}
			_, _ = provider.Acquire(context.Background(), "once")
			if requests != 1 {
				t.Fatal("failed purchase replayed")
			}
		})
	}
}

func TestSMSBowerOptionsRejectExplicitFailureEvenWhenResponseIncludesStock(t *testing.T) {
	client := workbenchprovider.NewHTTP(transportFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Query().Get("action") == "getPrices" {
			return smsResponse(request, `{"success":false,"1001":{"dr":{"cost":"0.1","count":5}}}`), nil
		}
		return smsResponse(request, `[{"id":1001,"chn":"日本"}]`), nil
	}))
	provider, err := workbenchprovider.NewSMS(workbenchprovider.SMSConfig{Provider: "smsbower", APIKey: "isolated-key"}, client)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Options(context.Background()); err == nil {
		t.Fatal("provider failure envelope was accepted as purchasable stock")
	}
}
