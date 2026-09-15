package accountworkbench

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskrunner"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

type OAuthView struct {
	ID              string      `json:"id"`
	TaskID          string      `json:"task_id"`
	Host            string      `json:"host"`
	Status          string      `json:"status"`
	Message         string      `json:"message"`
	ExpiresAt       string      `json:"expires_at"`
	Image           string      `json:"image,omitempty"`
	Width           int         `json:"width"`
	Height          int         `json:"height"`
	Scope           ExportScope `json:"scope"`
	RecoveryEnabled bool        `json:"recovery_enabled,omitempty"`
	CheckpointID    string      `json:"checkpoint_id,omitempty"`
}

type oauthSession struct {
	assist            *oauthAssist
	smsOriginTaskID   string
	callbacks         oauthCallbacks
	mu                sync.Mutex
	op                sync.Mutex
	owner             string
	scope             ExportScope
	batchID           string
	view              OAuthView
	target            configstore.TargetSettings
	expires           time.Time
	browser           browserlogin.OAuthBrowser
	cancel            context.CancelFunc
	finish            chan struct{}
	credentials       map[string]any
	previewIDs        []string
	timer             *time.Timer
	assistPaused      bool
	done              chan struct{}
	profile           *profileAuthorization
	checkpointSaved   bool
	checkpointID      string
	options           browserlogin.OAuthOptions
	verifier          string
	expectedEmail     string
	expectedWorkspace string
	restore           *browserlogin.OAuthRestoreOptions
	restoreRecord     *configstore.WorkbenchOAuthCheckpoint
	automaticRecord   *configstore.WorkbenchOAuthCheckpoint
	explicitCancel    bool
	exchangeStarted   bool
}

func (s *Service) StartOAuth(ctx context.Context, owner string) (OAuthView, error) {
	return s.StartOAuthWithInput(ctx, owner, OAuthStartInput{})
}

func (s *Service) StartOAuthWithInput(ctx context.Context, owner string, input OAuthStartInput) (OAuthView, error) {
	return s.startOAuthWithInput(ctx, owner, input, "")
}

func (s *Service) startOAuthWithInput(ctx context.Context, owner string, input OAuthStartInput, batchID string, profiles ...*profileAuthorization) (OAuthView, error) {
	s.cleanupMu.RLock()
	defer s.cleanupMu.RUnlock()
	if owner == "" || s.oauthFactory == nil {
		return OAuthView{}, errors.New("授权浏览器尚未配置，请检查 browser 服务")
	}
	assist, err := s.prepareOAuthAssist(input.Login)
	if err != nil {
		return OAuthView{}, err
	}
	launched := false
	defer func() {
		if !launched {
			assist.close()
		}
	}()
	input.Scope, err = normalizeWorkbenchScope(input.Scope)
	if err != nil {
		return OAuthView{}, err
	}
	ctx, target, err := s.bindWorkbenchScope(ctx, input.Scope, nil)
	if err != nil {
		return OAuthView{}, err
	}
	var profile *profileAuthorization
	if len(profiles) > 0 {
		profile = profiles[0]
	}
	if profile != nil {
		if input.Scope == ScopeLocalExport || input.RecoveryEnabled {
			return OAuthView{}, errors.New("独立导出不能使用线上账号绑定资料")
		}
		if err := s.validateReauthorizationProfiles(ctx, target, []*profileAuthorization{profile}); err != nil {
			return OAuthView{}, err
		}
	}
	options, verifier, err := oauthOptions()
	if err != nil {
		return OAuthView{}, err
	}
	proxyURL := input.ProxyURL
	if input.Login != nil && input.Login.ProxyURL != "" {
		if proxyURL != "" && proxyURL != input.Login.ProxyURL {
			return OAuthView{}, errors.New("登录代理配置冲突，请只保留一个代理地址")
		}
		proxyURL = input.Login.ProxyURL
	}
	options.ProxyURL, err = browserlogin.NewProxySessionURL(proxyURL)
	if err != nil {
		return OAuthView{}, err
	}
	id, err := randomID()
	if err != nil {
		return OAuthView{}, err
	}
	if err := s.journalOAuthSMSScoped(owner, id, input.Scope, target, assist); err != nil {
		return OAuthView{}, err
	}
	now := time.Now().UTC()
	if input.RecoveryEnabled {
		if _, supported := s.oauthFactory.(browserlogin.OAuthCheckpointReader); !supported {
			return OAuthView{}, errors.New("授权浏览器尚不支持自动检查点，请更新 browser 服务")
		}
	}
	if _, supported := s.oauthFactory.(browserlogin.OAuthRecoveryFactory); supported && (batchID == "" || input.RecoveryEnabled) && profile == nil {
		lease, leaseErr := randomID()
		if leaseErr != nil {
			return OAuthView{}, leaseErr
		}
		options.Recovery = &browserlogin.OAuthRecoveryBinding{Owner: exportHash(owner), Lease: lease, ExpiresAt: now.Add(browserlogin.Lifetime)}
	}
	if input.RecoveryEnabled {
		if options.Recovery == nil {
			return OAuthView{}, errors.New("授权浏览器尚不支持自动检查点")
		}
		options.Recovery.CheckpointID, err = randomWorkerCheckpointID()
		if err != nil {
			return OAuthView{}, err
		}
		options.Recovery.AutoCheckpoint = true
	}
	view := OAuthView{Scope: input.Scope, ID: id, TaskID: id, Host: "auth.openai.com", Status: "starting", Message: "正在启动授权浏览器", ExpiresAt: now.Add(browserlogin.Lifetime).Format(time.RFC3339Nano), Width: browserlogin.Width, Height: browserlogin.Height}
	value := &oauthSession{owner: owner, scope: input.Scope, batchID: batchID, view: view, target: target, expires: now.Add(browserlogin.Lifetime), finish: make(chan struct{}, 1), done: make(chan struct{}), profile: profile}
	value.callbacks = input.callbacks
	value.smsOriginTaskID = id
	value.options, value.verifier = options, verifier
	if assist != nil {
		value.expectedEmail, value.expectedWorkspace = assist.login.Email, assist.login.WorkspaceID
	}
	s.oauthMu.Lock()
	if s.oauthBusy || (s.oauthBatchID != "" && s.oauthBatchID != batchID) {
		s.oauthMu.Unlock()
		return OAuthView{}, errors.New("授权浏览器正在使用或关闭中，请稍后重试")
	}
	if len(s.oauthSessions) >= 20 {
		s.oauthMu.Unlock()
		return OAuthView{}, errors.New("授权会话已满，请关闭已有会话后重试")
	}
	s.oauthBusy = true
	s.oauthSessions[id] = value
	s.oauthMu.Unlock()
	if input.RecoveryEnabled {
		if err := s.prepareAutomaticCheckpoint(ctx, value); err != nil {
			s.removeOAuth(owner, id)
			s.releaseOAuthBrowser()
			return OAuthView{}, err
		}
		view = value.view
		defer func() {
			if !launched {
				_ = s.discardAutomaticCheckpoint(value)
			}
		}()
	}
	if value.callbacks.beforeLaunch != nil {
		if err := value.callbacks.beforeLaunch(ctx, view); err != nil {
			s.removeOAuth(owner, id)
			s.releaseOAuthBrowser()
			return OAuthView{}, err
		}
	}
	task := taskstore.Task{ID: id, Skill: Skill, Operation: "account-workbench-oauth", Status: "queued", Message: view.Message, CreatedAt: now.Format(time.RFC3339Nano), UpdatedAt: now.Format(time.RFC3339Nano), Result: map[string]any{"phase": "starting"}}
	if err := s.tasks.Save(ctx, task); err != nil {
		s.removeOAuth(owner, id)
		s.releaseOAuthBrowser()
		return OAuthView{}, err
	}
	value.mu.Lock()
	value.timer = time.AfterFunc(time.Until(value.expires), func() { s.removeOAuth(owner, id) })
	value.mu.Unlock()
	if err := taskrunner.GoTask(s.runner, id, func(parent context.Context) { s.runOAuth(parent, value, options, verifier, task, assist) }); err != nil {
		s.removeOAuth(owner, id)
		s.releaseOAuthBrowser()
		taskstore.PersistLaunchFailure(s.tasks, task, err)
		return OAuthView{}, err
	}
	launched = true
	return view, nil
}

func (s *Service) releaseOAuthBrowser() {
	s.oauthMu.Lock()
	s.oauthBusy = false
	s.oauthMu.Unlock()
}

func (s *Service) runOAuth(parent context.Context, value *oauthSession, options browserlogin.OAuthOptions, verifier string, task taskstore.Task, assist *oauthAssist) {
	if assist == nil {
		assist = &oauthAssist{smsOnly: true, attempted: make(map[string]bool)}
	}
	value.mu.Lock()
	value.assist = assist
	value.mu.Unlock()
	defer close(value.done)
	defer s.releaseOAuthBrowser()
	defer func() {
		value.op.Lock()
		defer value.op.Unlock()
		s.closeOAuthAssist(value, &task, assist)
		value.mu.Lock()
		value.assist = nil
		value.mu.Unlock()
	}()
	defer func() {
		persist, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.completeCheckpointRestore(persist, value, false)
		value.mu.Lock()
		value.verifier, value.options, value.restore = "", browserlogin.OAuthOptions{}, nil
		value.mu.Unlock()
	}()
	ctx, cancel := context.WithDeadline(parent, value.expires)
	defer cancel()
	value.mu.Lock()
	value.cancel = cancel
	if value.view.Status == "cancelled" {
		cancel()
	}
	value.mu.Unlock()
	err := ctx.Err()
	if err == nil {
		err = s.validateOAuthScope(ctx, value)
	}
	if err == nil && value.profile != nil {
		err = s.validateReauthorizationProfiles(ctx, value.target, []*profileAuthorization{value.profile})
	}
	if err == nil && assist != nil {
		err = assist.initialize(ctx)
		if err != nil {
			s.finishOAuth(value, &task, "failed", err.Error(), ctx.Err())
			return
		}
	}
	var browser browserlogin.OAuthBrowser
	if err == nil {
		browser, err = s.openOAuthBrowser(ctx, value, options)
	}
	if browser != nil {
		defer s.closeOAuthBrowser(value, browser)
	}
	if err == nil && browser == nil {
		err = errors.New("授权浏览器未就绪")
	}
	if err != nil {
		s.finishOAuth(value, &task, "failed", "授权浏览器启动失败，请检查 browser 服务后重试", err)
		return
	}
	defer func() { value.mu.Lock(); value.browser = nil; value.mu.Unlock() }()
	value.mu.Lock()
	value.browser = browser
	value.mu.Unlock()
	if err = s.oauthWaiting(ctx, value, &task, "等待完成账号授权"); err != nil {
		s.finishOAuth(value, &task, "failed", "授权任务状态保存失败，请重试", err)
		return
	}
	ticks := s.oauthAssistTicks
	if ticks == nil {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		ticks = ticker.C
	}
	for {
		select {
		case <-ctx.Done():
			s.finishOAuth(value, &task, "cancelled", "授权已关闭", ctx.Err())
			return
		case <-ticks:
			if err := s.tickOAuthAssist(ctx, value, &task, assist); err != nil {
				s.finishOAuth(value, &task, "failed", "授权任务或管理目标已变化，请重新授权", err)
				return
			}
		case <-value.finish:
			value.op.Lock()
			result, readErr := browser.AuthorizationCode(ctx)
			value.op.Unlock()
			if errors.Is(readErr, browserlogin.ErrOAuthPending) {
				if err := s.oauthWaiting(ctx, value, &task, "尚未收到授权回调，请完成登录后重试"); err != nil {
					s.finishOAuth(value, &task, "failed", "授权任务状态保存失败，请重试", err)
					return
				}
				continue
			}
			if readErr != nil || result.State != options.State {
				s.finishOAuth(value, &task, "failed", "授权回调未通过校验，请重新授权", ctx.Err())
				return
			}
			if assist != nil && assist.smsCodeSubmitted {
				assist.smsVerified = true
			}
			if err := s.validateOAuthScope(ctx, value); err != nil {
				s.finishOAuth(value, &task, "failed", "管理目标已变化，请重新授权", ctx.Err())
				return
			}
			if value.profile != nil {
				if err := s.validateReauthorizationProfiles(ctx, value.target, []*profileAuthorization{value.profile}); err != nil {
					s.finishOAuth(value, &task, "failed", "登录资料或绑定账号已变化，未交换授权码，请重新预览授权", ctx.Err())
					return
				}
			}
			task.Status, task.Message, task.Result = "running", "正在验证官方授权", map[string]any{"phase": "verifying"}
			if err := s.tasks.Save(ctx, task); err != nil {
				s.finishOAuth(value, &task, "failed", "授权任务状态保存失败，请重试", err)
				return
			}
			if err := s.revokeCheckpointForExchange(value); err != nil {
				s.finishOAuth(value, &task, "failed", "旧授权检查点撤销失败，未交换授权码，请检查私有存储和 browser 服务", err)
				return
			}
			item, exchangeErr := s.exchangeOAuth(ctx, result.Code, verifier, options.ProxyURL)
			if exchangeErr != nil {
				s.finishOAuth(value, &task, "failed", exchangeErr.Error(), ctx.Err())
				return
			}
			if value.profile != nil {
				identity := value.profile.identity
				if inputText(item.Credentials["chatgpt_user_id"]) != identity.identity.UserID || inputText(item.Credentials["chatgpt_account_id"]) != identity.workspaceID {
					s.finishOAuth(value, &task, "failed", "授权结果的官方用户或工作区与保存资料的稳定账号不一致，请重新登录指定账号", ctx.Err())
					return
				}
			}
			if assist != nil && assist.login.Email != "" && !strings.EqualFold(item.Email, assist.login.Email) {
				s.finishOAuth(value, &task, "failed", "授权结果邮箱与登录邮箱不一致，请重新确认账号", ctx.Err())
				return
			}
			if assist != nil && assist.login.WorkspaceID != "" && inputText(item.Credentials["chatgpt_account_id"]) != assist.login.WorkspaceID {
				s.finishOAuth(value, &task, "failed", "授权结果工作区与指定工作区不一致，请重新选择工作区", ctx.Err())
				return
			}
			if value.expectedEmail != "" && !strings.EqualFold(item.Email, value.expectedEmail) || value.expectedWorkspace != "" && inputText(item.Credentials["chatgpt_account_id"]) != value.expectedWorkspace {
				s.finishOAuth(value, &task, "failed", "恢复授权的邮箱或工作区与原检查点不一致，请重新授权", ctx.Err())
				return
			}
			if value.callbacks.onAuthorized != nil {
				persist, done := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
				err := value.callbacks.onAuthorized(persist, item.Credentials)
				done()
				if err != nil {
					s.finishOAuth(value, &task, "failed", "授权结果未可靠保存，已停止后续账号", err)
					return
				}
			}
			if value.profile != nil {
				if err := s.validateReauthorizationProfiles(ctx, value.target, []*profileAuthorization{value.profile}); err != nil {
					s.finishOAuth(value, &task, "failed", "登录资料或绑定账号在授权期间已变化，请重新预览授权", ctx.Err())
					return
				}
			}
			if err := s.validateOAuthScope(ctx, value); err != nil {
				s.finishOAuth(value, &task, "failed", "管理目标已变化，请重新授权", ctx.Err())
				return
			}
			value.mu.Lock()
			if ctx.Err() == nil && value.view.Status != "cancelled" {
				value.credentials = item.Credentials
			}
			value.mu.Unlock()
			message := "授权完成，等待确认导入"
			if value.scope == ScopeLocalExport {
				message = "授权完成，等待生成独立私有文件"
			}
			s.finishOAuth(value, &task, "authorized", message, ctx.Err())
			return
		}
	}
}

func (s *Service) oauthWaiting(ctx context.Context, value *oauthSession, task *taskstore.Task, message string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	value.mu.Lock()
	if value.view.Status != "cancelled" {
		value.view.Status, value.view.Message = "waiting", message
	}
	value.mu.Unlock()
	task.Status, task.Message, task.Result = "waiting_input", message, map[string]any{"phase": "waiting"}
	task.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	return s.tasks.Save(ctx, *task)
}

func (s *Service) finishOAuth(value *oauthSession, task *taskstore.Task, status, message string, cause error) {
	value.mu.Lock()
	defer value.mu.Unlock()
	if errors.Is(cause, context.DeadlineExceeded) || time.Now().After(value.expires) {
		status, message = "expired", "授权会话已过期，请重新授权"
	} else if errors.Is(cause, context.Canceled) || value.view.Status == "cancelled" {
		status, message = "cancelled", "授权已关闭"
	}
	if status != "authorized" {
		value.credentials = nil
	}
	value.view.Status, value.view.Message = status, message
	task.Status = "failed"
	if status == "authorized" {
		task.Status = "succeeded"
	}
	if status == "cancelled" {
		task.Status = "cancelled"
	}
	task.Message, task.Progress, task.Result = message, 100, map[string]any{"phase": status}
	if value.checkpointSaved {
		value.view.Message = "授权已暂停，检查点保存状态需要复核"
		task.Result = map[string]any{"phase": "checkpoint-review"}
		if value.checkpointID != "" {
			value.view.Message = "授权已保存检查点，可在到期前恢复"
			task.Result = map[string]any{"phase": "checkpointed", "checkpoint_id": value.checkpointID}
		}
		task.Message = value.view.Message
	}
	if value.automaticRecord != nil && !value.explicitCancel && !value.exchangeStarted && status != "authorized" && status != "expired" && !value.checkpointSaved {
		value.checkpointSaved = true
		value.view.Message = "授权已中断，请查看自动登录检查点后决定是否恢复"
		task.Message = value.view.Message
		task.Result = map[string]any{"phase": "checkpoint-review", "checkpoint_id": value.automaticRecord.ID}
	}
	task.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.tasks.Save(ctx, *task); err != nil {
		slog.Error("账号授权任务结果保存失败", "task_id", task.ID)
		value.credentials = nil
		value.view.Status, value.view.Message = "failed", "授权结果保存失败，请重新授权"
	}
}
