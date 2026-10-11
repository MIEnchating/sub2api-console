package accountworkbench_test

import (
	"encoding/json"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
)

func TestTemplatePayloadDecodingPreservesLargeIDsAndNestedDecimalNumbers(t *testing.T) {
	proxy, load := "9007199254740993", "1.1234567890123456789"
	config := accountworkbench.TemplateConfig{
		Concurrency:    1,
		Priority:       50,
		RateMultiplier: "0.1234567890123456789",
		ProxyID:        &proxy,
		LoadFactor:     &load,
		GroupIDs:       []string{"9007199254740995"},
		CredentialExtras: map[string]json.RawMessage{
			"model_mapping": json.RawMessage(`{"requested":"returned"}`),
		},
		Extra: map[string]json.RawMessage{
			"model_rate_limits": json.RawMessage(`{"large":9007199254740993,"decimal":0.1234567890123456789}`),
		},
	}
	item := accountworkbench.InputItem{Credentials: map[string]any{"access_token": "private-access", "chatgpt_account_id": "workspace", "chatgpt_user_id": "user"}}
	payload, err := accountworkbench.AccountPayload(item, &accountworkbench.Template{Config: config})
	if err != nil {
		t.Fatal(err)
	}
	limits := payload["extra"].(map[string]any)["model_rate_limits"].(map[string]any)
	if payload["proxy_id"] != json.Number(proxy) || payload["load_factor"] != json.Number(load) || payload["group_ids"].([]json.Number)[0] != json.Number("9007199254740995") || limits["large"] != json.Number("9007199254740993") || limits["decimal"] != json.Number("0.1234567890123456789") {
		t.Fatal("template decoding lost number precision")
	}
	credentials := payload["credentials"].(map[string]any)
	if credentials["model_mapping"].(map[string]any)["requested"] != "returned" || credentials["access_token"] != "private-access" || item.Credentials["model_mapping"] != nil {
		t.Fatal("template decoding changed credential identity or source")
	}
}

func TestTemplatePayloadInvalidRawConfigurationReturnsValidationError(t *testing.T) {
	config := accountworkbench.TemplateConfig{RateMultiplier: "1", CredentialExtras: map[string]json.RawMessage{"model_mapping": json.RawMessage(`{"requested":`)}}
	item := accountworkbench.InputItem{Credentials: map[string]any{"access_token": "private-access"}}
	payload, err := accountworkbench.AccountPayload(item, &accountworkbench.Template{Config: config})
	if err == nil || err.Error() != "模型映射无效" || payload != nil {
		t.Fatal("malformed template was decoded as a payload", err)
	}
}
