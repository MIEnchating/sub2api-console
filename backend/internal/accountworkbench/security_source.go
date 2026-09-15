package accountworkbench

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/mutationguard"
	"github.com/MIEnchating/sub2api-console/backend/internal/targetguard"
)

type SecuritySourceView struct {
	SourceOAuthID string      `json:"source_oauth_id"`
	Scope         ExportScope `json:"scope"`
	Email         string      `json:"email"`
	UserID        string      `json:"user_id"`
	WorkspaceID   string      `json:"workspace_id"`
	ExpiresAt     string      `json:"expires_at"`
}

type securityOrigin struct {
	scope            ExportScope
	target           configstore.TargetSettings
	identity         browserlogin.SecurityIdentity
	workspaceID      string
	source           *oauthSession
	checkpoint       *securityCheckpoint
	batchSource      *oauthBatch
	batchSourceIndex int
}

func (s *Service) SecuritySource(ctx context.Context, owner, id string) (SecuritySourceView, error) {
	origin, err := s.securityOAuthOrigin(ctx, owner, id)
	if err != nil {
		return SecuritySourceView{}, err
	}
	return SecuritySourceView{SourceOAuthID: id, Scope: origin.scope, Email: origin.identity.Email, UserID: origin.identity.UserID, WorkspaceID: origin.workspaceID, ExpiresAt: origin.source.expires.UTC().Format(time.RFC3339Nano)}, nil
}

func (s *Service) securityOAuthOrigin(ctx context.Context, owner, id string) (securityOrigin, error) {
	value, err := s.oauthSession(owner, id)
	if err != nil {
		return securityOrigin{}, err
	}
	value.mu.Lock()
	defer value.mu.Unlock()
	if value.view.Status != "authorized" || value.credentials == nil || !time.Now().Before(value.expires) {
		return securityOrigin{}, browserlogin.ErrSession
	}
	identity, workspace, err := securityCredentialIdentity(value.credentials, value.target)
	if err != nil {
		return securityOrigin{}, err
	}
	if _, _, err := s.bindWorkbenchScope(ctx, value.scope, &value.target); err != nil {
		return securityOrigin{}, err
	}
	return securityOrigin{scope: value.scope, target: value.target, identity: identity, workspaceID: workspace, source: value}, nil
}

func securityCredentialIdentity(credentials map[string]any, target configstore.TargetSettings) (browserlogin.SecurityIdentity, string, error) {
	identity := browserlogin.SecurityIdentity{Email: stringValue(credentials["email"]), UserID: stringValue(credentials["chatgpt_user_id"])}
	workspace := stringValue(credentials["chatgpt_account_id"])
	secrets := templateSourceSecrets(map[string]any{"credentials": credentials}, target.AdminKey)
	if identity.Validate() != nil || workspace == "" || len(workspace) > 256 || strings.ContainsAny(workspace, "\r\n\x00") || templateTextHasSecret(identity.Email, secrets) || templateTextHasSecret(identity.UserID, secrets) || templateTextHasSecret(workspace, secrets) {
		return browserlogin.SecurityIdentity{}, "", browserlogin.ErrSecurityIdentity
	}
	return identity, workspace, nil
}

func (s *Service) prepareSecurityOrigin(ctx context.Context, owner string, input SecurityStartInput, expected *securityBatchItem) (context.Context, securityOrigin, error) {
	scope, err := normalizeWorkbenchScope(input.Scope)
	if err != nil {
		return ctx, securityOrigin{}, err
	}
	if input.SourceOAuthID != "" {
		origin, err := s.securityOAuthOrigin(ctx, owner, input.SourceOAuthID)
		if err != nil {
			return ctx, securityOrigin{}, err
		}
		if origin.scope != scope || expected != nil {
			return ctx, securityOrigin{}, errors.New("安全操作与授权来源范围不一致，请重新选择授权结果")
		}
		ctx, _, err = s.bindWorkbenchScope(ctx, scope, &origin.target)
		return ctx, origin, err
	}
	ctx, err = targetguard.Pin(ctx, s.private)
	if err != nil {
		return ctx, securityOrigin{}, err
	}
	client, target, err := s.client(ctx)
	if err != nil {
		return ctx, securityOrigin{}, err
	}
	account, err := client.Account(ctx, input.AccountID)
	if err != nil {
		return ctx, securityOrigin{}, publicError(err)
	}
	current, err := securityBatchSnapshot(account, target)
	if err != nil {
		return ctx, securityOrigin{}, err
	}
	if current.accountID != input.AccountID || expected != nil && !sameSecurityBatchItem(current, *expected) {
		return ctx, securityOrigin{}, browserlogin.ErrSecurityIdentity
	}
	ctx, err = targetguard.Pin(targetguard.Expect(ctx, target), s.private)
	return ctx, securityOrigin{scope: scope, target: target, identity: current.identity, workspaceID: current.workspaceID}, err
}

func (s *Service) validateSecurityOrigin(ctx context.Context, value *securitySession) error {
	if value.batchSource != nil {
		return s.validateSecurityBatchSource(ctx, value)
	}
	if value.checkpoint != nil {
		return s.validateSecurityCheckpoint(ctx, value)
	}
	if value.source != nil {
		origin, err := s.securityOAuthOrigin(ctx, value.owner, value.view.SourceOAuthID)
		if err != nil {
			return err
		}
		if origin.source != value.source || origin.scope != value.view.Scope || origin.workspaceID != value.workspaceID || !sameSecurityIdentity(origin.identity, value.expected) || workbenchScopeFingerprint(origin.scope, origin.target) != workbenchScopeFingerprint(value.view.Scope, value.target) {
			return browserlogin.ErrSecurityIdentity
		}
		return nil
	}
	_, err := targetguard.Pin(targetguard.Expect(ctx, value.target), s.private)
	return err
}

func (s *Service) securitySourceGuard(ctx context.Context, value *securitySession) (context.Context, func() error, error) {
	resource := "workbench-security-user/" + exportHash(value.expected.UserID)
	var guarded context.Context
	var release func() error
	var err error
	if value.view.Scope == ScopeLocalExport {
		guarded, release, err = mutationguard.Acquire(ctx, s.repository, resource)
	} else {
		guarded, release, err = targetguard.Acquire(targetguard.Expect(ctx, value.target), s.repository, resource)
	}
	if err != nil {
		return nil, nil, err
	}
	// Serialize explicit source cancellation with each concrete official step.
	if value.source != nil {
		value.source.op.Lock()
	}
	if err := s.validateSecurityOrigin(guarded, value); err != nil {
		if value.source != nil {
			value.source.op.Unlock()
		}
		_ = release()
		return nil, nil, err
	}
	return guarded, func() error {
		if value.source != nil {
			value.source.op.Unlock()
		}
		return release()
	}, nil
}
