package browserlogin

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

type worker struct {
	ctx        context.Context
	factory    OAuthFactory
	mu         sync.Mutex
	oauthID    string
	oauth      OAuthBrowser
	securityID string
	security   SecurityBrowser
	cancel     context.CancelFunc
}

func RunWorker(ctx context.Context, socket string, factory OAuthFactory) error {
	ctx, cancelWorker := context.WithCancel(ctx)
	defer cancelWorker()
	if cleaner, ok := factory.(interface{ PruneOAuthCheckpoints() error }); ok {
		if err := cleaner.PruneOAuthCheckpoints(); err != nil {
			return err
		}
		go func() {
			ticker := time.NewTicker(time.Minute)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					_ = cleaner.PruneOAuthCheckpoints()
				}
			}
		}()
	}
	_ = os.Remove(socket)
	listener, err := net.Listen("unix", socket)
	if err != nil {
		return err
	}
	defer listener.Close()
	defer os.Remove(socket)
	if err = os.Chmod(socket, 0660); err != nil {
		return err
	}
	w := &worker{ctx: ctx, factory: factory}
	defer w.close("")
	server := &http.Server{Handler: w, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 50 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 8192}
	go func() { <-ctx.Done(); _ = server.Close() }()
	err = server.Serve(listener)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
func (w *worker) close(id string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if id != "" && w.oauthID != id && w.securityID != id {
		return
	}
	w.closeLocked()
}

func (w *worker) deleteOAuthSession(id string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.oauthID == "" || w.oauthID != id {
		return ErrSession
	}
	if current, ok := w.oauth.(*chromiumBrowser); ok {
		if err := current.discardAutomaticCheckpoint(); err != nil {
			return err
		}
	}
	w.closeLocked()
	return nil
}

func (w *worker) closeLocked() {
	if w.cancel != nil {
		w.cancel()
	}
	if w.oauth != nil {
		w.oauth.Close()
	}
	if w.security != nil {
		w.security.Close()
	}
	w.oauth = nil
	w.cancel = nil
	w.oauthID = ""
	w.security = nil
	w.securityID = ""
}
func (w *worker) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	response.Header().Set("Content-Type", "application/json")
	response.Header().Set("Cache-Control", "no-store")
	value, err := w.handle(request)
	if err != nil {
		response.WriteHeader(http.StatusConflict)
		value = wireResponse{Error: err.Error()}
		switch {
		case errors.Is(err, ErrOAuthPending):
			value.Code = "oauth_pending"
		case errors.Is(err, ErrOAuthState):
			value.Code = "oauth_state"
		case errors.Is(err, ErrOAuthRejected):
			value.Code = "oauth_rejected"
		case errors.Is(err, ErrSession):
			value.Code = "session"
		case errors.Is(err, ErrAuthPageChanged):
			value.Code = "auth_page_changed"
		case errors.Is(err, ErrSecurityPending):
			value.Code = "security_pending"
		case errors.Is(err, ErrSecurityIdentity):
			value.Code = "security_identity"
		case errors.Is(err, ErrSecurityUncertain):
			value.Code = "security_uncertain"
		case errors.Is(err, ErrSecurityConfirmation):
			value.Code = "security_confirmation"
		case errors.Is(err, ErrOAuthCheckpoint):
			value.Code = "oauth_checkpoint"
		case errors.Is(err, ErrOAuthCheckpointUnsafe):
			value.Code = "oauth_checkpoint_unsafe"
		case errors.Is(err, ErrOAuthRecoveryPaused):
			value.Code = "oauth_recovery_paused"
		}
	}
	_ = json.NewEncoder(response).Encode(value)
}
func (w *worker) handle(r *http.Request) (wireResponse, error) {
	if strings.HasPrefix(r.URL.Path, "/security/") {
		return w.handleSecurity(r)
	}
	if strings.HasPrefix(r.URL.Path, "/oauth/") {
		return w.handleOAuth(r)
	}
	return wireResponse{}, ErrSession
}

func (w *worker) handleOAuth(r *http.Request) (wireResponse, error) {
	if strings.HasPrefix(r.URL.Path, "/oauth/checkpoints/") {
		return w.handleOAuthRecovery(r)
	}
	if r.URL.Path == "/oauth/sessions" && r.Method == http.MethodPost {
		var options OAuthOptions
		if err := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 32768)).Decode(&options); err != nil {
			return wireResponse{}, errors.New("OAuth 启动参数无效")
		}
		if err := options.Validate(); err != nil {
			return wireResponse{}, err
		}
		w.mu.Lock()
		defer w.mu.Unlock()
		if w.oauthID != "" || w.securityID != "" {
			return wireResponse{}, errors.New("验证浏览器正在使用中")
		}
		expires := time.Now().Add(Lifetime)
		if options.Recovery != nil {
			expires = options.Recovery.ExpiresAt
		}
		ctx, cancel := context.WithDeadline(w.ctx, expires)
		stop := context.AfterFunc(r.Context(), cancel)
		browser, err := w.factory.OpenOAuth(ctx, options)
		stop()
		if err == nil {
			err = ctx.Err()
		}
		if err == nil {
			err = r.Context().Err()
		}
		if err != nil {
			cancel()
			if browser != nil {
				browser.Close()
			}
			return wireResponse{}, err
		}
		if browser == nil {
			cancel()
			return wireResponse{}, errors.New("OAuth 验证浏览器启动失败")
		}
		raw := make([]byte, 24)
		if _, err = rand.Read(raw); err != nil {
			cancel()
			browser.Close()
			return wireResponse{}, err
		}
		id := hex.EncodeToString(raw)
		w.oauthID, w.oauth, w.cancel = id, browser, cancel
		go func() { <-ctx.Done(); w.close(id) }()
		return wireResponse{ID: id}, nil
	}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 3 || len(parts) > 4 || parts[0] != "oauth" || parts[1] != "sessions" {
		return wireResponse{}, ErrSession
	}
	if len(parts) == 4 && parts[3] == "suspend" && r.Method == http.MethodPost {
		return w.suspendOAuth(r.Context(), parts[2])
	}
	if len(parts) == 4 && parts[3] == "detach" && r.Method == http.MethodPost {
		w.mu.Lock()
		defer w.mu.Unlock()
		if w.oauthID != parts[2] || w.oauth == nil {
			return wireResponse{}, ErrSession
		}
		w.closeLocked()
		return wireResponse{}, nil
	}
	if r.Method == http.MethodDelete && len(parts) == 3 {
		return wireResponse{}, w.deleteOAuthSession(parts[2])
	}
	w.mu.Lock()
	if w.oauthID != parts[2] || w.oauth == nil {
		w.mu.Unlock()
		return wireResponse{}, ErrSession
	}
	browser := w.oauth
	w.mu.Unlock()
	if len(parts) == 3 && r.Method == http.MethodGet {
		image, err := browser.Screenshot(r.Context())
		return wireResponse{Image: image}, err
	}
	if len(parts) == 4 && r.Method == http.MethodPost {
		switch parts[3] {
		case "input":
			var input Input
			if err := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 32768)).Decode(&input); err != nil {
				return wireResponse{}, errors.New("浏览器操作参数无效")
			}
			if err := input.Validate(); err != nil {
				return wireResponse{}, err
			}
			return wireResponse{}, browser.Input(r.Context(), input)
		case "authorization-code":
			result, err := browser.AuthorizationCode(r.Context())
			return wireResponse{OAuth: &result}, err
		case "auth-page", "auth-action":
			automation, ok := browser.(OAuthAutomation)
			if !ok {
				return wireResponse{}, errors.New("授权浏览器尚不支持自动辅助，请更新 browser 服务")
			}
			if parts[3] == "auth-page" {
				page, err := automation.InspectAuth(r.Context())
				return wireResponse{AuthPage: &page}, err
			}
			var action AuthAction
			if err := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 16384)).Decode(&action); err != nil {
				return wireResponse{}, errors.New("授权辅助参数无效")
			}
			if err := action.Validate(); err != nil {
				return wireResponse{}, err
			}
			return wireResponse{}, automation.ApplyAuth(r.Context(), action)
		}
	}
	return wireResponse{}, errors.New("不支持的 OAuth 浏览器操作")
}
