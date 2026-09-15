package accountworkbench

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskrunner"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

type SecurityStartInput struct {
	Scope         ExportScope `json:"scope,omitempty"`
	SourceOAuthID string      `json:"source_oauth_id,omitempty"`
	ProxyURL      string      `json:"proxy_url,omitempty"`
	AccountID     string      `json:"account_id,omitempty"`
	Operation     string      `json:"operation"`
	Confirmed     bool        `json:"confirmed"`
	Password      string      `json:"password,omitempty"`
}

type SecurityView struct {
	WorkspaceID        string      `json:"workspace_id,omitempty"`
	SourceOAuthBatchID string      `json:"source_oauth_batch_id,omitempty"`
	SourceIndex        *int        `json:"source_index,omitempty"`
	SourceCheckpointID string      `json:"source_checkpoint_id,omitempty"`
	UserID             string      `json:"user_id,omitempty"`
	IdentityConfirmed  bool        `json:"identity_confirmed,omitempty"`
	Scope              ExportScope `json:"scope"`
	SourceOAuthID      string      `json:"source_oauth_id,omitempty"`
	ID                 string      `json:"id"`
	TaskID             string      `json:"task_id"`
	AccountID          string      `json:"account_id,omitempty"`
	Operation          string      `json:"operation"`
	Email              string      `json:"email"`
	Status             string      `json:"status"`
	Message            string      `json:"message"`
	ExpiresAt          string      `json:"expires_at"`
	Image              string      `json:"image,omitempty"`
	Width              int         `json:"width"`
	Height             int         `json:"height"`
	ArtifactID         string      `json:"artifact_id,omitempty"`
}

type securitySession struct {
	mu       sync.Mutex
	op       sync.Mutex
	owner    string
	password string
	proxyURL string
	expected browserlogin.SecurityIdentity
	target   configstore.TargetSettings
	expires  time.Time
	browser  browserlogin.SecurityBrowser
	view     SecurityView
	cancel   context.CancelFunc
	timer    *time.Timer
	commands chan securityCommand
	done     chan struct{}
	// Only the task goroutine accesses step state. No external write is retried.
	passwordBegun     bool
	passwordSubmitted bool
	artifactID        string
	storage           *securityStorage
	batchExpected     *securityBatchItem
	source            *oauthSession
	workspaceID       string
	checkpoint        *securityCheckpoint
	restartedOAuthID  string
	batchSource       *oauthBatch
	batchSourceIndex  int
	securityBatchID   string
}

type securityCommand struct {
	auth         *browserlogin.AuthAction
	confirmation *browserlogin.SecurityIdentity
}

func validateSecurityInput(input SecurityStartInput) error {
	if err := browserlogin.ValidateProxyURL(input.ProxyURL); err != nil {
		return err
	}
	if !input.Confirmed {
		return errors.New("请确认对所选账号执行安全设置")
	}
	scope, err := normalizeWorkbenchScope(input.Scope)
	if err != nil {
		return err
	}
	if input.SourceOAuthID != "" {
		if input.AccountID != "" || !validExportID(input.SourceOAuthID) {
			return errors.New("请选择一个有效的授权结果或稳定账号 ID，不能同时指定")
		}
	} else if !validTemplateSourceID(input.AccountID) || scope != ScopeManaged {
		return errors.New("请选择有效的稳定账号 ID")
	}
	return validateSecurityOperation(input.Operation, input.Password)
}

func validateSecurityOperation(operation, password string) error {
	input := SecurityStartInput{Operation: operation, Password: password}
	if input.Operation == "totp" && input.Password == "" {
		return nil
	}
	if input.Operation != "password" {
		return errors.New("安全操作类型或参数无效")
	}
	var upper, lower, digit, symbol bool
	for _, r := range input.Password {
		switch {
		case r >= 'A' && r <= 'Z':
			upper = true
		case r >= 'a' && r <= 'z':
			lower = true
		case r >= '0' && r <= '9':
			digit = true
		default:
			symbol = true
		}
	}
	if len(input.Password) < 12 || len(input.Password) > 128 || strings.ContainsAny(input.Password, "\r\n\x00") || !upper || !lower || !digit || !symbol {
		return errors.New("新密码需要 12～128 个字符，并包含大小写字母、数字和符号")
	}
	return nil
}

func (s *Service) StartSecurity(ctx context.Context, owner string, input SecurityStartInput) (SecurityView, error) {
	return s.startSecurity(ctx, owner, input, "", nil)
}

func (s *Service) startSecurity(ctx context.Context, owner string, input SecurityStartInput, batchID string, expected *securityBatchItem) (SecurityView, error) {
	s.cleanupMu.RLock()
	defer s.cleanupMu.RUnlock()
	if err := validateSecurityInput(input); err != nil {
		return SecurityView{}, err
	}
	if owner == "" || s.securityFactory == nil {
		return SecurityView{}, errors.New("安全浏览器尚未配置，请检查 browser 服务")
	}
	s.securityMu.Lock()
	storage := s.securityStorage
	s.securityMu.Unlock()
	if storage == nil || !storage.available() {
		return SecurityView{}, ErrSecurityStorage
	}
	ctx, origin, err := s.prepareSecurityOrigin(ctx, owner, input, expected)
	if err != nil {
		return SecurityView{}, err
	}
	return s.launchSecurity(ctx, owner, input, origin, storage, batchID, expected)
}

func (s *Service) launchSecurity(ctx context.Context, owner string, input SecurityStartInput, origin securityOrigin, storage *securityStorage, batchID string, expected *securityBatchItem) (SecurityView, error) {
	id, err := randomID()
	if err != nil {
		return SecurityView{}, err
	}
	proxyURL, err := browserlogin.NewProxySessionURL(input.ProxyURL)
	if err != nil {
		return SecurityView{}, err
	}
	now := time.Now().UTC()
	expires := now.Add(browserlogin.Lifetime)
	if origin.source != nil && origin.source.expires.Before(expires) {
		expires = origin.source.expires
	}
	if origin.checkpoint != nil && origin.checkpoint.options.Checkpoint.ExpiresAt.Before(expires) {
		expires = origin.checkpoint.options.Checkpoint.ExpiresAt
	}
	if origin.batchSource != nil && origin.batchSource.expires.Before(expires) {
		expires = origin.batchSource.expires
	}
	view := SecurityView{Scope: origin.scope, SourceOAuthID: input.SourceOAuthID, ID: id, TaskID: id, AccountID: input.AccountID, Operation: input.Operation, Email: origin.identity.Email, Status: "starting", Message: "正在启动账号安全浏览器", ExpiresAt: expires.Format(time.RFC3339Nano), Width: browserlogin.Width, Height: browserlogin.Height}
	view.UserID, view.WorkspaceID = origin.identity.UserID, origin.workspaceID
	if origin.checkpoint != nil {
		view.SourceCheckpointID = origin.checkpoint.record.ID
	}
	if origin.batchSource != nil {
		view.SourceOAuthBatchID, view.SourceIndex = origin.batchSource.view.ID, new(origin.batchSourceIndex)
	}
	value := &securitySession{owner: owner, password: input.Password, expected: origin.identity, target: origin.target, expires: expires, view: view, commands: make(chan securityCommand, 1), done: make(chan struct{}), storage: storage, batchExpected: expected, source: origin.source, workspaceID: origin.workspaceID, checkpoint: origin.checkpoint}
	value.proxyURL = proxyURL
	value.batchSource, value.batchSourceIndex, value.securityBatchID = origin.batchSource, origin.batchSourceIndex, batchID
	s.oauthMu.Lock()
	if s.oauthBusy || s.oauthBatchID != batchID {
		s.oauthMu.Unlock()
		return SecurityView{}, errors.New("授权浏览器正在使用或关闭中，请稍后重试")
	}
	s.oauthBusy = true
	s.oauthMu.Unlock()
	s.securityMu.Lock()
	standalone := 0
	for _, session := range s.securitySessions {
		if session.securityBatchID == "" {
			standalone++
		}
	}
	// Batch results remain addressable for profile updates and fresh OAuth.
	if batchID == "" && standalone >= 20 || len(s.securitySessions) >= 20+10*maxInputItems {
		s.securityMu.Unlock()
		s.releaseOAuthBrowser()
		return SecurityView{}, errors.New("安全会话已满，请关闭已有会话后重试")
	}
	s.securitySessions[id] = value
	s.securityMu.Unlock()
	if err := s.claimSecurityCheckpoint(ctx, value); err != nil {
		s.removeSecurity(owner, id)
		s.releaseOAuthBrowser()
		return SecurityView{}, err
	}
	task := taskstore.Task{ID: id, Skill: Skill, Operation: "account-workbench-security-" + input.Operation, Status: "queued", Message: view.Message, CreatedAt: now.Format(time.RFC3339Nano), UpdatedAt: now.Format(time.RFC3339Nano), Result: securityTaskResult(view)}
	if err := s.tasks.Save(ctx, task); err != nil {
		_ = s.completeSecurityCheckpoint(ctx, value, false)
		s.removeSecurity(owner, id)
		s.releaseOAuthBrowser()
		return SecurityView{}, err
	}
	value.mu.Lock()
	value.timer = time.AfterFunc(time.Until(value.expires), func() { s.removeSecurity(owner, id) })
	value.mu.Unlock()
	if err := taskrunner.GoTask(s.runner, id, func(parent context.Context) { s.runSecurity(parent, value, task) }); err != nil {
		_ = s.completeSecurityCheckpoint(ctx, value, false)
		s.removeSecurity(owner, id)
		s.releaseOAuthBrowser()
		taskstore.PersistLaunchFailure(s.tasks, task, err)
		return SecurityView{}, err
	}
	return cloneSecurityView(view), nil
}
