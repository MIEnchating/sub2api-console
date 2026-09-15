package accountworkbench

import (
	"context"
	"strings"

	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

type SourceProfileSecurityInput struct {
	Scope      ExportScope `json:"scope"`
	Revision   int64       `json:"revision"`
	SecurityID string      `json:"security_id"`
	Confirmed  bool        `json:"confirmed"`
}

func (s *Service) ApplySecurityToSourceProfile(ctx context.Context, owner, id string, input SourceProfileSecurityInput) (SourceProfileView, error) {
	if err := validateSourceProfileScope(owner, input.Scope); err != nil {
		return SourceProfileView{}, err
	}
	if !input.Confirmed || !validExportID(id) || input.Revision < 1 {
		return SourceProfileView{}, configstore.ErrWorkbenchSourceProfile
	}
	security, err := s.securitySession(owner, input.SecurityID)
	if err != nil {
		return SourceProfileView{}, err
	}
	security.mu.Lock()
	view, identity, workspace, storage := security.view, security.expected, security.workspaceID, security.storage
	security.mu.Unlock()
	if view.Scope != ScopeLocalExport || view.Status != "succeeded" || view.ArtifactID == "" || view.AccountID != "" || workspace == "" {
		return SourceProfileView{}, browserlogin.ErrSecurityIdentity
	}
	store, err := s.sourceProfileStore()
	if err != nil {
		return SourceProfileView{}, err
	}
	profile, err := store.WorkbenchSourceProfile(ctx, exportHash(owner), string(ScopeLocalExport), id)
	if err != nil || profile.Revision != input.Revision || profile.WorkspaceID != workspace || !sameSecurityIdentity(browserlogin.SecurityIdentity{UserID: profile.UserID, Email: profile.Email}, identity) {
		return SourceProfileView{}, configstore.ErrWorkbenchSourceProfile
	}
	artifact, err := readSecurityProfileArtifact(storage, view.ArtifactID)
	if err != nil {
		return SourceProfileView{}, err
	}
	if artifact.Scope != ScopeLocalExport || artifact.AccountID != "" || artifact.TargetFingerprint != workbenchScopeFingerprint(ScopeLocalExport, configstore.TargetSettings{}) || artifact.SourceOAuthID != view.SourceOAuthID || artifact.SourceCheckpointID != view.SourceCheckpointID || artifact.SourceOAuthBatchID != view.SourceOAuthBatchID || artifact.WorkspaceID != workspace || artifact.UserID != profile.UserID || !strings.EqualFold(artifact.Email, profile.Email) || artifact.Operation != view.Operation {
		return SourceProfileView{}, configstore.ErrWorkbenchSourceProfile
	}
	items, failures := ParseOAuthLogins("[" + string(profile.Login) + "]")
	if len(failures) != 0 || len(items) != 1 {
		return SourceProfileView{}, configstore.ErrWorkbenchSourceProfile
	}
	login := items[0]
	switch view.Operation {
	case "password":
		if artifact.Password == "" {
			return SourceProfileView{}, ErrSecurityStorage
		}
		login.Password = artifact.Password
	case "totp":
		if artifact.Secret == "" {
			return SourceProfileView{}, ErrSecurityStorage
		}
		login.TOTPSecret = artifact.Secret
	default:
		return SourceProfileView{}, ErrSecurityStorage
	}
	return s.SaveSourceProfile(ctx, owner, SourceProfileSaveInput{Scope: ScopeLocalExport, ID: id, Revision: input.Revision, Login: login, Confirmed: true})
}
