package accountworkbench

import (
	"context"
	"sync"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

type SecurityBatchPreviewInput struct {
	Scope      ExportScope               `json:"scope,omitempty"`
	Sources    []SecuritySourceReference `json:"sources,omitempty"`
	ProxyURL   string                    `json:"proxy_url,omitempty"`
	AccountIDs []string                  `json:"account_ids,omitempty"`
	Operation  string                    `json:"operation"`
	Password   string                    `json:"password,omitempty"`
}

type SecurityBatchRow struct {
	WorkspaceID string                   `json:"workspace_id,omitempty"`
	Source      *SecuritySourceReference `json:"source,omitempty"`
	SecurityID  string                   `json:"security_id,omitempty"`
	Index       int                      `json:"index"`
	AccountID   string                   `json:"account_id,omitempty"`
	UserID      string                   `json:"user_id"`
	Email       string                   `json:"email"`
	Status      string                   `json:"status"`
	Message     string                   `json:"message"`
	ArtifactID  string                   `json:"artifact_id,omitempty"`
}

type SecurityBatchPreview struct {
	Scope     ExportScope        `json:"scope"`
	ID        string             `json:"id"`
	Target    string             `json:"target"`
	Operation string             `json:"operation"`
	ExpiresAt string             `json:"expires_at"`
	Items     []SecurityBatchRow `json:"items"`
	Errors    []InputError       `json:"errors"`
}

type SecurityBatchView struct {
	Scope             ExportScope        `json:"scope"`
	ID                string             `json:"id"`
	TaskID            string             `json:"task_id"`
	Operation         string             `json:"operation"`
	Status            string             `json:"status"`
	Message           string             `json:"message"`
	ExpiresAt         string             `json:"expires_at"`
	CurrentSecurityID string             `json:"current_security_id,omitempty"`
	Completed         int                `json:"completed"`
	Succeeded         int                `json:"succeeded"`
	Items             []SecurityBatchRow `json:"items"`
}

type securityBatchItem struct {
	source      *SecuritySourceReference
	origin      securityOrigin
	accountID   string
	identity    browserlogin.SecurityIdentity
	workspaceID string
}

type preparedSecurityBatch struct {
	proxyURL string
	owner    string
	target   configstore.TargetSettings
	expires  time.Time
	password string
	items    []securityBatchItem
	view     SecurityBatchPreview
}

type securityBatch struct {
	users    map[string]bool
	proxyURL string
	mu       sync.Mutex
	owner    string
	target   configstore.TargetSettings
	expires  time.Time
	password string
	items    []securityBatchItem
	view     SecurityBatchView
	cancel   context.CancelFunc
	timer    *time.Timer
	done     chan struct{}
}

type securityBatches struct {
	mu       sync.Mutex
	previews map[string]*preparedSecurityBatch
	active   map[string]*securityBatch
	closing  map[string]*securityBatch
}

func newSecurityBatches() *securityBatches {
	return &securityBatches{previews: make(map[string]*preparedSecurityBatch), active: make(map[string]*securityBatch), closing: make(map[string]*securityBatch)}
}
