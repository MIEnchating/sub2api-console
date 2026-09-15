package accountworkbench

import (
	"context"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func (s *Service) SourceProfileIdentity(ctx context.Context, owner string, input SourceProfileIdentityInput) (SourceProfileIdentity, error) {
	if err := validateSourceProfileScope(owner, input.Scope); err != nil {
		return SourceProfileIdentity{}, err
	}
	source := input.Source
	if source.SourceOAuthID != "" {
		if source.ArtifactID != "" || source.Index != nil {
			return SourceProfileIdentity{}, configstore.ErrWorkbenchSourceProfile
		}
		origin, err := s.securityOAuthOrigin(ctx, owner, source.SourceOAuthID)
		if err != nil {
			return SourceProfileIdentity{}, err
		}
		if origin.scope != ScopeLocalExport {
			return SourceProfileIdentity{}, configstore.ErrWorkbenchSourceProfile
		}
		view := SourceProfileIdentity{Scope: input.Scope, Source: source, UserID: origin.identity.UserID, WorkspaceID: origin.workspaceID, Email: origin.identity.Email, ExpiresAt: origin.source.expires.UTC().Format(time.RFC3339Nano)}
		view.SourceRevision, err = exportRevision(view)
		return view, err
	}
	if !validExportID(source.ArtifactID) || source.Index == nil || *source.Index < 0 {
		return SourceProfileIdentity{}, configstore.ErrWorkbenchSourceProfile
	}
	state, err := s.exportStorage()
	if err != nil {
		return SourceProfileIdentity{}, err
	}
	accounts, metadata, err := state.readAccountArtifact(exportHash(owner), workbenchScopeFingerprint(ScopeLocalExport, configstore.TargetSettings{}), source.ArtifactID)
	if err != nil || *source.Index >= len(accounts) {
		return SourceProfileIdentity{}, configstore.ErrWorkbenchSourceProfile
	}
	account := accounts[*source.Index]
	identity, err := securityAccountIdentity(account)
	workspace := stringValue(inputObject(account["credentials"])["chatgpt_account_id"])
	if err != nil || workspace == "" || templateTextHasSecret(workspace, templateSourceSecrets(account, "")) {
		return SourceProfileIdentity{}, browserlogin.ErrSecurityIdentity
	}
	view := SourceProfileIdentity{Scope: input.Scope, Source: source, UserID: identity.UserID, WorkspaceID: workspace, Email: identity.Email, ExpiresAt: metadata.ExpiresAt}
	view.SourceRevision, err = exportRevision(struct {
		View     SourceProfileIdentity
		Metadata ExportMetadata
	}{view, metadata})
	return view, err
}
