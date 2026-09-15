package browserlogin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

// Remote talks only over the private shared Unix socket, never a browser URL.
type Remote struct{ client *http.Client }
type remoteBrowser struct {
	remote    *Remote
	id        string
	challenge atomic.Pointer[string]
}
type remoteOAuthBrowser struct {
	remote *Remote
	id     string
	state  string
}
type wireResponse struct {
	ID                 string                  `json:"id,omitempty"`
	Image              []byte                  `json:"image,omitempty"`
	ChallengeCode      string                  `json:"challenge_code,omitempty"`
	Record             *configstore.AuthRecord `json:"record,omitempty"`
	Error              string                  `json:"error,omitempty"`
	Code               string                  `json:"code,omitempty"`
	OAuth              *OAuthResult            `json:"oauth,omitempty"`
	AuthPage           *AuthPage               `json:"auth_page,omitempty"`
	SecurityIdentity   *SecurityIdentity       `json:"security_identity,omitempty"`
	SecurityEnrollment *SecurityEnrollment     `json:"security_enrollment,omitempty"`
	Enabled            *bool                   `json:"enabled,omitempty"`
	Checkpoint         *OAuthCheckpoint        `json:"checkpoint,omitempty"`
}

func NewRemote(socket string) *Remote {
	return &Remote{client: &http.Client{Timeout: 45 * time.Second, Transport: &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", socket)
	}}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}
func (r *Remote) call(ctx context.Context, method, path string, payload any) (wireResponse, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return wireResponse{}, err
	}
	req, err := http.NewRequestWithContext(ctx, method, "http://browser-worker"+path, bytes.NewReader(raw))
	if err != nil {
		return wireResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	response, err := r.client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return wireResponse{}, ctx.Err()
		}
		return wireResponse{}, errors.New("验证浏览器服务连接失败，请确认 browser 容器已启动")
	}
	defer response.Body.Close()
	var value wireResponse
	if err := json.NewDecoder(io.LimitReader(response.Body, 12<<20)).Decode(&value); err != nil {
		return value, errors.New("验证浏览器服务响应无效")
	}
	if response.StatusCode != http.StatusOK {
		switch value.Code {
		case "oauth_pending":
			return wireResponse{}, ErrOAuthPending
		case "oauth_state":
			return wireResponse{}, ErrOAuthState
		case "oauth_rejected":
			return wireResponse{}, ErrOAuthRejected
		case "session":
			return wireResponse{}, ErrSession
		case "auth_page_changed":
			return wireResponse{}, ErrAuthPageChanged
		case "security_pending":
			return wireResponse{}, ErrSecurityPending
		case "security_identity":
			return wireResponse{}, ErrSecurityIdentity
		case "security_uncertain":
			return wireResponse{}, ErrSecurityUncertain
		case "security_confirmation":
			return wireResponse{}, ErrSecurityConfirmation
		case "oauth_checkpoint":
			return wireResponse{}, ErrOAuthCheckpoint
		case "oauth_checkpoint_unsafe":
			return wireResponse{}, ErrOAuthCheckpointUnsafe
		case "oauth_recovery_paused":
			return wireResponse{}, ErrOAuthRecoveryPaused
		}
		if value.Error != "" {
			return value, errors.New(value.Error)
		}
		return value, errors.New("验证浏览器操作失败")
	}
	return value, nil
}
func (r *Remote) Open(ctx context.Context, record configstore.AuthRecord) (Browser, error) {
	// Do not send existing credentials to the browser worker.
	candidate := configstore.AuthRecord{Host: record.Host, BaseURL: record.BaseURL, UpstreamType: record.UpstreamType}
	value, err := r.call(ctx, http.MethodPost, "/sessions", candidate)
	if err != nil {
		return nil, err
	}
	if len(value.ID) != 48 || strings.ContainsAny(value.ID, "/\\") {
		return nil, errors.New("验证浏览器会话响应无效")
	}
	return &remoteBrowser{remote: r, id: value.ID}, nil
}
func (b *remoteBrowser) Screenshot(ctx context.Context) ([]byte, error) {
	v, err := b.remote.call(ctx, http.MethodGet, "/sessions/"+b.id, nil)
	if err == nil {
		b.challenge.Store(&v.ChallengeCode)
	}
	return v.Image, err
}
func (b *remoteBrowser) ChallengeCode() string {
	if code := b.challenge.Load(); code != nil {
		return *code
	}
	return ""
}
func (b *remoteBrowser) Input(ctx context.Context, v Input) error {
	_, err := b.remote.call(ctx, http.MethodPost, "/sessions/"+b.id+"/input", v)
	return err
}
func (b *remoteBrowser) Credentials(ctx context.Context) (configstore.AuthRecord, error) {
	v, err := b.remote.call(ctx, http.MethodPost, "/sessions/"+b.id+"/credentials", nil)
	if err != nil {
		return configstore.AuthRecord{}, err
	}
	if v.Record == nil {
		return configstore.AuthRecord{}, errors.New("浏览器尚未取得登录凭据")
	}
	return *v.Record, nil
}
func (b *remoteBrowser) Close() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, _ = b.remote.call(ctx, http.MethodDelete, "/sessions/"+b.id, nil)
}

func (r *Remote) OpenOAuth(ctx context.Context, options OAuthOptions) (OAuthBrowser, error) {
	if err := options.Validate(); err != nil {
		return nil, err
	}
	value, err := r.call(ctx, http.MethodPost, "/oauth/sessions", options)
	if err != nil {
		return nil, err
	}
	if len(value.ID) != 48 || strings.ContainsAny(value.ID, "/\\") {
		return nil, errors.New("OAuth 浏览器会话响应无效")
	}
	return &remoteOAuthBrowser{remote: r, id: value.ID, state: options.State}, nil
}
func (b *remoteOAuthBrowser) Screenshot(ctx context.Context) ([]byte, error) {
	v, err := b.remote.call(ctx, http.MethodGet, "/oauth/sessions/"+b.id, nil)
	return v.Image, err
}
func (b *remoteOAuthBrowser) Input(ctx context.Context, v Input) error {
	_, err := b.remote.call(ctx, http.MethodPost, "/oauth/sessions/"+b.id+"/input", v)
	return err
}
func (b *remoteOAuthBrowser) AuthorizationCode(ctx context.Context) (OAuthResult, error) {
	v, err := b.remote.call(ctx, http.MethodPost, "/oauth/sessions/"+b.id+"/authorization-code", nil)
	if err != nil {
		return OAuthResult{}, err
	}
	if v.OAuth == nil {
		return OAuthResult{}, errors.New("OAuth 浏览器授权结果响应无效")
	}
	if v.OAuth.State != b.state {
		return OAuthResult{}, ErrOAuthState
	}
	if v.OAuth.Code == "" || len(v.OAuth.Code) > 8192 || strings.ContainsFunc(v.OAuth.Code, func(r rune) bool { return r <= ' ' || r == 127 }) {
		return OAuthResult{}, ErrOAuthRejected
	}
	return *v.OAuth, nil
}
func (b *remoteOAuthBrowser) Close() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, _ = b.remote.call(ctx, http.MethodDelete, "/oauth/sessions/"+b.id, nil)
}

func (b *remoteOAuthBrowser) InspectAuth(ctx context.Context) (AuthPage, error) {
	v, err := b.remote.call(ctx, http.MethodPost, "/oauth/sessions/"+b.id+"/auth-page", nil)
	if err != nil {
		return AuthPage{}, err
	}
	if v.AuthPage == nil {
		return AuthPage{}, ErrAuthPageChanged
	}
	return *v.AuthPage, nil
}

func (b *remoteOAuthBrowser) ApplyAuth(ctx context.Context, action AuthAction) error {
	if err := action.Validate(); err != nil {
		return err
	}
	_, err := b.remote.call(ctx, http.MethodPost, "/oauth/sessions/"+b.id+"/auth-action", action)
	return err
}
