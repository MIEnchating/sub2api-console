package accountworkbench

import (
	"context"
	"sync"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

type WorkbenchRunInput struct {
	Scope            ExportScope    `json:"scope,omitempty"`
	RecoveryEnabled  bool           `json:"recovery_enabled,omitempty"`
	Content          string         `json:"content"`
	TemplateID       string         `json:"template_id,omitempty"`
	CheckAfterImport bool           `json:"check_after_import"`
	Model            string         `json:"model"`
	ExportOnly       bool           `json:"export_only"`
	ProxyURL         string         `json:"proxy_url,omitempty"`
	SMS              *OAuthSMSInput `json:"sms,omitempty"`
}

type WorkbenchRunRow struct {
	Index       int    `json:"index"`
	Kind        string `json:"kind"`
	Name        string `json:"name"`
	Email       string `json:"email,omitempty"`
	WorkspaceID string `json:"workspace_id,omitempty"`
	HasPassword bool   `json:"has_password"`
	HasTOTP     bool   `json:"has_totp"`
	HasProxy    bool   `json:"has_proxy"`
	MailKind    string `json:"mail_kind,omitempty"`
	SMSProvider string `json:"sms_provider,omitempty"`
	Status      string `json:"status"`
	Message     string `json:"message"`
}

type WorkbenchRunPreview struct {
	Scope           ExportScope       `json:"scope,omitempty"`
	RecoveryEnabled bool              `json:"recovery_enabled,omitempty"`
	ID              string            `json:"id"`
	ExpiresAt       string            `json:"expires_at"`
	Target          string            `json:"target"`
	ExportOnly      bool              `json:"export_only"`
	Items           []WorkbenchRunRow `json:"items"`
	Errors          []InputError      `json:"errors"`
}

type WorkbenchRunView struct {
	Scope           ExportScope       `json:"scope,omitempty"`
	RecoveryEnabled bool              `json:"recovery_enabled,omitempty"`
	RecoveryID      string            `json:"recovery_id,omitempty"`
	ID              string            `json:"id"`
	TaskID          string            `json:"task_id"`
	Status          string            `json:"status"`
	Message         string            `json:"message"`
	ExpiresAt       string            `json:"expires_at"`
	ExportOnly      bool              `json:"export_only"`
	OAuthBatchID    string            `json:"oauth_batch_id,omitempty"`
	CurrentOAuthID  string            `json:"current_oauth_id,omitempty"`
	Available       int               `json:"available"`
	Items           []WorkbenchRunRow `json:"items"`
	Errors          []InputError      `json:"errors"`
}

type mixedEntry struct {
	index int
	item  *InputItem
	login *OAuthLoginInput
}

type preparedWorkbenchRun struct {
	owner           string
	target          configstore.TargetSettings
	expires         time.Time
	entries         []mixedEntry
	options         PreviewInput
	oauthPreviewID  string
	oauth           *preparedOAuthBatch
	view            WorkbenchRunPreview
	loadedTemplates map[string]int64
}

type workbenchRun struct {
	mu          sync.Mutex
	prepared    *preparedWorkbenchRun
	view        WorkbenchRunView
	items       []InputItem
	previews    []string
	ready       bool
	cancel      context.CancelFunc
	timer       *time.Timer
	done        chan struct{}
	expires     time.Time
	queue       *configstore.WorkbenchQueue
	queueFrozen bool
	oauthQueue  *oauthQueuePayload
}

type workbenchRuns struct {
	mu       sync.Mutex
	previews map[string]*preparedWorkbenchRun
	active   map[string]*workbenchRun
}

func newWorkbenchRuns() *workbenchRuns {
	return &workbenchRuns{previews: map[string]*preparedWorkbenchRun{}, active: map[string]*workbenchRun{}}
}
