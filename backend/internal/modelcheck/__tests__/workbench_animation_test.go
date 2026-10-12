package modelcheck_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
)

func TestWorkbenchOAuthAnimationUsesLocalCredentialWithoutPlatformGeneration(t *testing.T) {
	for _, mode := range []string{"animation", "precheck"} {
		t.Run(mode, func(t *testing.T) {
			f, private := oauthAccountFixture(t, `{"id":1,"type":"oauth","platform":"openai","credentials":{"chatgpt_account_id":"isolated-workspace","chatgpt_user_id":"isolated-user"},"credentials_status":{"has_access_token":true}}`)
			store, err := configstore.Open(filepath.Join(t.TempDir(), "private.db"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = store.Close() })
			target := private.target
			if err := store.ConfigureTarget(t.Context(), target.BaseURL, target.AdminKey, target.TimeoutSeconds); err != nil {
				t.Fatal(err)
			}
			targetHash := sha256.Sum256([]byte(target.BaseURL + "\x00" + target.AdminKey))
			targetKey := hex.EncodeToString(targetHash[:])
			identityJSON, _ := json.Marshal([]string{targetKey, "1"})
			accountHash := sha256.Sum256(identityJSON)
			accountKey := hex.EncodeToString(accountHash[:])
			credentials := map[string]any{"access_token": oauthFixtureToken, "chatgpt_account_id": "isolated-workspace", "chatgpt_user_id": "isolated-user"}
			raw, err := json.Marshal(map[string]any{"target_key": targetKey, "public": map[string]any{"id": accountKey, "account_id": "1"}, "input": map[string]any{"credentials": credentials}, "generation_credentials": credentials})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := store.SaveWorkbenchDocument(t.Context(), "remembered-account:"+targetKey+":"+accountKey, 0, raw); err != nil {
				t.Fatal(err)
			}
			f.service.UseOAuthCredentialSource(accountworkbench.New(store))
			calls := 0
			f.service.UseOAuthTransport(oauthTransportFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.URL.String() != "https://chatgpt.com/backend-api/codex/responses" || r.Header.Get("Authorization") != "Bearer "+oauthFixtureToken || r.Header.Get("ChatGPT-Account-Id") != "isolated-workspace" {
					t.Error("local credential lost its official endpoint or account binding")
				}
				if mode == "precheck" {
					return oauthResponse(200, "application/json", `{"status":"completed","output_text":"21"}`), nil
				}
				return oauthResponse(200, "text/event-stream", fmt.Sprintf("data: {\"type\":\"response.output_text.delta\",\"delta\":%q}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\"}}\n\n", fixtureSVG)), nil
			}))
			input := request("1")
			input.Mode = mode
			if mode == "precheck" {
				input.PrecheckQuestions = []string{"candy"}
			}
			if _, err := f.service.EnqueueAnimation(t.Context(), input); err != nil {
				t.Fatal(err)
			}
			task := finished(t, f)
			if task.Status != "succeeded" || calls != 1 {
				t.Fatalf("local generation failed: %s %#v", task.Status, task.Result)
			}
			result := task.Result["animations"].([]modelcheck.AnimationResult)[0]
			if mode == "animation" && !strings.Contains(result.SVG, "<svg") {
				t.Fatal("generated artifact missing")
			}
			if mode == "precheck" && (result.Precheck == nil || result.Precheck.Verdict != "passed") {
				t.Fatal("precheck did not pass")
			}
			history, _ := json.Marshal(task)
			if strings.Contains(string(history), oauthFixtureToken) {
				t.Fatal("task exposed local credential")
			}
		})
	}
}
