package accountworkbench

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/adminclient"
	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/targetguard"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskrunner"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/workbenchprovider"
)

const Skill = "account-workbench"

type privateStore interface {
	TargetSettings(context.Context) (configstore.TargetSettings, error)
	WorkbenchTemplates(context.Context, string) ([]configstore.WorkbenchTemplate, error)
	WorkbenchTemplate(context.Context, string, string) (configstore.WorkbenchTemplate, error)
	SaveWorkbenchTemplate(context.Context, configstore.WorkbenchTemplate) error
	DeleteWorkbenchTemplate(context.Context, string, string, int64) error
}

type taskRepository interface {
	Save(context.Context, taskstore.Task) error
	ListBySkill(context.Context, string, int) ([]taskstore.Task, error)
}

type OAuthChecker interface {
	CheckOAuth(context.Context, string, string, map[string]any, string, int) (map[string]any, error)
}

type Service struct {
	private            privateStore
	tasks              taskRepository
	repository         any
	checker            OAuthChecker
	runner             taskrunner.Runner
	transport          http.RoundTripper
	mu                 sync.Mutex
	previews           map[string]*preparedImport
	exportState        *exportState
	maintenanceMu      sync.Mutex
	maintenanceRunning bool
	maintenanceOwner   *maintenanceOwner
	maintenanceTicks   <-chan time.Time
	syncAccounts       func(context.Context, string) (business.ManagementSyncResult, error)
	oauthFactory       browserlogin.OAuthFactory
	oauthTransport     http.RoundTripper
	providerTransport  http.RoundTripper
	oauthAssistTicks   <-chan time.Time
	smsPool            *workbenchprovider.SMSPool
	oauthMu            sync.Mutex
	oauthBusy          bool
	oauthBatchID       string
	oauthSessions      map[string]*oauthSession
	batches            *oauthBatches
	securityFactory    browserlogin.SecurityFactory
	securityStorage    *securityStorage
	securityMu         sync.Mutex
	securitySessions   map[string]*securitySession
	securityBatches    *securityBatches
	mixed              *workbenchRuns
	cleanupMu          sync.RWMutex
	cleanupPreviews    map[string]*preparedCleanup
}

func New(private privateStore, tasks taskRepository, repository any, checker OAuthChecker, runner taskrunner.Runner) *Service {
	return &Service{private: private, tasks: tasks, repository: repository, checker: checker, runner: runner, previews: make(map[string]*preparedImport), oauthSessions: make(map[string]*oauthSession), securitySessions: make(map[string]*securitySession), smsPool: workbenchprovider.NewSMSPool(), batches: newOAuthBatches(), securityBatches: newSecurityBatches(), mixed: newWorkbenchRuns()}
}

func (s *Service) UseOAuthBrowser(factory browserlogin.OAuthFactory) { s.oauthFactory = factory }

func (s *Service) UseOAuthTransport(transport http.RoundTripper) { s.oauthTransport = transport }

func (s *Service) UseProviderTransport(transport http.RoundTripper) { s.providerTransport = transport }

func (s *Service) UseSecurityBrowser(factory browserlogin.SecurityFactory) {
	s.securityFactory = factory
}

// UseOAuthAssistTicks replaces the polling clock for deterministic integration tests.
func (s *Service) UseOAuthAssistTicks(ticks <-chan time.Time) { s.oauthAssistTicks = ticks }

// UseTransport replaces only the external HTTP boundary for isolated tests.
func (s *Service) UseTransport(transport http.RoundTripper) { s.transport = transport }
func (s *Service) UseAccountSync(sync func(context.Context, string) (business.ManagementSyncResult, error)) {
	s.syncAccounts = sync
}

type PreviewInput struct {
	Scope            ExportScope `json:"scope,omitempty"`
	ExportOnly       bool        `json:"export_only,omitempty"`
	Content          string      `json:"content"`
	TemplateID       string      `json:"template_id"`
	CheckAfterImport bool        `json:"check_after_import"`
	Model            string      `json:"model"`
}

type PreviewItem struct {
	ID               string   `json:"id"`
	Index            int      `json:"index"`
	Name             string   `json:"name"`
	Email            string   `json:"email"`
	PlanType         string   `json:"plan_type"`
	TemplateID       string   `json:"template_id"`
	TemplateName     string   `json:"template_name"`
	TemplateRevision int64    `json:"template_revision"`
	GroupIDs         []string `json:"group_ids"`
	Duplicate        bool     `json:"duplicate"`
	AccountID        string   `json:"account_id,omitempty"`
	Action           string   `json:"action,omitempty"`
	RefreshRequired  bool     `json:"refresh_required,omitempty"`
}

type Preview struct {
	Scope            ExportScope   `json:"scope,omitempty"`
	ExportOnly       bool          `json:"export_only,omitempty"`
	ID               string        `json:"id"`
	ExpiresAt        string        `json:"expires_at"`
	Target           string        `json:"target"`
	Items            []PreviewItem `json:"items"`
	Errors           []InputError  `json:"errors"`
	CheckAfterImport bool          `json:"check_after_import"`
	Model            string        `json:"model"`
}

type preparedImport struct {
	maintenanceBatchID string
	maintenanceRetry   bool
	completion         chan taskstore.Task
	lifecycle          context.Context
	profiles           []*profileAuthorization
	owner              string
	expires            time.Time
	target             configstore.TargetSettings
	view               Preview
	items              []InputItem
	templates          []*configstore.WorkbenchTemplate
	execution          *configstore.WorkbenchExecution
	retry              *retryPreparation
}

var ErrPreview = errors.New("导入预览已失效或不属于当前会话，请重新解析")

func (s *Service) client(ctx context.Context) (*adminclient.Client, configstore.TargetSettings, error) {
	target, err := targetguard.Settings(ctx, s.private)
	if err != nil {
		return nil, target, err
	}
	client, err := s.clientFor(target)
	return client, target, err
}

func (s *Service) clientFor(target configstore.TargetSettings) (*adminclient.Client, error) {
	return adminclient.New(adminclient.Config{BaseURL: target.BaseURL, AdminKey: target.AdminKey, Timeout: time.Duration(target.TimeoutSeconds) * time.Second, Attempts: 1}, s.transport)
}

func (s *Service) Preview(ctx context.Context, owner string, input PreviewInput) (Preview, error) {
	if owner == "" {
		return Preview{}, ErrPreview
	}
	if len(input.Content) > 2<<20 {
		return Preview{}, errors.New("单批输入不能超过 2 MiB")
	}
	var err error
	input.Scope, err = normalizeWorkbenchScope(input.Scope)
	if err != nil {
		return Preview{}, err
	}
	if input.Scope == ScopeLocalExport && (!input.ExportOnly || input.TemplateID != "" || input.CheckAfterImport) {
		return Preview{}, errors.New("独立导出只生成私有文件，不能选择线上模板或导入检测")
	}
	items, inputErrors := Parse(input.Content)
	if len(inputErrors) > 0 {
		return Preview{Scope: input.Scope, Items: []PreviewItem{}, Errors: inputErrors, CheckAfterImport: input.CheckAfterImport && !input.ExportOnly, Model: strings.TrimSpace(input.Model), ExportOnly: input.ExportOnly}, nil
	}
	return s.previewItems(ctx, owner, input, items)
}

// previewItems accepts private validated inputs, preserving original indexes
// for mixed batches while sharing the ordinary preview validation boundary.
func (s *Service) previewItems(ctx context.Context, owner string, input PreviewInput, items []InputItem) (Preview, error) {
	if input.Scope == ScopeLocalExport {
		return s.previewLocalItems(ctx, owner, input, items)
	}
	if _, err := normalizeWorkbenchScope(input.Scope); err != nil {
		return Preview{}, err
	}
	view := Preview{Items: []PreviewItem{}, Errors: []InputError{}, CheckAfterImport: input.CheckAfterImport && !input.ExportOnly, Model: strings.TrimSpace(input.Model), ExportOnly: input.ExportOnly}
	if owner == "" {
		return view, ErrPreview
	}
	if len(items) == 0 || len(items) > 500 {
		return view, errors.New("单批需要 1～500 个有效账号")
	}
	if view.Model == "" {
		view.Model = "gpt-5.6-sol"
	}
	if len(view.Model) > 200 || strings.ContainsAny(view.Model, "\r\n") {
		return view, errors.New("检测模型无效")
	}
	ctx, err := targetguard.Pin(ctx, s.private)
	if err != nil {
		return view, err
	}
	client, target, err := s.client(ctx)
	if err != nil {
		return view, err
	}
	view.Target = target.BaseURL
	templates, err := s.private.WorkbenchTemplates(ctx, target.BaseURL)
	if err != nil {
		return view, err
	}
	var accounts []map[string]any
	if !input.ExportOnly {
		accounts, err = client.Accounts(ctx)
		if err != nil {
			return view, publicError(err)
		}
	}
	prepared := &preparedImport{owner: owner, target: target, items: items, expires: time.Now().Add(10 * time.Minute)}
	accountIdentities := newAccountIdentityIndex(accounts)
	identities := make(map[string]bool)
	for itemIndex := range items {
		item := items[itemIndex]
		// Resolve refresh-token-only inputs during preview so stable identity and
		// duplicate detection use the post-refresh account metadata. The
		// materialized credential remains server-side in the short-lived preview.
		if stringValue(item.Credentials["access_token"]) == "" && stringValue(item.Credentials["refresh_token"]) != "" {
			materialized, refreshErr := materialize(ctx, client, item)
			if refreshErr != nil {
				view.Errors = append(view.Errors, InputError{Index: item.Index, Message: "刷新令牌无法换取有效账号身份，请检查令牌后重试"})
				continue
			}
			item = materialized
			items[itemIndex] = item
		}
		identity := inputIdentity(item.Credentials)
		key := identity.key()
		nameContainsCredential := false
		for _, credentialKey := range []string{"access_token", "refresh_token", "id_token"} {
			if secret := stringValue(item.Credentials[credentialKey]); secret != "" && strings.Contains(item.Name, secret) {
				view.Errors = append(view.Errors, InputError{Index: item.Index, Message: "账号名称不能包含授权凭据"})
				nameContainsCredential = true
			}
		}
		if nameContainsCredential {
			continue
		}
		if key != "" && identities[key] {
			return view, fmt.Errorf("第 %d 项与本批其他账号重复，请删除重复项", item.Index+1)
		}
		identities[key] = true
		template, matchErr := MatchTemplate(item, templates, input.TemplateID)
		if matchErr != nil {
			return view, matchErr
		}
		row := PreviewItem{ID: strconv.Itoa(item.Index), Index: item.Index, Name: item.Name, Email: item.Email, PlanType: item.PlanType, GroupIDs: []string{}}
		if template != nil {
			row.TemplateID, row.TemplateName, row.TemplateRevision = template.ID, template.Name, template.Revision
			row.GroupIDs = configIDs(template.Config["group_ids"])
		}
		for _, account := range accountIdentities.candidates(identity) {
			if !identitiesMatch(identity, account.identity) {
				continue
			}
			if row.AccountID != "" {
				return view, errors.New("线上存在重复稳定身份，请先在账号管理核对后再导入")
			}
			row.Duplicate, row.AccountID = true, account.id
		}
		view.Items = append(view.Items, row)
		prepared.templates = append(prepared.templates, template)
	}
	if len(view.Errors) > 0 {
		view.Items = []PreviewItem{}
		return view, nil
	}
	if _, err := targetguard.Pin(targetguard.Expect(ctx, target), s.private); err != nil {
		return Preview{}, err
	}
	view.ID, err = randomID()
	if err != nil {
		return view, err
	}
	view.ExpiresAt = prepared.expires.UTC().Format(time.RFC3339)
	prepared.view = view
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, value := range s.previews {
		if time.Now().After(value.expires) || value.owner == owner {
			delete(s.previews, id)
		}
	}
	if len(s.previews) >= 20 {
		return Preview{}, errors.New("预览任务已满，请稍后重试")
	}
	s.previews[view.ID] = prepared
	time.AfterFunc(time.Until(prepared.expires), func() { s.DeletePreview(owner, view.ID) })
	return view, nil
}

func (s *Service) DeletePreview(owner, id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if item := s.previews[id]; item != nil && item.owner == owner {
		delete(s.previews, id)
	}
}

func (s *Service) History(ctx context.Context) ([]taskstore.Task, error) {
	return s.tasks.ListBySkill(ctx, Skill, 100)
}

func (s *Service) audit(ctx context.Context, id, operation, state string, confirmed bool) error {
	if repo, ok := s.repository.(interface {
		RecordAccountOperation(context.Context, business.AccountOperation) error
	}); ok {
		return repo.RecordAccountOperation(ctx, business.AccountOperation{OperationID: mustID(), OperationType: operation, State: state, Phase: state, Actor: "account-workbench", ObjectID: id, RemoteConfirmed: confirmed, ReadbackConfirmed: confirmed, Writeback: true})
	}
	return nil
}

func publicError(err error) error {
	if err == nil {
		return nil
	}
	var response *adminclient.HTTPError
	if errors.As(err, &response) {
		return fmt.Errorf("管理接口返回 HTTP %d，请检查权限、账号状态后重试", response.StatusCode)
	}
	var unknown *adminclient.CommitUnknownError
	if errors.As(err, &unknown) {
		return errors.New("远端提交结果尚未确定，请核对线上账号和任务记录后再操作")
	}
	if errors.Is(err, context.Canceled) {
		return errors.New("操作已取消，请核对任务中的已完成项目")
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return errors.New("操作超时，请核对远端状态后重试")
	}
	return errors.New("管理接口读取或写入未通过校验，请检查目标站点与账号后重试")
}

func randomID() (string, error) {
	raw := make([]byte, 16)
	_, err := rand.Read(raw)
	return hex.EncodeToString(raw), err
}
func mustID() string { id, _ := randomID(); return id }
func stringValue(value any) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}
func configIDs(raw json.RawMessage) []string {
	var values []any
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.UseNumber()
	if decoder.Decode(&values) != nil {
		return []string{}
	}
	ids := make([]string, 0, len(values))
	for _, value := range values {
		ids = append(ids, stringValue(value))
	}
	return ids
}
