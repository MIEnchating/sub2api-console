// Package browserlogin provides short-lived, operator-controlled login browsers.
// Only screenshots and bounded input commands cross the console API boundary.
package browserlogin

import (
	"context"
	"encoding/base64"
	"errors"
	"math"
	"net/url"
	"strings"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

const Width = 1100
const Height = 760
const Lifetime = 15 * time.Minute

var ErrSession = errors.New("浏览器验证会话不存在、已过期或不属于当前登录会话")
var ErrOAuthPending = errors.New("OAuth 尚未取得授权回调，请先完成登录")
var ErrOAuthState = errors.New("OAuth 回调 state 不匹配，请重新授权")
var ErrOAuthRejected = errors.New("OAuth 授权回调无效或被上游拒绝，请重新授权")

type View struct {
	ID            string `json:"id"`
	TaskID        string `json:"task_id"`
	Host          string `json:"host"`
	Status        string `json:"status"`
	Message       string `json:"message"`
	ExpiresAt     string `json:"expires_at"`
	Image         string `json:"image,omitempty"`
	ChallengeCode string `json:"challenge_code,omitempty"`
	Width         int    `json:"width"`
	Height        int    `json:"height"`
}

type Input struct {
	Kind  string  `json:"kind"`
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
	Delta float64 `json:"delta"`
	Text  string  `json:"text"`
	Key   string  `json:"key"`
	Shift bool    `json:"shift"`
}

func (v Input) Validate() error {
	finite := func(n float64) bool { return !math.IsNaN(n) && !math.IsInf(n, 0) }
	switch v.Kind {
	case "reload":
		if v.Text == "" && v.Key == "" && v.X == 0 && v.Y == 0 && v.Delta == 0 && !v.Shift {
			return nil
		}
	case "click":
		if finite(v.X) && finite(v.Y) && v.X >= 0 && v.X < Width && v.Y >= 0 && v.Y < Height {
			return nil
		}
	case "scroll":
		if finite(v.Delta) && math.Abs(v.Delta) <= 1500 {
			return nil
		}
	case "text":
		if len(v.Text) > 0 && len(v.Text) <= 4096 {
			return nil
		}
	case "key":
		switch v.Key {
		case "Tab", "Enter", "Backspace", "Delete", "Escape", "ArrowLeft", "ArrowRight", "ArrowUp", "ArrowDown", "Home", "End":
			return nil
		}
	}
	return errors.New("浏览器操作参数无效")
}

type Browser interface {
	Screenshot(context.Context) ([]byte, error)
	Input(context.Context, Input) error
	Credentials(context.Context) (configstore.AuthRecord, error)
	Close()
}

type Factory interface {
	Open(context.Context, configstore.AuthRecord) (Browser, error)
}

// OAuthOptions describes the fixed OpenAI authorization transaction. The
// browser worker validates these fields before opening a page.
type OAuthOptions struct {
	AuthorizationURL string                `json:"authorization_url"`
	State            string                `json:"state"`
	RedirectURI      string                `json:"redirect_uri"`
	ProxyURL         string                `json:"proxy_url,omitempty"`
	Recovery         *OAuthRecoveryBinding `json:"recovery,omitempty"`
}

func (o OAuthOptions) Validate() error {
	if o.Recovery != nil {
		if err := o.Recovery.validate(); err != nil {
			return err
		}
	}
	if err := ValidateProxyURL(o.ProxyURL); err != nil {
		return err
	}
	u, err := url.Parse(o.AuthorizationURL)
	if err != nil || u.Scheme != "https" || (u.Host != "auth.openai.com" && u.Host != "auth.openai.com:443") || u.EscapedPath() != "/oauth/authorize" || u.User != nil || u.Fragment != "" {
		return errors.New("OAuth 授权地址必须是 auth.openai.com 官方地址")
	}
	if o.State == "" || len(o.State) > 512 || strings.ContainsFunc(o.State, func(r rune) bool { return r <= ' ' || r == 127 }) {
		return errors.New("OAuth state 无效")
	}
	if o.RedirectURI != "http://localhost:1455/auth/callback" {
		return errors.New("OAuth 回调地址必须使用固定本机地址")
	}
	query, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return errors.New("OAuth 授权参数格式无效")
	}
	for key, values := range query {
		if len(values) != 1 {
			return errors.New("OAuth 授权参数不得重复")
		}
		switch key {
		case "client_id", "response_type", "code_challenge_method", "state", "redirect_uri", "code_challenge", "scope":
		case "codex_cli_simplified_flow", "id_token_add_organizations":
			if values[0] != "true" {
				return errors.New("OAuth 授权选项不符合固定协议")
			}
		default:
			return errors.New("OAuth 授权包含不支持的参数")
		}
	}
	if query.Get("client_id") != "app_EMoamEEZ73f0CkXaXp7hrann" || query.Get("response_type") != "code" || query.Get("code_challenge_method") != "S256" || query.Get("state") != o.State || query.Get("redirect_uri") != o.RedirectURI {
		return errors.New("OAuth 授权参数不符合固定协议")
	}
	challenge := query.Get("code_challenge")
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(challenge)
	if err != nil || len(decoded) != 32 || len(challenge) != 43 {
		return errors.New("OAuth 授权需要有效的 S256 PKCE")
	}
	scopes := map[string]bool{"openid": false, "profile": false, "email": false, "offline_access": false}
	for _, scope := range strings.Split(query.Get("scope"), " ") {
		seen, allowed := scopes[scope]
		if !allowed || seen {
			return errors.New("OAuth 授权权限范围不符合固定协议")
		}
		scopes[scope] = true
	}
	for _, seen := range scopes {
		if !seen {
			return errors.New("OAuth 授权缺少必要权限范围")
		}
	}
	return nil
}

type OAuthResult struct {
	Code  string `json:"code"`
	State string `json:"state"`
}

type OAuthBrowser interface {
	Screenshot(context.Context) ([]byte, error)
	Input(context.Context, Input) error
	// AuthorizationCode never waits for user input; unfinished login returns ErrOAuthPending.
	AuthorizationCode(context.Context) (OAuthResult, error)
	Close()
}

type OAuthFactory interface {
	OpenOAuth(context.Context, OAuthOptions) (OAuthBrowser, error)
}
type Commit func(context.Context, configstore.AuthRecord) error
