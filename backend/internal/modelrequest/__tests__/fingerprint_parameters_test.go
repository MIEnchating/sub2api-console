package modelrequest_test

import (
	"encoding/json"
	"github.com/MIEnchating/sub2api-console/backend/internal/modelrequest"
	"testing"
)

func TestFingerprintParametersPreserveZeroTemperatureAndSystemAcrossProtocols(t *testing.T) {
	for _, protocol := range []modelrequest.Protocol{modelrequest.Chat, modelrequest.Responses, modelrequest.Anthropic, modelrequest.Gemini} {
		t.Run(string(protocol), func(t *testing.T) {
			zero := 0.0
			_, body, err := modelrequest.Build(protocol, modelrequest.Parameters{Model: "model-a", Prompt: "question", Instructions: "fixed system", Temperature: &zero, MaxOutputTokens: 80})
			if err != nil {
				t.Fatal(err)
			}
			raw, _ := json.Marshal(body)
			var payload map[string]any
			_ = json.Unmarshal(raw, &payload)
			if protocol == modelrequest.Gemini {
				config := payload["generationConfig"].(map[string]any)
				if config["temperature"] != float64(0) {
					t.Fatal(string(raw))
				}
				if payload["systemInstruction"] == nil {
					t.Fatal("system instruction lost")
				}
			} else {
				if payload["temperature"] != float64(0) {
					t.Fatal(string(raw))
				}
				switch protocol {
				case modelrequest.Chat:
					messages := payload["messages"].([]any)
					if len(messages) != 2 || messages[0].(map[string]any)["role"] != "system" {
						t.Fatal(string(raw))
					}
				case modelrequest.Anthropic:
					if payload["system"] != "fixed system" {
						t.Fatal(string(raw))
					}
				case modelrequest.Responses:
					if payload["instructions"] != "fixed system" {
						t.Fatal(string(raw))
					}
				}
			}
		})
	}
}
