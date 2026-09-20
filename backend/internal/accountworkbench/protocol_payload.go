package accountworkbench

import (
	"context"
	"errors"
	"html"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

var protocolSessionPattern = regexp.MustCompile(`"session_id"\s*:\s*"(us_[A-Za-z0-9_-]+)"`)
var protocolSessionArrayPattern = regexp.MustCompile(`session_id",\s*"(us_[A-Za-z0-9_-]+)"`)
var protocolSessionReverseInputPattern = regexp.MustCompile(`value="(us_[A-Za-z0-9_-]+)"[^>]+name="session_id"`)
var protocolSessionInputPattern = regexp.MustCompile(`name="session_id"[^>]+value="(us_[A-Za-z0-9_-]+)"`)

func protocolSessionID(body []byte) string {
	decoded := strings.ReplaceAll(html.UnescapeString(string(body)), `\"`, `"`)
	for _, pattern := range []*regexp.Regexp{protocolSessionPattern, protocolSessionArrayPattern, protocolSessionInputPattern, protocolSessionReverseInputPattern} {
		match := pattern.FindStringSubmatch(decoded)
		if len(match) == 2 {
			return match[1]
		}
	}
	return ""
}
func protocolIsCallback(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "http" && u.Host == "localhost:1455" && u.Path == "/auth/callback" && u.User == nil && u.Fragment == "" && u.Opaque == ""
}
func protocolCallback(raw string) (string, string) {
	if !protocolIsCallback(raw) {
		return "", ""
	}
	u, _ := url.Parse(raw)
	q, err := url.ParseQuery(u.RawQuery)
	if err != nil || len(q["code"]) != 1 || len(q["state"]) != 1 || len(q["error"]) > 0 {
		return "", ""
	}
	return q.Get("code"), q.Get("state")
}
func protocolPage(payload map[string]any) string { return text(object(payload["page"])["type"]) }
func protocolPath(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return u.Path
}
func isProtocolMFA(payload map[string]any) bool {
	return protocolPage(payload) == "mfa_challenge" || strings.HasPrefix(protocolPath(protocolURL(payload)), "/mfa-challenge/")
}
func protocolFactorID(payload map[string]any) string {
	session := object(payload["oai-client-auth-session"])
	for _, key := range []string{"mfa_challenge_factors", "mfa_factors"} {
		items, _ := session[key].([]any)
		for _, item := range items {
			factor := object(item)
			if text(factor["factor_type"]) == "totp" {
				if id := text(factor["id"]); id != "" {
					return id
				}
			}
		}
	}
	return ""
}
func protocolWorkspaceID(payload map[string]any) string {
	session := object(payload["oai-client-auth-session"])
	items, _ := session["workspaces"].([]any)
	first := ""
	for _, item := range items {
		workspace := object(item)
		id := text(workspace["id"])
		if id == "" {
			continue
		}
		if text(workspace["kind"]) == "organization" {
			return id
		}
		if first == "" {
			first = id
		}
	}
	return first
}
func (p *protocolClient) selectWorkspace(ctx context.Context, payload map[string]any) (map[string]any, error) {
	id := protocolWorkspaceID(payload)
	if id == "" {
		return payload, nil
	}
	response, err := p.request(ctx, http.MethodPost, protocolAuth+"/api/accounts/workspace/select", protocolJSON(map[string]any{"workspace_id": id}), protocolHeaders(protocolURL(payload)))
	if err != nil {
		return nil, err
	}
	return protocolPayload(response.Body)
}

func (s *Service) exchangeProtocolCode(ctx context.Context, client *protocolClient, value *privateRun, index int, code, verifier string) (map[string]any, error) {
	if code == "" || len(code) > 8192 || strings.ContainsAny(code, "\r\n\x00") {
		return nil, errors.New("官方授权码无效")
	}
	if err := s.executionAllowed(ctx, value); err != nil {
		return nil, err
	}
	value.Phases[value.Public.Items[index].ID] = "exchanging"
	if err := s.persistRun(value); err != nil {
		return nil, errors.New("授权交换前保存失败")
	}
	// Exchange uses the same pinned HTTP session as login and never replays.
	// Credential validation is reused without mutating the shared service transport.
	exchange, stop := context.WithTimeout(context.WithoutCancel(ctx), 35*time.Second)
	defer stop()
	return s.tokenRequestWithTransport(exchange, url.Values{"grant_type": {"authorization_code"}, "code": {code}, "code_verifier": {verifier}, "client_id": {officialClientID}, "redirect_uri": {officialRedirect}}, client.http.Transport)
}
