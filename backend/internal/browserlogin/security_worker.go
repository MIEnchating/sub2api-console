package browserlogin

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

func (w *worker) handleSecurity(r *http.Request) (wireResponse, error) {
	if r.URL.Path == "/security/checkpoints/transfer" {
		return w.openSecurityCheckpoint(r)
	}
	if r.URL.Path == "/security/sessions" && r.Method == http.MethodPost {
		return w.openSecurity(r)
	}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 3 || len(parts) > 5 || parts[0] != "security" || parts[1] != "sessions" {
		return wireResponse{}, ErrSession
	}
	w.mu.Lock()
	if w.securityID != parts[2] || w.security == nil {
		w.mu.Unlock()
		return wireResponse{}, ErrSession
	}
	if r.Method == http.MethodDelete && len(parts) == 3 {
		w.closeLocked()
		w.mu.Unlock()
		return wireResponse{}, nil
	}
	browser := w.security
	w.mu.Unlock()
	if r.Method == http.MethodGet && len(parts) == 3 {
		image, err := browser.Screenshot(r.Context())
		return wireResponse{Image: image}, err
	}
	if r.Method != http.MethodPost || len(parts) < 4 {
		return wireResponse{}, ErrSession
	}
	ctx := r.Context()
	operation := strings.Join(parts[3:], "/")
	decode := func(target any) error { return json.NewDecoder(http.MaxBytesReader(nil, r.Body, 16384)).Decode(target) }
	switch operation {
	case "identity/confirm":
		var input securityRequest
		confirmation, ok := browser.(SecurityIdentityConfirmation)
		if !ok || decode(&input) != nil || input.Identity.Validate() != nil {
			return wireResponse{}, ErrSecurityIdentity
		}
		return wireResponse{}, confirmation.ConfirmSecurityIdentity(ctx, input.Identity)
	case "identity":
		value, err := browser.Identity(ctx)
		return wireResponse{SecurityIdentity: &value}, err
	case "auth-page":
		value, err := browser.InspectAuth(ctx)
		return wireResponse{AuthPage: &value}, err
	case "input":
		var input Input
		if err := decode(&input); err != nil || input.Validate() != nil {
			return wireResponse{}, ErrAuthPageChanged
		}
		return wireResponse{}, browser.Input(ctx, input)
	case "auth-action", "password/submit":
		var action AuthAction
		if err := decode(&action); err != nil || action.Validate() != nil {
			return wireResponse{}, ErrAuthPageChanged
		}
		if operation == "password/submit" {
			return wireResponse{}, browser.SubmitPassword(ctx, action)
		}
		if action.Stage == "new_password" {
			return wireResponse{}, ErrAuthPageChanged
		}
		return wireResponse{}, browser.ApplyAuth(ctx, action)
	case "password/result":
		value, err := browser.PasswordResult(ctx)
		return wireResponse{Enabled: &value}, err
	case "password/begin", "totp/status", "totp/enroll", "totp/activate":
		var input securityRequest
		if err := decode(&input); err != nil || input.Identity.Validate() != nil {
			return wireResponse{}, ErrSecurityIdentity
		}
		switch operation {
		case "password/begin":
			return wireResponse{}, browser.BeginPassword(ctx, input.Identity)
		case "totp/status":
			value, err := browser.TOTPEnabled(ctx, input.Identity)
			return wireResponse{Enabled: &value}, err
		case "totp/enroll":
			value, err := browser.EnrollTOTP(ctx, input.Identity)
			return wireResponse{SecurityEnrollment: &value}, err
		case "totp/activate":
			return wireResponse{}, browser.ActivateTOTP(ctx, input.Identity, input.Enrollment, input.Code)
		}
	}
	return wireResponse{}, ErrSession
}

func (w *worker) openSecurity(r *http.Request) (wireResponse, error) {
	factory, ok := w.factory.(SecurityFactory)
	if !ok {
		return wireResponse{}, errors.New("账号安全浏览器尚未配置")
	}
	var options SecurityOptions
	if err := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 16384)).Decode(&options); err != nil || options.Validate() != nil {
		return wireResponse{}, ErrSecurityIdentity
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.oauthID != "" || w.securityID != "" {
		return wireResponse{}, errors.New("验证浏览器正在使用中")
	}
	ctx, cancel := context.WithTimeout(w.ctx, Lifetime)
	stop := context.AfterFunc(r.Context(), cancel)
	b, err := factory.OpenSecurity(ctx, options)
	stop()
	if err == nil {
		err = ctx.Err()
	}
	if err == nil {
		err = r.Context().Err()
	}
	if err != nil || b == nil {
		cancel()
		if b != nil {
			b.Close()
		}
		if err == nil {
			err = ErrSession
		}
		return wireResponse{}, err
	}
	raw := make([]byte, 24)
	if _, err = rand.Read(raw); err != nil {
		cancel()
		b.Close()
		return wireResponse{}, err
	}
	id := hex.EncodeToString(raw)
	w.securityID, w.security, w.cancel = id, b, cancel
	go func() { <-ctx.Done(); w.close(id) }()
	return wireResponse{ID: id}, nil
}
