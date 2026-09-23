package modelcheck_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
)

const oauthAnimationAccount = `{"id":1,"type":"oauth","platform":"openai","credentials":{"access_token":"isolated-oauth-access-token","refresh_token":"isolated-refresh-token","chatgpt_account_id":"isolated-workspace"}}`

func TestOAuthAnimationWithoutKeyBindingUsesOfficialStreamAndSanitizesSVG(t *testing.T) {
	f, _ := oauthAccountFixture(t, oauthAnimationAccount)
	f.service.UseOAuthTransport(oauthTransportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != "https://chatgpt.com/backend-api/codex/responses" || r.Header.Get("Authorization") != "Bearer "+oauthFixtureToken || r.Header.Get("ChatGPT-Account-Id") != "isolated-workspace" || r.Header.Get("X-Request-ID") == "" {
			t.Error("animation lost its official endpoint, credential or request ID")
		}
		var body struct {
			Model  string `json:"model"`
			Stream bool   `json:"stream"`
			Store  bool   `json:"store"`
			Input  []struct {
				Content string `json:"content"`
			} `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			return nil, err
		}
		if body.Model != "animation-only-model" || !body.Stream || body.Store || len(body.Input) != 1 || !strings.Contains(body.Input[0].Content, "SVG") {
			t.Errorf("unexpected animation payload: %#v", body)
		}
		return oauthResponse(200, "text/event-stream", fmt.Sprintf("data: {\"type\":\"response.output_text.delta\",\"delta\":%q}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"model\":\"animation-only-model\"}}\n\n", fixtureSVG)), nil
	}))
	input := request("1")
	input.Targets[0].Model = "animation-only-model"
	if _, err := f.service.EnqueueAnimation(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	task := finished(t, f)
	rows := task.Result["animations"].([]modelcheck.AnimationResult)
	if task.Status != "succeeded" || len(rows) != 1 || !strings.Contains(rows[0].SVG, "<svg") || rows[0].ResponseModel != "animation-only-model" {
		t.Fatalf("animation result: %#v", task)
	}
	stored, err := f.tasks.ListBySkill(context.Background(), "sub2api-model-animation", 10)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(stored)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{oauthFixtureToken, "isolated-refresh-token", "isolated-workspace", "isolated-admin-key"} {
		if strings.Contains(string(raw), secret) {
			t.Error("animation history exposed credentials")
		}
	}
	statuses, err := f.service.AccountStatuses(context.Background())
	if err != nil || len(statuses) != 0 {
		t.Fatal("animation changed behavioral check status")
	}
}

func TestOAuthPrecheckRunsOnlySelectedQuestionWithLowReasoning(t *testing.T) {
	f, _ := oauthAccountFixture(t, oauthAnimationAccount)
	var calls int
	f.service.UseOAuthTransport(oauthTransportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		var body struct {
			Reasoning struct {
				Effort string `json:"effort"`
			} `json:"reasoning"`
			Input []struct {
				Content string `json:"content"`
			} `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			return nil, err
		}
		if body.Reasoning.Effort != "low" || len(body.Input) != 1 || !strings.Contains(body.Input[0].Content, "圆形苹果") || !strings.HasSuffix(r.Header.Get("X-Request-ID"), "-candy") {
			t.Errorf("unexpected selected precheck request: %#v", body)
		}
		return oauthResponse(200, "application/json", `{"status":"completed","output_text":"21"}`), nil
	}))
	input := request("1")
	input.Mode, input.PrecheckQuestions, input.Targets[0].Model = "precheck", []string{"candy"}, "gpt-6-astra"
	if _, err := f.service.EnqueueAnimation(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	task := finished(t, f)
	rows := task.Result["animations"].([]modelcheck.AnimationResult)
	if task.Status != "succeeded" || calls != 1 || rows[0].SVG != "" || rows[0].Precheck == nil || rows[0].Precheck.Verdict != "passed" || len(rows[0].Precheck.Questions) != 1 {
		t.Fatalf("selected precheck result: %#v", rows)
	}
}
