package accountworkbench

import (
	"context"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

type SourceProfileReference struct {
	SourceOAuthID string `json:"source_oauth_id,omitempty"`
	ArtifactID    string `json:"artifact_id,omitempty"`
	Index         *int   `json:"index,omitempty"`
}

type SourceProfileIdentityInput struct {
	Scope  ExportScope            `json:"scope"`
	Source SourceProfileReference `json:"source"`
}

type SourceProfileIdentity struct {
	Scope          ExportScope            `json:"scope"`
	Source         SourceProfileReference `json:"source"`
	SourceRevision string                 `json:"source_revision"`
	UserID         string                 `json:"user_id"`
	WorkspaceID    string                 `json:"workspace_id"`
	Email          string                 `json:"email"`
	ExpiresAt      string                 `json:"expires_at"`
}

type SourceProfileView struct {
	ID          string      `json:"id"`
	Scope       ExportScope `json:"scope"`
	UserID      string      `json:"user_id"`
	WorkspaceID string      `json:"workspace_id"`
	Email       string      `json:"email"`
	Revision    int64       `json:"revision"`
	UpdatedAt   string      `json:"updated_at"`
	HasPassword bool        `json:"has_password"`
	HasTOTP     bool        `json:"has_totp"`
	HasProxy    bool        `json:"has_proxy"`
	MailKind    string      `json:"mail_kind,omitempty"`
	SMSProvider string      `json:"sms_provider,omitempty"`
}

type SourceProfileSaveInput struct {
	Scope          ExportScope            `json:"scope"`
	ID             string                 `json:"id,omitempty"`
	Revision       int64                  `json:"revision,omitempty"`
	Source         SourceProfileReference `json:"source,omitempty"`
	SourceRevision string                 `json:"source_revision,omitempty"`
	Login          OAuthLoginInput        `json:"login"`
	Confirmed      bool                   `json:"confirmed"`
}

type SourceProfileSelectionInput struct {
	Scope         ExportScope              `json:"scope"`
	Items         []ProfileExportSelection `json:"items"`
	FreshLogin    bool                     `json:"fresh_login,omitempty"`
	FailedBatchID string                   `json:"failed_batch_id,omitempty"`
}

type SourceProfileExportPreview struct {
	ID        string              `json:"id"`
	Scope     ExportScope         `json:"scope"`
	Kind      ExportKind          `json:"kind"`
	ExpiresAt string              `json:"expires_at"`
	Items     []SourceProfileView `json:"items"`
}

type sourceProfileStore interface {
	WorkbenchSourceProfiles(context.Context, string, string) ([]configstore.WorkbenchSourceProfile, error)
	WorkbenchSourceProfile(context.Context, string, string, string) (configstore.WorkbenchSourceProfile, error)
	SaveWorkbenchSourceProfile(context.Context, configstore.WorkbenchSourceProfile) (configstore.WorkbenchSourceProfile, error)
	DeleteWorkbenchSourceProfile(context.Context, string, string, string, int64) error
}
