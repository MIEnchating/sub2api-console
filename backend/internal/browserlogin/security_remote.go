package browserlogin

import (
	"context"
	"net/http"
	"strings"
	"time"
)

type remoteSecurityBrowser struct {
	remote *Remote
	id     string
}

type securityRequest struct {
	Identity   SecurityIdentity   `json:"identity"`
	Enrollment SecurityEnrollment `json:"enrollment"`
	Code       string             `json:"code"`
}

func (r *Remote) OpenSecurity(ctx context.Context, options SecurityOptions) (SecurityBrowser, error) {
	if err := options.Validate(); err != nil {
		return nil, err
	}
	value, err := r.call(ctx, http.MethodPost, "/security/sessions", options)
	if err != nil {
		return nil, err
	}
	if len(value.ID) != 48 || strings.ContainsAny(value.ID, "/\\") {
		return nil, ErrSession
	}
	return &remoteSecurityBrowser{remote: r, id: value.ID}, nil
}

func (r *Remote) OpenSecurityFromCheckpoint(ctx context.Context, options SecurityCheckpointOptions) (SecurityBrowser, error) {
	if err := options.Validate(); err != nil {
		return nil, err
	}
	value, err := r.call(ctx, http.MethodPost, "/security/checkpoints/transfer", options)
	if err != nil {
		return nil, err
	}
	if value.ID != options.SessionID {
		return nil, ErrSession
	}
	return &remoteSecurityBrowser{remote: r, id: value.ID}, nil
}

func (b *remoteSecurityBrowser) call(ctx context.Context, path string, payload any) (wireResponse, error) {
	return b.remote.call(ctx, http.MethodPost, "/security/sessions/"+b.id+"/"+path, payload)
}

func (b *remoteSecurityBrowser) Screenshot(ctx context.Context) ([]byte, error) {
	value, err := b.remote.call(ctx, http.MethodGet, "/security/sessions/"+b.id, nil)
	return value.Image, err
}

func (b *remoteSecurityBrowser) Input(ctx context.Context, input Input) error {
	if err := input.Validate(); err != nil {
		return err
	}
	_, err := b.call(ctx, "input", input)
	return err
}

func (b *remoteSecurityBrowser) InspectAuth(ctx context.Context) (AuthPage, error) {
	value, err := b.call(ctx, "auth-page", nil)
	if err != nil {
		return AuthPage{}, err
	}
	if value.AuthPage == nil {
		return AuthPage{}, ErrAuthPageChanged
	}
	return *value.AuthPage, nil
}

func (b *remoteSecurityBrowser) ApplyAuth(ctx context.Context, action AuthAction) error {
	if err := action.Validate(); err != nil {
		return err
	}
	_, err := b.call(ctx, "auth-action", action)
	return err
}

func (b *remoteSecurityBrowser) Identity(ctx context.Context) (SecurityIdentity, error) {
	value, err := b.call(ctx, "identity", nil)
	if err != nil {
		return SecurityIdentity{}, err
	}
	if value.SecurityIdentity == nil || value.SecurityIdentity.Validate() != nil {
		return SecurityIdentity{}, ErrSecurityIdentity
	}
	return *value.SecurityIdentity, nil
}

func (b *remoteSecurityBrowser) ConfirmSecurityIdentity(ctx context.Context, identity SecurityIdentity) error {
	if identity.Validate() != nil {
		return ErrSecurityIdentity
	}
	_, err := b.call(ctx, "identity/confirm", securityRequest{Identity: identity})
	return err
}

func (b *remoteSecurityBrowser) BeginPassword(ctx context.Context, identity SecurityIdentity) error {
	_, err := b.call(ctx, "password/begin", securityRequest{Identity: identity})
	return err
}

func (b *remoteSecurityBrowser) SubmitPassword(ctx context.Context, action AuthAction) error {
	if action.Stage != "new_password" || action.Validate() != nil {
		return ErrAuthPageChanged
	}
	_, err := b.call(ctx, "password/submit", action)
	return err
}

func (b *remoteSecurityBrowser) PasswordResult(ctx context.Context) (bool, error) {
	value, err := b.call(ctx, "password/result", nil)
	if err != nil {
		return false, err
	}
	if value.Enabled == nil {
		return false, ErrSecurityUncertain
	}
	return *value.Enabled, nil
}

func (b *remoteSecurityBrowser) TOTPEnabled(ctx context.Context, identity SecurityIdentity) (bool, error) {
	value, err := b.call(ctx, "totp/status", securityRequest{Identity: identity})
	if err != nil {
		return false, err
	}
	if value.Enabled == nil {
		return false, ErrSecurityUncertain
	}
	return *value.Enabled, nil
}

func (b *remoteSecurityBrowser) EnrollTOTP(ctx context.Context, identity SecurityIdentity) (SecurityEnrollment, error) {
	value, err := b.call(ctx, "totp/enroll", securityRequest{Identity: identity})
	if err != nil {
		return SecurityEnrollment{}, err
	}
	if value.SecurityEnrollment == nil {
		return SecurityEnrollment{}, ErrSecurityUncertain
	}
	return *value.SecurityEnrollment, nil
}

func (b *remoteSecurityBrowser) ActivateTOTP(ctx context.Context, identity SecurityIdentity, enrollment SecurityEnrollment, code string) error {
	_, err := b.call(ctx, "totp/activate", securityRequest{Identity: identity, Enrollment: enrollment, Code: code})
	return err
}

func (b *remoteSecurityBrowser) Close() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, _ = b.remote.call(ctx, http.MethodDelete, "/security/sessions/"+b.id, nil)
}
