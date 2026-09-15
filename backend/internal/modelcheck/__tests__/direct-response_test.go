package modelcheck_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
)

func TestBehaviorCheckRejectsHTTP200BusinessFailureAcrossProtocols(t *testing.T) {
	for _, path := range []string{"/v1/responses", "/v1/chat/completions", "/v1/messages"} {
		t.Run(path, func(t *testing.T) {
			platform := "openai"
			if path == "/v1/messages" {
				platform = "anthropic"
			}
			f := setup(t, 1, platform, func(writer http.ResponseWriter, request *http.Request) {
				if request.URL.Path != path {
					writer.WriteHeader(http.StatusNotFound)
					return
				}
				payload := behaviorResponse()
				payload["error"] = map[string]string{"message": "private upstream failure"}
				_ = json.NewEncoder(writer).Encode(payload)
			})
			model := f.service.Capabilities().SolModels[0]
			if platform == "anthropic" {
				model = f.service.Capabilities().ClaudeStandards[0]
			}
			if _, err := f.service.Enqueue(context.Background(), modelcheck.Request{AccountIDs: []string{"1"}, Models: []string{model}, TimeoutSeconds: 5}); err != nil {
				t.Fatal(err)
			}
			task := finished(t, f)
			row := task.Result["tests"].([]map[string]any)[0]
			if row["verdict"] != "ERROR" || row["requests"].(map[string]any)["successful"] != 0 {
				t.Fatalf("business failure was scored as successful: %+v", row)
			}
		})
	}
}

func TestBehaviorCheckDoesNotPersistCredentialFromResponseModel(t *testing.T) {
	f := setup(t, 1, "openai", func(writer http.ResponseWriter, _ *http.Request) {
		payload := behaviorResponse()
		payload["model"] = fixtureSecret
		_ = json.NewEncoder(writer).Encode(payload)
	})
	if _, err := f.service.Enqueue(context.Background(), modelcheck.Request{AccountIDs: []string{"1"}, Models: f.service.Capabilities().SolModels[:1], TimeoutSeconds: 5}); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(finished(t, f))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), fixtureSecret) {
		t.Fatal("response model persisted an upstream credential")
	}
}

func behaviorResponse() map[string]any {
	answer := `["A","A","A","A","A","A","A","A","A","A","A","A"]`
	return map[string]any{
		"output_text": answer,
		"content":     []map[string]string{{"type": "text", "text": answer}},
		"choices":     []map[string]any{{"message": map[string]string{"content": answer}}},
	}
}
