package modelcheck_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
)

type oauthAccountStore struct {
	credentials
	target configstore.TargetSettings
}

func (s *oauthAccountStore) TargetSettings(context.Context) (configstore.TargetSettings, error) {
	return s.target, nil
}

func (s *oauthAccountStore) UpstreamKeySecret(context.Context, string, string, string) (*configstore.UpstreamKeySecret, error) {
	return nil, errors.New("OAuth tasks must not read API Key bindings")
}

func oauthAccountFixture(t *testing.T, accountJSON string, preview ...http.HandlerFunc) (*fixture, *oauthAccountStore) {
	t.Helper()
	f := setup(t, 1, "openai", func(http.ResponseWriter, *http.Request) {
		t.Error("OAuth account must not use its configured Base URL")
	})
	accountType := "oauth"
	f.catalog.rows[0].AccountType = &accountType
	f.catalog.details["1"].AccountType = &accountType
	f.catalog.details["1"].Bindings = nil
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(preview) > 0 && r.Method == http.MethodPost && r.URL.Path == "/api/v1/admin/accounts/1/generate-preview" {
			preview[0](w, r)
			return
		}
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/admin/accounts/1" {
			t.Errorf("unexpected management request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"code":0,"data":%s}`, accountJSON)
	}))
	t.Cleanup(server.Close)
	private := &oauthAccountStore{target: configstore.TargetSettings{BaseURL: server.URL, AdminKey: "isolated-admin-key", TimeoutSeconds: 2}}
	service, err := modelcheck.New(f.tasks, private, &oauthProfileCatalog{catalog: f.catalog}, credentials{})
	if err != nil {
		t.Fatal(err)
	}
	service.UseTaskRunner(f.runner)
	f.service = service
	return f, private
}

func TestOAuthAccountSolCheckUsesProfileCapturedBeforeQueueing(t *testing.T) {
	f, _ := oauthAccountFixture(t, `{"id":1,"type":"oauth","platform":"openai","credentials":{"access_token":"isolated-oauth-access-token"}}`)
	configureOAuthProfile(t, f.service)
	queuedProfile := f.service.Configuration().Active
	runner := &deferredRunner{}
	f.service.UseTaskRunner(runner)
	f.service.UseOAuthTransport(oauthTransportFunc(func(r *http.Request) (*http.Response, error) {
		var input struct {
			Model string `json:"model"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			return nil, err
		}
		if input.Model != "fixture-sol" {
			t.Errorf("unexpected OAuth model: %s", input.Model)
		}
		return oauthResponse(200, "application/json", `{"status":"completed","output_text":"[\"7\"]","model":"fixture-sol"}`), nil
	}))
	_, err := f.service.Enqueue(context.Background(), modelcheck.Request{AccountIDs: []string{"1"}, Models: []string{"fixture-sol"}})
	if err != nil {
		t.Fatal(err)
	}
	payload := f.service.Configuration().Active.Payload
	payload.SolProfile.CandidateModels = []string{"new-sol", "new-luna", "new-terra"}
	draft, err := f.service.SaveDraft(context.Background(), modelcheck.SaveDraftRequest{ExpectedFingerprint: queuedProfile.Fingerprint, Payload: payload}, "isolated-test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.PublishDraft(context.Background(), modelcheck.PublishRequest{ExpectedFingerprint: draft.Draft.Fingerprint}, "isolated-test"); err != nil {
		t.Fatal(err)
	}
	runner.runs[0](context.Background())
	task := finished(t, f)
	rows := task.Result["tests"].([]map[string]any)
	if rows[0]["verdict"] != "SOL_CONSISTENT" || task.Result["profile_fingerprint"] != queuedProfile.Fingerprint {
		t.Fatalf("queued OAuth task did not preserve its profile: %#v", task.Result)
	}
}

func TestOAuthAccountRejectsInvalidRemoteIdentityOrCredentialsBeforeGeneration(t *testing.T) {
	for _, tc := range []struct {
		name, account, message string
	}{
		{"different stable ID", `{"id":2,"type":"oauth","platform":"openai"}`, "凭据读取失败"},
		{"changed account type", `{"id":1,"type":"apikey","platform":"openai"}`, "账号类型或平台已变化"},
		{"changed platform", `{"id":1,"type":"oauth","platform":"anthropic"}`, "账号类型或平台已变化"},
		{"missing token", `{"id":1,"type":"oauth","platform":"openai","credentials":{}}`, "Access Token 缺失或无效"},
		{"invalid token", `{"id":1,"type":"oauth","platform":"openai","credentials":{"access_token":"invalid\nheader"}}`, "Access Token 缺失或无效"},
		{"configured proxy", `{"id":1,"type":"oauth","platform":"openai","proxy_id":7}`, "配置了出站代理"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, _ := oauthAccountFixture(t, tc.account)
			f.service.UseOAuthTransport(oauthTransportFunc(func(*http.Request) (*http.Response, error) {
				t.Error("invalid account reached generation endpoint")
				return oauthResponse(500, "application/json", `{}`), nil
			}))
			_, err := f.service.Enqueue(context.Background(), modelcheck.Request{AccountIDs: []string{"1"}, Models: []string{"gpt-6-astra"}})
			if err != nil {
				t.Fatal(err)
			}
			task := finished(t, f)
			rows := task.Result["tests"].([]map[string]any)
			message, _ := rows[0]["error"].(string)
			if rows[0]["verdict"] != "ERROR" || !strings.Contains(message, tc.message) {
				t.Fatalf("invalid OAuth account result: %#v", rows[0])
			}
		})
	}
}

func TestOAuthAccountRejectsTargetOrAccountChangesWhileQueued(t *testing.T) {
	for _, changed := range []string{"target", "account type"} {
		t.Run(changed, func(t *testing.T) {
			f, private := oauthAccountFixture(t, `{"id":1,"type":"oauth","platform":"openai","credentials":{"access_token":"isolated-oauth-access-token"}}`)
			runner := &deferredRunner{}
			f.service.UseTaskRunner(runner)
			f.service.UseOAuthTransport(oauthTransportFunc(func(*http.Request) (*http.Response, error) {
				t.Error("changed target or account reached generation endpoint")
				return oauthResponse(500, "application/json", `{}`), nil
			}))
			_, err := f.service.Enqueue(context.Background(), modelcheck.Request{AccountIDs: []string{"1"}, Models: []string{"gpt-6-astra"}})
			if err != nil {
				t.Fatal(err)
			}
			if changed == "target" {
				private.target.AdminKey = "changed-admin-key"
			} else {
				accountType := "apikey"
				f.catalog.details["1"].AccountType = &accountType
			}
			runner.runs[0](context.Background())
			task := finished(t, f)
			if task.Status != "failed" || !strings.Contains(task.Message, "已变化") {
				t.Fatalf("changed configuration was accepted: %#v", task)
			}
		})
	}
}

func TestOAuthAccountRejectsClaudeModelWithoutSendingGeneration(t *testing.T) {
	f, _ := oauthAccountFixture(t, `{"id":1,"type":"oauth","platform":"openai","credentials":{"access_token":"isolated-oauth-access-token"}}`)
	f.service.UseOAuthTransport(oauthTransportFunc(func(*http.Request) (*http.Response, error) {
		t.Error("Claude model reached OpenAI OAuth endpoint")
		return oauthResponse(500, "application/json", `{}`), nil
	}))
	_, err := f.service.Enqueue(context.Background(), modelcheck.Request{AccountIDs: []string{"1"}, Models: []string{f.service.Capabilities().ClaudeStandards[0]}})
	if err != nil {
		t.Fatal(err)
	}
	task := finished(t, f)
	rows := task.Result["tests"].([]map[string]any)
	if rows[0]["verdict"] != "ERROR" || !strings.Contains(rows[0]["error"].(string), "不支持 Claude") {
		t.Fatalf("unsupported model result: %#v", rows[0])
	}
}

func TestOAuthAccountFailedOrSensitiveResponsesRemainPrivateFailures(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{
		{"expired authorization", 401, `{"error":{"message":"isolated-oauth-access-token"}}`},
		{"echoed access token", 200, `{"status":"completed","output_text":"isolated-oauth-access-token"}`},
		{"echoed refresh token", 200, `{"status":"completed","output_text":"isolated-refresh-token"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, _ := oauthAccountFixture(t, `{"id":1,"type":"oauth","platform":"openai","credentials":{"access_token":"isolated-oauth-access-token","refresh_token":"isolated-refresh-token"}}`)
			f.service.UseOAuthTransport(oauthTransportFunc(func(*http.Request) (*http.Response, error) {
				return oauthResponse(tc.status, "application/json", tc.body), nil
			}))
			_, err := f.service.Enqueue(context.Background(), modelcheck.Request{AccountIDs: []string{"1"}, Models: []string{"gpt-6-astra"}})
			if err != nil {
				t.Fatal(err)
			}
			task := finished(t, f)
			rows := task.Result["tests"].([]map[string]any)
			if rows[0]["verdict"] != "ERROR" {
				t.Fatalf("failed OAuth request was classified: %#v", rows[0])
			}
			raw, err := json.Marshal(task)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(raw), oauthFixtureToken) || strings.Contains(string(raw), "isolated-refresh-token") {
				t.Error("failed OAuth task exposed credentials")
			}
		})
	}
}

func TestOAuthAccountWithoutKeyBindingRunsRequestedRoundsAndKeepsCredentialsPrivate(t *testing.T) {
	f, _ := oauthAccountFixture(t, `{"id":1,"type":"oauth","platform":"openai","credentials":{"access_token":"isolated-oauth-access-token","refresh_token":"isolated-refresh-token","chatgpt_account_id":"isolated-workspace"}}`)
	var calls atomic.Int32
	f.service.UseOAuthTransport(oauthTransportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != "https://chatgpt.com/backend-api/codex/responses" || r.Header.Get("Authorization") != "Bearer "+oauthFixtureToken || r.Header.Get("ChatGPT-Account-Id") != "isolated-workspace" {
			t.Error("OAuth request lost the official endpoint or bound credential")
		}
		answers := []string{"21", "我无法提供知识截止日期。", "2", "4"}
		answer := answers[(calls.Add(1)-1)%4]
		return oauthResponse(200, "application/json", fmt.Sprintf(`{"status":"completed","output_text":%q}`, answer)), nil
	}))
	_, err := f.service.Enqueue(context.Background(), modelcheck.Request{AccountIDs: []string{"1"}, Models: []string{"gpt-6-astra"}, Rounds: 2, TimeoutSeconds: 5})
	if err != nil {
		t.Fatal(err)
	}
	task := finished(t, f)
	rows, _ := task.Result["tests"].([]map[string]any)
	if len(rows) != 1 || rows[0]["verdict"] != "MATCH" || calls.Load() != 8 {
		t.Fatalf("OAuth account did not complete two rounds: result=%#v requests=%d", task.Result, calls.Load())
	}
	if rows[0]["transport"] != "oauth-direct" || rows[0]["production_path_equivalent"] != false || rows[0]["credentials_persisted"] != false || task.Result["credentials_persisted"] != false {
		t.Fatalf("incorrect OAuth task metadata: %#v", task.Result)
	}
	stored, err := f.tasks.ListBySkill(context.Background(), "sub2api-model-check", 10)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(stored)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{oauthFixtureToken, "isolated-refresh-token", "isolated-workspace", "isolated-admin-key"} {
		if strings.Contains(string(raw), secret) {
			t.Error("task history contains private OAuth data")
		}
	}
}
