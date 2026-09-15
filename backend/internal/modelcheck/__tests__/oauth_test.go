package modelcheck_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
)

const oauthFixtureToken = "isolated-oauth-access-token"

type oauthTransportFunc func(*http.Request) (*http.Response, error)

func (transport oauthTransportFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return transport(request)
}

type oauthProfileCatalog struct{ *catalog }

func (*oauthProfileCatalog) LoadModelCheckConfiguration(context.Context) ([]byte, error) {
	return nil, nil
}

func (*oauthProfileCatalog) SaveModelCheckConfiguration(context.Context, []byte, string, string) error {
	return nil
}

func oauthService(t *testing.T, transport oauthTransportFunc) *modelcheck.Service {
	t.Helper()
	f := setup(t, 0, "openai", func(http.ResponseWriter, *http.Request) {
		t.Error("OAuth checks must not contact the account catalog endpoint")
	})
	service, err := modelcheck.New(f.tasks, credentials{}, &oauthProfileCatalog{catalog: f.catalog}, credentials{})
	if err != nil {
		t.Fatal(err)
	}
	view := service.Configuration()
	payload := view.Active.Payload
	payload.SolProfile.CandidateModels = []string{"fixture-sol", "fixture-luna", "fixture-terra"}
	payload.SolProfile.Quick = []modelcheck.ProbeDefinition{{
		ID: "quick-number", Kind: "numeric", Question: "Return the number seven.",
		Clusters:  []modelcheck.NumericClusterDefinition{{ID: "seven", Center: 7}},
		Tolerance: &modelcheck.NumericTolerance{Value: 0, Mode: "absolute"},
		Weights:   map[string][]float64{"seven": {0, -10, -10}},
	}}
	payload.SolProfile.Reserve = []modelcheck.ProbeDefinition{{
		ID: "reserve-number", Kind: "numeric", Question: "Return the number eight.",
		Clusters:  []modelcheck.NumericClusterDefinition{{ID: "eight", Center: 8}},
		Tolerance: &modelcheck.NumericTolerance{Value: 0, Mode: "absolute"},
		Weights:   map[string][]float64{"eight": {0, -10, -10}},
	}}
	draft, err := service.SaveDraft(context.Background(), modelcheck.SaveDraftRequest{
		ExpectedFingerprint: view.Active.Fingerprint, Payload: payload,
	}, "isolated-test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.PublishDraft(context.Background(), modelcheck.PublishRequest{
		ExpectedFingerprint: draft.Draft.Fingerprint,
	}, "isolated-test"); err != nil {
		t.Fatal(err)
	}
	service.UseOAuthTransport(transport)
	return service
}

func oauthResponse(status int, contentType, body string) *http.Response {
	return &http.Response{
		StatusCode: status, Header: http.Header{"Content-Type": {contentType}},
		Body: io.NopCloser(strings.NewReader(body)),
	}
}

func TestOAuthCheckSendsBoundTokenOnlyToOfficialEndpoint(t *testing.T) {
	var calls atomic.Int32
	service := oauthService(t, func(request *http.Request) (*http.Response, error) {
		calls.Add(1)
		if request.Method != http.MethodPost || request.URL.String() != "https://chatgpt.com/backend-api/codex/responses" {
			t.Error("OAuth credential destination changed")
		}
		for key, expected := range map[string]string{
			"Authorization": "Bearer " + oauthFixtureToken, "ChatGPT-Account-Id": "workspace-fixture",
			"User-Agent": "fixture-codex-client", "Accept": "text/event-stream", "Content-Type": "application/json",
			"OpenAI-Beta": "responses=experimental", "Originator": "codex_cli_rs",
		} {
			if request.Header.Get(key) != expected {
				t.Errorf("OAuth header %s did not preserve the protocol contract", key)
			}
		}
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Error(err)
			return nil, err
		}
		if body["model"] != "fixture-sol" || body["stream"] != true || body["store"] != false || body["temperature"] != nil || body["max_output_tokens"] != nil {
			t.Error("OAuth Responses payload was not adapted")
		}
		if body["instructions"] != "Follow the output contract exactly. Do not explain, add markdown, or repeat the questions." || body["reasoning"].(map[string]any)["effort"] != "none" {
			t.Error("behavior profile instructions changed")
		}
		input := body["input"].([]any)[0].(map[string]any)
		if input["role"] != "user" || !strings.Contains(input["content"].(string), "Return the number seven.") {
			t.Error("active behavior profile was not used")
		}
		if _, ok := request.Context().Deadline(); !ok {
			t.Error("OAuth request has no timeout")
		}
		return oauthResponse(http.StatusOK, "text/event-stream", "data: {\"type\":\"response.output_text.delta\",\"delta\":\"[\\\"7\\\"]\"}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"model\":\"fixture-sol\"}}\n\n"), nil
	})
	result, err := service.CheckOAuth(context.Background(), "account-41", "隔离测试账号", map[string]any{
		"access_token": oauthFixtureToken, "refresh_token": "isolated-refresh-token", "chatgpt_account_id": "workspace-fixture",
		"user_agent": "fixture-codex-client", "base_url": "https://credential-destination.example.invalid", "proxy_url": "https://ignored.example.invalid",
	}, "fixture-sol", 5)
	if err != nil || result["verdict"] != "SOL_CONSISTENT" || calls.Load() != 1 {
		t.Fatalf("active OAuth profile check failed: result=%v err=%v calls=%d", result, err, calls.Load())
	}
	if result["profile_fingerprint"] != service.Configuration().Active.Fingerprint || result["credentials_persisted"] != false {
		t.Fatal("OAuth result omitted profile provenance or credential handling")
	}
	raw, _ := json.Marshal(result)
	for _, secret := range []string{oauthFixtureToken, "isolated-refresh-token", "workspace-fixture", "credential-destination.example.invalid"} {
		if strings.Contains(string(raw), secret) {
			t.Fatal("OAuth result exposed private credential data")
		}
	}
}

func TestOAuthCheckAcceptsCompletedJSONAndResponsesStreamVariants(t *testing.T) {
	for _, test := range []struct{ name, contentType, body string }{
		{"JSON output", "application/json", `{"status":"completed","output":[{"content":[{"type":"output_text","text":"[\"7\"]"}]}]}`},
		{"adjacent SSE events", "text/event-stream", "data: {\"type\":\"output_text.delta\",\"delta\":\"[\\\"7\\\"]\"}\ndata: {\"type\":\"response.done\",\"response\":{\"status\":\"completed\"}}\n"},
		{"multiline SSE event", "text/event-stream", "data: {\"type\":\"response.completed\",\ndata: \"response\":{\"status\":\"completed\",\"output_text\":\"[\\\"7\\\"]\"}}\n\n"},
		{"completed output wins over deltas", "text/event-stream", "data: {\"type\":\"response.output_text.delta\",\"delta\":\"[\\\"0\\\"]\"}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"output_text\":\"[\\\"7\\\"]\"}}\n\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			service := oauthService(t, func(*http.Request) (*http.Response, error) {
				return oauthResponse(http.StatusOK, test.contentType, test.body), nil
			})
			result, err := service.CheckOAuth(context.Background(), "41", "测试账号", map[string]any{"access_token": oauthFixtureToken}, "fixture-sol", 5)
			if err != nil || result["verdict"] != "SOL_CONSISTENT" {
				t.Fatalf("valid OAuth response rejected: %v %v", result, err)
			}
		})
	}
}

func TestOAuthCheckRejectsUnsuccessfulOrUnfinishedResponses(t *testing.T) {
	for _, test := range []struct{ name, body string }{
		{"upstream business error", `{"status":"completed","error":{"message":"private upstream detail"},"output_text":"[\"7\"]"}`},
		{"incomplete JSON", `{"status":"incomplete","output_text":"[\"7\"]"}`},
		{"trailing JSON", `{"output_text":"[\"7\"]"} {"extra":true}`},
		{"missing completion", "data: {\"type\":\"response.output_text.delta\",\"delta\":\"[\\\"7\\\"]\"}\n\ndata: [DONE]\n\n"},
		{"failed terminal event", "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"failed\",\"output_text\":\"[\\\"7\\\"]\"}}\n\n"},
		{"stream error after completion", "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"output_text\":\"[\\\"7\\\"]\"}}\n\ndata: {\"type\":\"error\",\"message\":\"private upstream detail\"}\n\n"},
		{"body too large", strings.Repeat("x", (4<<20)+1)},
	} {
		t.Run(test.name, func(t *testing.T) {
			service := oauthService(t, func(*http.Request) (*http.Response, error) {
				return oauthResponse(http.StatusOK, "text/event-stream", test.body), nil
			})
			result, err := service.CheckOAuth(context.Background(), "41", "测试账号", map[string]any{"access_token": oauthFixtureToken}, "fixture-sol", 5)
			if err != nil || result["verdict"] != "ERROR" || result["error"] == nil {
				t.Fatalf("invalid upstream response must produce a failure: %v %v", result, err)
			}
			raw, _ := json.Marshal(result)
			if strings.Contains(string(raw), "private upstream detail") {
				t.Fatal("raw upstream error escaped into the check result")
			}
		})
	}
}

func TestOAuthCheckRejectsInvalidInputBeforeNetwork(t *testing.T) {
	service := oauthService(t, func(*http.Request) (*http.Response, error) {
		t.Error("invalid OAuth input reached the transport")
		return nil, errors.New("unexpected network call")
	})
	for _, test := range []struct {
		name, accountID, model string
		timeout                int
		credentials            map[string]any
	}{
		{"missing token", "41", "fixture-sol", 5, nil},
		{"token header injection", "41", "fixture-sol", 5, map[string]any{"access_token": "bad\r\nheader"}},
		{"workspace header injection", "41", "fixture-sol", 5, map[string]any{"access_token": oauthFixtureToken, "account_id": "bad\nheader"}},
		{"user agent header injection", "41", "fixture-sol", 5, map[string]any{"access_token": oauthFixtureToken, "user_agent": "bad\nheader"}},
		{"missing stable ID", "", "fixture-sol", 5, map[string]any{"access_token": oauthFixtureToken}},
		{"model outside active profile", "41", "gpt-5.6-sol", 5, map[string]any{"access_token": oauthFixtureToken}},
		{"timeout too large", "41", "fixture-sol", 121, map[string]any{"access_token": oauthFixtureToken}},
		{"negative timeout", "41", "fixture-sol", -1, map[string]any{"access_token": oauthFixtureToken}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := service.CheckOAuth(context.Background(), test.accountID, "测试账号", test.credentials, test.model, test.timeout); err == nil {
				t.Fatal("invalid OAuth input was accepted")
			}
		})
	}
}

func TestOAuthCheckDoesNotFollowRedirectsOrExposeStatusResponse(t *testing.T) {
	var calls atomic.Int32
	service := oauthService(t, func(request *http.Request) (*http.Response, error) {
		calls.Add(1)
		if request.URL.Host != "chatgpt.com" {
			t.Error("redirect received OAuth credentials")
			return nil, errors.New("redirect reached an untrusted host")
		}
		response := oauthResponse(http.StatusTemporaryRedirect, "application/json", oauthFixtureToken+" private response body")
		response.Header.Set("Location", "https://untrusted.example.invalid/capture")
		return response, nil
	})
	result, err := service.CheckOAuth(context.Background(), "41", "测试账号", map[string]any{"access_token": oauthFixtureToken}, "fixture-sol", 5)
	if err != nil || result["verdict"] != "ERROR" || calls.Load() != 2 {
		t.Fatalf("redirect must fail the quick and reserve requests: %v %v calls=%d", result, err, calls.Load())
	}
	raw, _ := json.Marshal(result)
	if strings.Contains(string(raw), oauthFixtureToken) || strings.Contains(string(raw), "private response body") {
		t.Fatal("status response leaked credentials")
	}
}

func TestOAuthCheckDropsUnrecognizedResponseModelAndSanitizesTransportErrors(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(fmt.Sprintf("transport error %v", fail), func(t *testing.T) {
			service := oauthService(t, func(*http.Request) (*http.Response, error) {
				if fail {
					return nil, errors.New("private transport error " + oauthFixtureToken)
				}
				return oauthResponse(http.StatusOK, "application/json", fmt.Sprintf(`{"status":"completed","output_text":"[\"7\"]","model":%q}`, oauthFixtureToken)), nil
			})
			result, err := service.CheckOAuth(context.Background(), "41", "测试账号", map[string]any{"access_token": oauthFixtureToken}, "fixture-sol", 5)
			if err != nil {
				t.Fatal(err)
			}
			expectedVerdict := "SOL_CONSISTENT"
			if fail {
				expectedVerdict = "ERROR"
			}
			if result["verdict"] != expectedVerdict {
				t.Fatalf("unexpected transport outcome: %v", result)
			}
			raw, _ := json.Marshal(result)
			if strings.Contains(string(raw), oauthFixtureToken) || strings.Contains(string(raw), "private transport error") {
				t.Fatal("upstream metadata exposed credential or transport details")
			}
		})
	}
}

func TestOAuthCheckUsesActiveModelAndBoundedTimeoutDefaults(t *testing.T) {
	service := oauthService(t, func(request *http.Request) (*http.Response, error) {
		if request.Header.Get("User-Agent") != "codex_cli_rs/0.144.1 (Windows 10.0; x86_64)" {
			t.Error("default Codex OAuth client identity was lost")
		}
		if request.Header.Get("ChatGPT-Account-Id") != "fallback-workspace" {
			t.Error("explicit account_id fallback was not used")
		}
		return nil, context.DeadlineExceeded
	})
	result, err := service.CheckOAuth(context.Background(), "41", "测试账号", map[string]any{
		"access_token": oauthFixtureToken, "account_id": "fallback-workspace",
	}, "", 0)
	if err != nil || result["claimed_model"] != "fixture-sol" || result["verdict"] != "ERROR" || result["error"] != "请求超时" {
		t.Fatalf("default model or timeout outcome was incorrect: %v %v", result, err)
	}
}

func TestOAuthCheckReturnsCallerCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	service := oauthService(t, func(request *http.Request) (*http.Response, error) {
		cancel()
		<-request.Context().Done()
		return nil, request.Context().Err()
	})
	_, err := service.CheckOAuth(ctx, "41", "测试账号", map[string]any{"access_token": oauthFixtureToken}, "fixture-sol", 5)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("caller cancellation was hidden: %v", err)
	}
}
