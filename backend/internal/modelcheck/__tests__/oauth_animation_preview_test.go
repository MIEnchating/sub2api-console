package modelcheck_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
)

const redactedOAuthAccount = `{"id":1,"type":"oauth","platform":"openai","proxy_id":7,"credentials":{},"credentials_status":{"has_access_token":true}}`

func TestOAuthAnimationWithRedactedCredentialsUsesBoundPreviewAndKeepsSVGValidation(t *testing.T) {
	for _, unsafe := range []bool{false, true} {
		t.Run(map[bool]string{false: "safe result", true: "unsafe result"}[unsafe], func(t *testing.T) {
			called := false
			f, _ := oauthAccountFixture(t, redactedOAuthAccount, func(w http.ResponseWriter, r *http.Request) {
				called = true
				var input struct {
					ModelID         string `json:"model_id"`
					Prompt          string `json:"prompt"`
					RequestID       string `json:"request_id"`
					ReasoningEffort string `json:"reasoning_effort"`
				}
				if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
					t.Error(err)
					return
				}
				if input.ModelID != "gpt-6-astra" || !strings.Contains(input.Prompt, "SVG") || input.RequestID == "" || input.ReasoningEffort != "low" || r.Header.Get("X-API-Key") != "isolated-admin-key" {
					t.Error("preview lost its model, prompt, request identity or management authentication")
				}
				text := fixtureSVG
				if unsafe {
					text = `<svg><script>alert(1)</script></svg>`
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"account_id": 1, "request_id": input.RequestID, "model": "gpt-6-astra", "text": text}})
			})
			input := request("1")
			input.Targets[0].Model = "gpt-6-astra"
			if _, err := f.service.EnqueueAnimation(context.Background(), input); err != nil {
				t.Fatal(err)
			}
			task := finished(t, f)
			rows := task.Result["animations"].([]modelcheck.AnimationResult)
			if !called {
				t.Fatal("redacted OAuth credential prevented the controlled preview request")
			}
			if unsafe {
				if task.Status != "failed" || rows[0].SVG != "" {
					t.Fatal("unsafe SVG was accepted")
				}
			} else if task.Status != "succeeded" || !strings.Contains(rows[0].SVG, "<svg") {
				t.Fatalf("controlled preview failed: %#v", rows)
			}
		})
	}
}

func TestOAuthPreviewMissingEndpointReportsUpgradeWithoutCallingLegacyTest(t *testing.T) {
	f, _ := oauthAccountFixture(t, redactedOAuthAccount, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNotFound) })
	if _, err := f.service.EnqueueAnimation(context.Background(), request("1")); err != nil {
		t.Fatal(err)
	}
	rows := finished(t, f).Result["animations"].([]modelcheck.AnimationResult)
	if len(rows) != 1 || !strings.Contains(rows[0].Error, "升级 Sub2API") {
		t.Fatalf("missing upgrade guidance: %#v", rows)
	}
}

func TestOAuthPreviewPrecheckUsesOnlySelectedQuestion(t *testing.T) {
	f, _ := oauthAccountFixture(t, redactedOAuthAccount, func(w http.ResponseWriter, r *http.Request) {
		var input map[string]any
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			t.Error(err)
			return
		}
		prompt, _ := input["prompt"].(string)
		if !strings.Contains(prompt, "圆形苹果") {
			t.Error("preview changed the selected precheck prompt")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"account_id": 1, "request_id": input["request_id"], "model": "gpt-6-astra", "text": "21"}})
	})
	input := request("1")
	input.Mode, input.PrecheckQuestions, input.Targets[0].Model = "precheck", []string{"candy"}, "gpt-6-astra"
	if _, err := f.service.EnqueueAnimation(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	rows := finished(t, f).Result["animations"].([]modelcheck.AnimationResult)
	if rows[0].Precheck == nil || rows[0].Precheck.Verdict != "passed" {
		t.Fatalf("precheck failed: %#v", rows)
	}
}
