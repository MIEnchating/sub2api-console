package api

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountdelete"
	"github.com/MIEnchating/sub2api-console/backend/internal/accountops"
	"github.com/MIEnchating/sub2api-console/backend/internal/authrecovery"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/config"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/inspection"
	consolelogs "github.com/MIEnchating/sub2api-console/backend/internal/logs"
	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
	"github.com/MIEnchating/sub2api-console/backend/internal/mutationguard"
	"github.com/MIEnchating/sub2api-console/backend/internal/newapimanagement"
	"github.com/MIEnchating/sub2api-console/backend/internal/notification"
	"github.com/MIEnchating/sub2api-console/backend/internal/notificationtarget"
	"github.com/MIEnchating/sub2api-console/backend/internal/onboarding"
	"github.com/MIEnchating/sub2api-console/backend/internal/opstraffic"
	"github.com/MIEnchating/sub2api-console/backend/internal/pricing"
	"github.com/MIEnchating/sub2api-console/backend/internal/probe"
	"github.com/MIEnchating/sub2api-console/backend/internal/redact"
	"github.com/MIEnchating/sub2api-console/backend/internal/routing"
	"github.com/MIEnchating/sub2api-console/backend/internal/routingwrite"
	"github.com/MIEnchating/sub2api-console/backend/internal/runtimepolicy"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/upstreamconfig"
	"github.com/MIEnchating/sub2api-console/backend/internal/upstreamdetect"
	"github.com/MIEnchating/sub2api-console/backend/internal/upstreamsync"
)

const (
	sessionCookie       = "sub2api_console_session"
	sessionTTL          = 12 * time.Hour
	maximumRequestBytes = 2 << 20
)

type Business interface {
	Bootstrap(context.Context) error
	Mode(context.Context) (string, error)
	Ready(context.Context) (bool, error)
	RuntimeSnapshot(context.Context) (business.RuntimeSnapshot, error)
	SetMode(context.Context, string) (business.RuntimeSnapshot, error)
	OverviewSummary(context.Context) (business.OverviewSummary, error)
	EnableNotificationChannel(context.Context, string) error
	SetProbeEnabled(context.Context, bool) error
	ProbeEnabled(context.Context) (bool, error)
	Accounts(context.Context) ([]business.AccountStatus, error)
	Account(context.Context, string) (*business.AccountDetail, error)
	TrafficRanking(context.Context, business.TrafficRankingQuery) (business.TrafficRanking, error)
	Groups(context.Context) ([]business.GroupStatus, error)
	GroupAllocation(context.Context, string) (business.GroupAllocation, error)
	GroupProbeModels(context.Context, string) (business.GroupProbeModels, error)
	ControlPolicy(context.Context) (map[string]any, error)
	PolicySnapshot(context.Context) (business.PolicySnapshot, error)
	UpdatePolicy(context.Context, map[string]any, string) (business.PolicySnapshot, error)
	SetAccountTestModel(context.Context, string, *string, string) error
	UpdateGroupPolicy(context.Context, string, map[string]any, string) (business.GroupStatus, error)
	ClearGroupPolicy(context.Context, string, string) (business.GroupStatus, error)
	SetGroupExcluded(context.Context, string, bool, string) (business.GroupStatus, error)
	Upstreams(context.Context) (business.UpstreamSummary, error)
	UpstreamGroups(context.Context, string, bool) ([]business.UpstreamGroup, error)
	UpstreamGroupHistory(context.Context, string, int) ([]business.UpstreamGroupChange, error)
	Events(context.Context, *int) ([]business.RunEvent, error)
	Alerts(context.Context, *int) ([]business.AlertListItem, error)
	ClearAlerts(context.Context) (int64, error)
	AlertPolicy(context.Context) (business.AlertPolicy, error)
	UpdateAlertPolicy(context.Context, map[string]any) (business.AlertPolicy, error)
	RecordRuntimeEvent(context.Context, string, string, string, map[string]any) (int64, error)
}

type NotificationTester interface {
	Test(context.Context, string, bool) (notification.TestResult, error)
}

type NotificationTargetDiscovery interface {
	Enqueue(context.Context, notificationtarget.Request) (taskstore.Task, error)
	Cancel(string) bool
}

type notificationQueueReader interface {
	NotificationQueueSnapshot(context.Context, string, bool) (business.NotificationQueueSnapshot, error)
}

type notificationQueueDetailReader interface {
	NotificationQueueDetails(context.Context, string, bool) (business.NotificationQueueDetails, error)
}

type notificationQueueStatusReader interface {
	Status() notification.QueueStatus
}

type RequestTraceReader interface {
	RequestTrace(context.Context, string) (business.RequestTrace, error)
}

type SystemLogReader interface {
	SearchSystemLogs(context.Context, opstraffic.SystemLogQuery) (business.SystemLogPage, error)
}

type NewAPIManagementService interface {
	Workspace(context.Context, string) (newapimanagement.Workspace, error)
	SavePlatform(context.Context, newapimanagement.PlatformInput) (configstore.NewAPIPlatformSummary, error)
	DeletePlatform(context.Context, string) (bool, error)
	Refresh(context.Context, string) (newapimanagement.RemoteSnapshot, error)
	ManagementModelPrices(context.Context, string) ([]newapimanagement.Sub2APIModelPrice, error)
	RemoteModelPricingSource(context.Context, string) (newapimanagement.RemotePricingSource, error)
	SaveBindings(context.Context, string, []newapimanagement.GroupBindingInput) ([]business.NewAPIGroupBinding, error)
	CreateChannelKey(context.Context, string, newapimanagement.ChannelKeyInput) (newapimanagement.ChannelKey, error)
	FetchChannelModels(context.Context, string, newapimanagement.ChannelModelsInput) ([]string, error)
	CreateChannel(context.Context, string, newapimanagement.ChannelInput) (map[string]any, error)
	SaveModelPrices(context.Context, string, []newapimanagement.ModelPriceInput) (newapimanagement.RemoteSnapshot, error)
}

type InspectionController interface {
	Status(context.Context) (inspection.Status, error)
	UpdateConfig(context.Context, business.AutoInspectionConfig) (inspection.Status, error)
	Cancel(context.Context) (inspection.Status, bool, error)
	Resume(context.Context) (inspection.Status, error)
	ClearHistory(context.Context) (int64, error)
	Subscribe() (<-chan struct{}, func())
}

type InspectionTaskEnqueuer interface {
	Enqueue(context.Context, inspection.RunRequest) (taskstore.Task, error)
}

type RoutingControlRestorer interface {
	RestoreControl(context.Context, string) (routingwrite.Result, error)
}

type TaskRepository interface {
	Get(context.Context, string) (taskstore.Task, error)
	LatestByOperation(context.Context, string, string) (taskstore.Task, error)
}

type LogReader interface {
	Query(context.Context, consolelogs.Query) (consolelogs.Page, error)
}

type LogMaintenanceController interface {
	Status(context.Context) (consolelogs.CleanupStatus, error)
	Update(context.Context, bool, int) (consolelogs.CleanupStatus, error)
	ClearExpired(context.Context, int) (consolelogs.CleanupResult, error)
}

type AlertTaskEnqueuer interface {
	Enqueue(context.Context) (taskstore.Task, error)
}

type ManagementTaskEnqueuer interface {
	EnqueueSync(context.Context, string) (taskstore.Task, error)
}

type AccountMaintenanceTaskEnqueuer interface {
	EnqueueAccountRateSync(context.Context, []string, string) (taskstore.Task, error)
	EnqueueAccountRevalidation(context.Context, []string, string) (taskstore.Task, error)
	EnqueueAccountBaseURLValidation(context.Context, []string, string) (taskstore.Task, error)
	EnqueueAccountConfigurationCheck(context.Context, []string, string) (taskstore.Task, error)
	EnqueueAccountBaseURLRepair(context.Context, []string, string) (taskstore.Task, error)
	EnqueueAccountUpstreamHostRepair(context.Context, []string, string) (taskstore.Task, error)
	EnqueueAccountNameRepair(context.Context, []string, string) (taskstore.Task, error)
	EnqueueAccountDefaultsRepair(context.Context, []string, string) (taskstore.Task, error)
	EnqueueMissingBindingCleanup(context.Context, []string, string) (taskstore.Task, error)
}

type AccountTaskEnqueuer interface {
	EnqueueControl(context.Context, string, string, string) (taskstore.Task, error)
	EnqueueFields(context.Context, string, accountops.FieldPatch, string) (taskstore.Task, error)
	EnqueueSettings(context.Context, string, accountops.SettingsInput, string) (taskstore.Task, error)
	EnqueueManualPriority(context.Context, string, int64, string, int64, bool, string) (taskstore.Task, error)
	EnqueueClearManualPriority(context.Context, string, string) (taskstore.Task, error)
	Models(context.Context, string) ([]string, error)
}

type AccountDeleteService interface {
	Preview(context.Context, string) (accountdelete.Preview, error)
	PreviewBatch(context.Context, []string) (accountdelete.BatchPreview, error)
	Enqueue(context.Context, string, *accountdelete.Binding, string, string) (taskstore.Task, error)
	EnqueueBatch(context.Context, []accountdelete.Confirmation, string) (taskstore.Task, error)
}

type ProbeTaskEnqueuer interface {
	Enqueue(context.Context, probe.Request, string) (taskstore.Task, error)
}

type ModelCheckService interface {
	Capabilities() modelcheck.Capabilities
	AccountStatuses(context.Context) ([]modelcheck.AccountCheckStatus, error)
	Enqueue(context.Context, modelcheck.Request) (taskstore.Task, error)
}

type PricingService interface {
	Snapshot(context.Context) (pricing.Snapshot, error)
	Changes(context.Context) ([]business.PricingChangeRecord, error)
	UpdateConfig(context.Context, pricing.Config, string) (pricing.Snapshot, error)
	Enqueue(context.Context, string) (taskstore.Task, error)
	EnqueueRevenue(context.Context, pricing.RevenueRequest, string) (taskstore.Task, error)
	CreateBackup(context.Context, string, string) (business.PricingBackup, error)
	Backups(context.Context) ([]business.PricingBackup, error)
	DeleteBackup(context.Context, string) error
	EnqueueRestore(context.Context, string, string) (taskstore.Task, error)
}

type UpstreamDetector interface {
	Detect(context.Context, string) (upstreamdetect.Result, error)
}

type UpstreamConfigurationService interface {
	Get(context.Context, string) (upstreamconfig.Configuration, error)
	Create(context.Context, upstreamconfig.Input, string) (upstreamconfig.Configuration, error)
	Update(context.Context, string, upstreamconfig.Input, string) (upstreamconfig.Configuration, error)
}

type UpstreamSyncTaskEnqueuer interface {
	EnqueueAll(context.Context, upstreamsync.Scope, string, string) (taskstore.Task, error)
	EnqueueHost(context.Context, string, upstreamsync.Scope, string, string) (taskstore.Task, error)
	SyncHost(context.Context, string, upstreamsync.Scope, string) (upstreamsync.HostResult, error)
}

type OnboardingService interface {
	Candidates(context.Context, string) ([]business.OnboardingCandidate, error)
	ProbeModels(context.Context, string, string) ([]string, error)
	Probe(context.Context, string, string, string) (onboarding.ProbeResult, error)
	CancelProbe(context.Context, string, string) error
	PreviewUnboundKeys(context.Context, string) (onboarding.KeyCleanupPreview, error)
	EnqueueKeyCleanup(context.Context, string, []string, string) (taskstore.Task, error)
	Enqueue(context.Context, onboarding.Request) (taskstore.Task, error)
	EnqueueBatch(context.Context, []onboarding.Request) (taskstore.Task, error)
}

type UpstreamDeleteService interface {
	Preview(context.Context, string) (business.UpstreamDeletePreview, error)
	Enqueue(context.Context, string, []string, string) (taskstore.Task, error)
}

type AuthRecoveryService interface {
	VerifyManual(context.Context, authrecovery.ManualInput, string) (authrecovery.ManualResult, error)
	Enqueue(context.Context, string, string, bool, string) (taskstore.Task, error)
	EnqueueBatch(context.Context, []string, string) (taskstore.Task, error)
	SubmitCaptcha(context.Context, string, string, string) (authrecovery.CaptchaCompletion, error)
	CancelCaptcha(string) bool
}

type Dependencies struct {
	Notification       NotificationTester
	NotificationTarget NotificationTargetDiscovery
	Inspection         InspectionController
	InspectionTasks    InspectionTaskEnqueuer
	RoutingControl     RoutingControlRestorer
	Tasks              TaskRepository
	Logs               LogReader
	LogMaintenance     LogMaintenanceController
	AlertTasks         AlertTaskEnqueuer
	ManagementTasks    ManagementTaskEnqueuer
	AccountMaintenance AccountMaintenanceTaskEnqueuer
	AccountTasks       AccountTaskEnqueuer
	AccountDelete      AccountDeleteService
	ProbeTasks         ProbeTaskEnqueuer
	ModelChecks        ModelCheckService
	UpstreamDetect     UpstreamDetector
	UpstreamConfigs    UpstreamConfigurationService
	UpstreamSync       UpstreamSyncTaskEnqueuer
	UpstreamDelete     UpstreamDeleteService
	AuthRecovery       AuthRecoveryService
	Onboarding         OnboardingService
	RequestTrace       RequestTraceReader
	SystemLogs         SystemLogReader
	Pricing            PricingService
	NewAPIManagement   NewAPIManagementService
}

type Server struct {
	config             config.Config
	private            *configstore.Store
	business           Business
	notifier           NotificationTester
	notificationTarget NotificationTargetDiscovery
	inspection         InspectionController
	inspectionTasks    InspectionTaskEnqueuer
	routingControl     RoutingControlRestorer
	tasks              TaskRepository
	logs               LogReader
	logMaintenance     LogMaintenanceController
	alertTasks         AlertTaskEnqueuer
	managementTasks    ManagementTaskEnqueuer
	accountMaintenance AccountMaintenanceTaskEnqueuer
	accountTasks       AccountTaskEnqueuer
	accountDelete      AccountDeleteService
	probeTasks         ProbeTaskEnqueuer
	modelChecks        ModelCheckService
	upstreamDetect     UpstreamDetector
	upstreamConfigs    UpstreamConfigurationService
	upstreamSync       UpstreamSyncTaskEnqueuer
	upstreamDelete     UpstreamDeleteService
	authRecovery       AuthRecoveryService
	onboarding         OnboardingService
	traceReader        RequestTraceReader
	systemLogReader    SystemLogReader
	pricing            PricingService
	newAPIManagement   NewAPIManagementService
	loginThrottle      *loginThrottle
	sseSlots           chan struct{}
	now                func() time.Time
}

const (
	maximumSSEConnections = 100
	taskSSEPollInterval   = time.Second
)

type initializeRequest struct {
	Username     string `json:"username" binding:"required,min=2,max=80"`
	Password     string `json:"password" binding:"required,min=10,max=256"`
	AdminBaseURL string `json:"admin_base_url" binding:"max=2048"`
	AdminKey     string `json:"admin_key" binding:"max=4096"`
}

type loginRequest struct {
	Username string `json:"username" binding:"required,min=2,max=80"`
	Password string `json:"password" binding:"required,min=1,max=256"`
}

type profileRequest struct {
	Username        string  `json:"username" binding:"required,min=2,max=80"`
	CurrentPassword string  `json:"current_password" binding:"required,min=1,max=256"`
	NewPassword     *string `json:"new_password" binding:"omitempty,min=10,max=256"`
}

type sessionStatus struct {
	Authenticated bool    `json:"authenticated"`
	Username      *string `json:"username"`
}

type setupStatusResponse struct {
	Initialized         bool     `json:"initialized"`
	TargetConfigured    bool     `json:"target_configured"`
	SetupTokenRequired  bool     `json:"setup_token_required"`
	ConfigurationErrors []string `json:"configuration_errors"`
}

type runtimeModeRequest struct {
	Mode string `json:"mode" binding:"required"`
}

type adminTargetRequest struct {
	AdminBaseURL          string `json:"admin_base_url" binding:"required,min=1,max=2048"`
	AdminKey              string `json:"admin_key" binding:"max=4096"`
	RequestTimeoutSeconds int    `json:"request_timeout_seconds" binding:"required,min=1,max=120"`
}

type notificationConfigRequest struct {
	AppID           string `json:"app_id" binding:"required,min=1,max=4096"`
	ClientSecret    string `json:"client_secret" binding:"max=65536"`
	HomeChannel     string `json:"home_channel" binding:"required,min=1,max=4096"`
	HomeChannelType string `json:"home_channel_type" binding:"required,oneof=c2c group channel"`
}

type notificationTargetDiscoveryRequest struct {
	AppID        string `json:"app_id" binding:"max=4096"`
	ClientSecret string `json:"client_secret" binding:"max=65536"`
	TargetType   string `json:"target_type" binding:"required,oneof=c2c group channel"`
}

type notificationQueueResponse struct {
	ProducerFiring    int  `json:"producer_firing"`
	ProducerRecovered int  `json:"producer_recovered"`
	ConsumerPending   int  `json:"consumer_pending"`
	ConsumerFailed    int  `json:"consumer_failed"`
	ConsumerActive    bool `json:"consumer_active"`
}

type notificationStatusResponse struct {
	configstore.NotificationStatus
	Queues notificationQueueResponse `json:"queues"`
}

type probeSettingsRequest struct {
	Enabled *bool `json:"enabled" binding:"required"`
}

type accountDefaultsRequest struct {
	Concurrency int64 `json:"concurrency" binding:"required,min=1,max=10000000"`
	Priority    int64 `json:"priority" binding:"required,min=1,max=10000000"`
}

type accountSettingsRequest struct {
	Priority    int64   `json:"priority" binding:"required,min=1,max=10000000"`
	LoadFactor  string  `json:"load_factor" binding:"required,min=1,max=128"`
	Concurrency int64   `json:"concurrency" binding:"required,min=1,max=10000000"`
	TestModel   *string `json:"test_model"`
	Paused      *bool   `json:"paused" binding:"required"`
	Excluded    *bool   `json:"excluded" binding:"required"`
}

type newAPIPlatformRequest struct {
	ID       string `json:"id" binding:"max=64"`
	Name     string `json:"name" binding:"required,min=1,max=120"`
	BaseURL  string `json:"base_url" binding:"required,min=1,max=2048"`
	AdminKey string `json:"admin_key" binding:"max=4096"`
	UserID   string `json:"user_id" binding:"required,min=1,max=128"`
}

type newAPIGroupBindingsRequest struct {
	Bindings []newapimanagement.GroupBindingInput `json:"bindings" binding:"required,max=500,dive"`
}

type newAPIChannelRequest struct {
	Sub2APIGroupID string   `json:"sub2api_group_id" binding:"required,min=1,max=128"`
	KeyID          string   `json:"key_id" binding:"required,min=1,max=128"`
	BaseURL        string   `json:"base_url" binding:"required,min=1,max=2048,url"`
	Models         []string `json:"models" binding:"required,min=1,max=500,dive,required,max=256"`
	NewAPIGroups   []string `json:"newapi_groups" binding:"required,min=1,max=500,dive,required,max=128"`
}

type newAPIChannelModelsRequest struct {
	Sub2APIGroupID string `json:"sub2api_group_id" binding:"required,min=1,max=128"`
	KeyID          string `json:"key_id" binding:"required,min=1,max=128"`
	BaseURL        string `json:"base_url" binding:"required,min=1,max=2048,url"`
}

type newAPIChannelKeyRequest struct {
	Sub2APIGroupID   string `json:"sub2api_group_id" binding:"required,min=1,max=128"`
	CredentialSource string `json:"credential_source" binding:"required,oneof=vault custom"`
	VaultEntry       string `json:"vault_entry" binding:"omitempty,min=1,max=255"`
	Username         string `json:"username" binding:"omitempty,max=65536"`
	Password         string `json:"password" binding:"omitempty,max=65536"`
}

type newAPIModelPricesRequest struct {
	Prices []newapimanagement.ModelPriceInput `json:"prices" binding:"required,min=1,max=1000,dive"`
}

type notificationTestRequest struct {
	Message string `json:"message" binding:"required,min=1,max=4000"`
	DryRun  *bool  `json:"dry_run" binding:"required"`
}

type autoInspectionRequest struct {
	Enabled                        *bool `json:"enabled" binding:"required"`
	IntervalSeconds                int   `json:"interval_seconds" binding:"required,min=15,max=86400"`
	AccountRateSyncIntervalSeconds int   `json:"account_rate_sync_interval_seconds" binding:"omitempty,min=15,max=86400"`
	AccountRateSyncBatchSize       int   `json:"account_rate_sync_batch_size" binding:"min=0,max=100000"`
	AccountRateSyncBatchPercent    int   `json:"account_rate_sync_batch_percent" binding:"min=0,max=100"`
}

type modelCheckRequest struct {
	AccountIDs     []string `json:"account_ids" binding:"required,min=1,max=20,dive,required,max=64"`
	Models         []string `json:"models" binding:"required,min=1,max=20,dive,required,max=256"`
	Rounds         int      `json:"rounds" binding:"min=0,max=3"`
	TimeoutSeconds int      `json:"timeout_seconds" binding:"min=0,max=120"`
}

type logCleanupRequest struct {
	Enabled       *bool `json:"enabled" binding:"required"`
	RetentionDays int   `json:"retention_days" binding:"required,min=1,max=3650"`
}

type overviewResponse struct {
	DatabasePath      string  `json:"database_path"`
	DatabaseAvailable bool    `json:"database_available"`
	AccountCount      int     `json:"account_count"`
	GroupCount        int     `json:"group_count"`
	OpenAlerts        int     `json:"open_alerts"`
	RecentRuns        int     `json:"recent_runs"`
	LastActivity      *string `json:"last_activity"`
	Mode              string  `json:"mode"`
}

type runtimeConfigResponse struct {
	DatabasePath              string   `json:"database_path"`
	DataDatabasePath          string   `json:"data_database_path"`
	DatabaseAvailable         bool     `json:"database_available"`
	DataDatabaseAvailable     bool     `json:"data_database_available"`
	Mode                      string   `json:"mode"`
	ConfigKeys                any      `json:"config_keys"`
	SecretValuesHidden        bool     `json:"secret_values_hidden"`
	ProbesEnabled             bool     `json:"probes_enabled"`
	AdminBaseURL              *string  `json:"admin_base_url"`
	RequestTimeoutSeconds     int      `json:"request_timeout_seconds"`
	AccountDefaultConcurrency int64    `json:"account_default_concurrency"`
	AccountDefaultPriority    int64    `json:"account_default_priority"`
	Initialized               bool     `json:"initialized"`
	TargetConfigured          bool     `json:"target_configured"`
	ConsoleUsername           *string  `json:"console_username"`
	ConfigurationErrors       []string `json:"configuration_errors"`
}

func New(cfg config.Config, private *configstore.Store, business Business, dependencies ...Dependencies) *gin.Engine {
	var services Dependencies
	if len(dependencies) > 0 {
		services = dependencies[0]
	}
	server := &Server{
		config:             cfg,
		private:            private,
		business:           business,
		notifier:           services.Notification,
		notificationTarget: services.NotificationTarget,
		inspection:         services.Inspection,
		inspectionTasks:    services.InspectionTasks,
		routingControl:     services.RoutingControl,
		tasks:              services.Tasks,
		logs:               services.Logs,
		logMaintenance:     services.LogMaintenance,
		alertTasks:         services.AlertTasks,
		managementTasks:    services.ManagementTasks,
		accountMaintenance: services.AccountMaintenance,
		accountTasks:       services.AccountTasks,
		accountDelete:      services.AccountDelete,
		probeTasks:         services.ProbeTasks,
		modelChecks:        services.ModelChecks,
		upstreamDetect:     services.UpstreamDetect,
		upstreamConfigs:    services.UpstreamConfigs,
		upstreamSync:       services.UpstreamSync,
		upstreamDelete:     services.UpstreamDelete,
		authRecovery:       services.AuthRecovery,
		onboarding:         services.Onboarding,
		traceReader:        services.RequestTrace,
		systemLogReader:    services.SystemLogs,
		pricing:            services.Pricing,
		newAPIManagement:   services.NewAPIManagement,
		loginThrottle:      newLoginThrottle(cfg.TrustedProxyCIDRs),
		sseSlots:           make(chan struct{}, maximumSSEConnections),
		now:                time.Now,
	}
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery(), limitRequestBody(), server.cors())
	api := router.Group("/api")
	api.GET("/setup/status", server.setupStatus)
	api.POST("/setup/initialize", server.initialize)
	api.GET("/auth/session", server.authSession)
	api.POST("/auth/login", server.login)
	api.POST("/auth/logout", server.logout)
	authorized := api.Group("")
	authorized.Use(server.authorize())
	authorized.PUT("/profile", server.updateProfile)
	authorized.GET("/health", server.health)
	authorized.GET("/overview", server.overview)
	authorized.GET("/config", server.runtimeConfig)
	authorized.POST("/config/mode", server.updateRuntimeMode)
	authorized.POST("/config/probes", server.updateProbeSettings)
	authorized.POST("/config/account-defaults", server.updateAccountDefaults)
	authorized.POST("/config/target", server.updateAdminTarget)
	authorized.GET("/notifications/status", server.notificationStatus)
	authorized.GET("/notifications/queue", server.notificationQueue)
	authorized.POST("/notifications/config", server.configureNotification)
	authorized.POST("/notifications/test", server.testNotification)
	authorized.POST("/notifications/target-discovery", server.discoverNotificationTarget)
	authorized.DELETE("/notifications/target-discovery/:task_id", server.cancelNotificationTargetDiscovery)
	authorized.GET("/accounts", server.accounts)
	authorized.POST("/accounts/delete-preview", server.accountDeleteBatchPreview)
	authorized.POST("/accounts/delete", server.deleteAccounts)
	authorized.GET("/accounts/:account_id", server.account)
	authorized.GET("/accounts/:account_id/delete-preview", server.accountDeletePreview)
	authorized.POST("/accounts/:account_id/delete", server.deleteAccount)
	authorized.GET("/traffic/ranking", server.trafficRanking)
	authorized.POST("/accounts/:account_id/control", server.setAccountControl)
	authorized.GET("/accounts/:account_id/models", server.accountModels)
	authorized.PUT("/accounts/:account_id/test-model", server.setAccountTestModel)
	authorized.POST("/accounts/:account_id/sync", server.syncAccountFields)
	authorized.PUT("/accounts/:account_id/settings", server.saveAccountSettings)
	authorized.PUT("/accounts/:account_id/manual-priority", server.setAccountManualPriority)
	authorized.DELETE("/accounts/:account_id/manual-priority", server.clearAccountManualPriority)
	authorized.POST("/management/sync", server.syncManagement)
	authorized.POST("/management/accounts/rates/sync", server.syncAccountRates)
	authorized.POST("/management/accounts/revalidate", server.revalidateAccounts)
	authorized.POST("/management/accounts/base-url/validate", server.validateAccountBaseURLs)
	authorized.POST("/management/accounts/configuration/check", server.checkAccountConfiguration)
	authorized.POST("/management/accounts/base-url/repair", server.repairAccountBaseURLs)
	authorized.POST("/management/accounts/upstream-hosts/repair", server.repairAccountUpstreamHosts)
	authorized.POST("/management/accounts/names/repair", server.repairAccountNames)
	authorized.POST("/management/accounts/defaults/repair", server.repairAccountDefaults)
	authorized.POST("/management/accounts/missing-bindings/cleanup", server.cleanupMissingBindings)
	authorized.POST("/onboarding", server.createOnboarding)
	authorized.POST("/onboarding/batch", server.createOnboardingBatch)
	authorized.POST("/onboarding/prepare", server.prepareOnboarding)
	authorized.POST("/onboarding/keys/cleanup-preview", server.onboardingKeyCleanupPreview)
	authorized.POST("/onboarding/keys/cleanup", server.onboardingKeyCleanup)
	authorized.POST("/onboarding/probe/models", server.onboardingProbeModels)
	authorized.POST("/onboarding/probe", server.onboardingProbe)
	authorized.POST("/onboarding/probe/cancel", server.cancelOnboardingProbe)
	authorized.GET("/groups", server.groups)
	authorized.GET("/groups/:group_id/allocation", server.groupAllocation)
	authorized.GET("/groups/:group_id/models", server.groupProbeModels)
	authorized.PUT("/groups/:group_id/policy", server.updateGroupPolicy)
	authorized.DELETE("/groups/:group_id/policy", server.clearGroupPolicy)
	authorized.PUT("/groups/:group_id/excluded", server.setGroupExcluded)
	authorized.GET("/policy", server.policy)
	authorized.PUT("/policy", server.updatePolicy)
	authorized.POST("/policy/restore-control", server.restoreRoutingControl)
	authorized.GET("/pricing", server.pricingSnapshot)
	authorized.GET("/pricing/changes", server.pricingChanges)
	authorized.PUT("/pricing/config", server.updatePricingConfig)
	authorized.POST("/pricing/apply", server.applyPricing)
	authorized.POST("/pricing/revenue", server.calculatePricingRevenue)
	authorized.GET("/pricing/revenue/latest", server.latestPricingRevenue)
	authorized.GET("/pricing/backups", server.pricingBackups)
	authorized.POST("/pricing/backups", server.createPricingBackup)
	authorized.DELETE("/pricing/backups/:backup_id", server.deletePricingBackup)
	authorized.POST("/pricing/backups/:backup_id/restore", server.restorePricingBackup)
	authorized.GET("/newapi", server.newAPIWorkspace)
	authorized.POST("/newapi/platforms", server.saveNewAPIPlatform)
	authorized.DELETE("/newapi/platforms/:platform_id", server.deleteNewAPIPlatform)
	authorized.POST("/newapi/platforms/:platform_id/refresh", server.refreshNewAPIPlatform)
	authorized.GET("/newapi/platforms/:platform_id/management-model-prices", server.managementModelPrices)
	authorized.GET("/newapi/platforms/:platform_id/remote-model-prices/raw", server.remoteModelPricingSource)
	authorized.PUT("/newapi/platforms/:platform_id/group-bindings", server.saveNewAPIGroupBindings)
	authorized.POST("/newapi/platforms/:platform_id/channel-key", server.createNewAPIChannelKey)
	authorized.POST("/newapi/platforms/:platform_id/channel-models", server.fetchNewAPIChannelModels)
	authorized.POST("/newapi/platforms/:platform_id/channels", server.createNewAPIChannel)
	authorized.PUT("/newapi/platforms/:platform_id/model-prices", server.saveNewAPIModelPrices)
	authorized.GET("/upstreams", server.upstreams)
	authorized.POST("/upstreams", server.createUpstream)
	authorized.POST("/upstreams/detect", server.detectUpstream)
	authorized.POST("/upstreams/balances/sync", server.syncAllUpstreamBalances)
	authorized.POST("/upstreams/names/repair", server.repairUpstreamNames)
	authorized.POST("/upstreams/groups/sync", server.syncAllUpstreamGroups)
	authorized.POST("/upstreams/sync", server.syncAllUpstreams)
	authorized.POST("/upstreams/:host/rate-sync", server.syncUpstreamRates)
	authorized.POST("/upstreams/:host/balance-sync", server.syncUpstreamBalance)
	authorized.GET("/upstreams/:host/configuration", server.upstreamConfiguration)
	authorized.PUT("/upstreams/:host/configuration", server.updateUpstreamConfiguration)
	authorized.GET("/upstreams/:host/groups", server.upstreamGroups)
	authorized.GET("/upstreams/:host/group-history", server.upstreamGroupHistory)
	authorized.GET("/upstreams/:host/delete-preview", server.upstreamDeletePreview)
	authorized.POST("/upstreams/:host/delete", server.deleteUpstream)
	authorized.GET("/auth-recovery/config", server.authRecoveryConfiguration)
	authorized.POST("/auth-recovery/vault-entry", server.configureVaultEntry)
	authorized.DELETE("/auth-recovery/vault-entry", server.deleteVaultEntry)
	authorized.POST("/auth-recovery/manual", server.verifyManualAuth)
	authorized.POST("/auth-recovery/run", server.runAuthRecovery)
	authorized.POST("/auth-recovery/run-batch", server.runAuthRecoveryBatch)
	authorized.POST("/auth-recovery/captcha/submit", server.submitAuthCaptcha)
	authorized.POST("/auth-recovery/captcha/cancel", server.cancelAuthCaptcha)
	authorized.GET("/events", server.events)
	authorized.POST("/inspection/run", server.runInspection)
	authorized.POST("/inspection/probe", server.runActiveProbe)
	authorized.GET("/model-checks/capabilities", server.modelCheckCapabilities)
	authorized.GET("/model-checks/account-statuses", server.modelCheckAccountStatuses)
	authorized.POST("/model-checks", server.runModelCheck)
	authorized.GET("/inspection/automation", server.autoInspectionStatus)
	authorized.PUT("/inspection/automation", server.updateAutoInspection)
	authorized.GET("/inspection/automation/events", server.autoInspectionEvents)
	authorized.POST("/inspection/automation/cancel", server.cancelAutoInspection)
	authorized.POST("/inspection/automation/resume", server.resumeAutoInspection)
	authorized.DELETE("/inspection/automation/history", server.clearAutoInspectionHistory)
	authorized.GET("/usage/trace/*request_id", server.requestTrace)
	authorized.GET("/ops/system-logs", server.systemLogs)
	authorized.GET("/alerts", server.alerts)
	authorized.DELETE("/alerts", server.clearAlerts)
	authorized.GET("/alerts/policy", server.alertPolicy)
	authorized.PUT("/alerts/policy", server.updateAlertPolicy)
	authorized.POST("/alerts/evaluate", server.evaluateAlerts)
	authorized.GET("/tasks/:task_id", server.taskDetail)
	authorized.GET("/tasks/:task_id/events", server.taskEvents)
	authorized.GET("/logs", server.logsPage)
	authorized.DELETE("/logs", server.clearLogs)
	authorized.GET("/config/log-cleanup", server.logCleanupStatus)
	authorized.PUT("/config/log-cleanup", server.updateLogCleanup)
	return router
}

func limitRequestBody() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Body == nil || c.Request.Method == http.MethodGet || c.Request.Method == http.MethodHead || c.Request.Method == http.MethodOptions {
			c.Next()
			return
		}
		if c.Request.ContentLength > maximumRequestBytes {
			writeError(c, http.StatusRequestEntityTooLarge, "请求体不能超过 2 MiB")
			c.Abort()
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maximumRequestBytes)
		c.Next()
	}
}

func (s *Server) setupStatus(c *gin.Context) {
	status, err := s.private.PublicStatus(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusInternalServerError, "控制台配置读取失败")
		return
	}
	c.JSON(http.StatusOK, setupStatusResponse{
		Initialized:         status.Initialized,
		TargetConfigured:    status.TargetConfigured,
		SetupTokenRequired:  s.setupTokenRequired(c.Request),
		ConfigurationErrors: status.ConfigurationErrors,
	})
}

func (s *Server) initialize(c *gin.Context) {
	if s.setupTokenRequired(c.Request) && !s.validSetupCredential(c.Request) {
		writeError(c, http.StatusForbidden, "远程初始化需要有效的初始化令牌")
		return
	}
	var payload initializeRequest
	if err := bindRequestJSON(c, &payload); err != nil {
		writeError(c, http.StatusUnprocessableEntity, "初始化参数无效")
		return
	}
	status, err := s.private.PublicStatus(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusInternalServerError, "控制台配置读取失败")
		return
	}
	if len(status.ConfigurationErrors) > 0 {
		writeError(c, http.StatusConflict, "初始化前置配置无效："+strings.Join(status.ConfigurationErrors, "、"))
		return
	}
	if status.Initialized {
		writeError(c, http.StatusConflict, "控制台已经初始化，不能重复覆盖配置")
		return
	}
	if err := s.business.Bootstrap(c.Request.Context()); err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	if err := s.private.Initialize(c.Request.Context(), payload.Username, payload.Password, payload.AdminBaseURL, payload.AdminKey); err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	if err := s.setSession(c, strings.TrimSpace(payload.Username)); err != nil {
		writeError(c, http.StatusInternalServerError, "控制台会话创建失败")
		return
	}
	status, err = s.private.PublicStatus(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusInternalServerError, "控制台配置读取失败")
		return
	}
	c.JSON(http.StatusOK, setupStatusResponse{
		Initialized:         status.Initialized,
		TargetConfigured:    status.TargetConfigured,
		SetupTokenRequired:  s.setupTokenRequired(c.Request),
		ConfigurationErrors: status.ConfigurationErrors,
	})
}

func (s *Server) authSession(c *gin.Context) {
	initialized, err := s.private.IsInitialized(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusInternalServerError, "控制台配置读取失败")
		return
	}
	if !initialized {
		c.JSON(http.StatusOK, sessionStatus{Authenticated: false, Username: nil})
		return
	}
	username, err := s.sessionUser(c)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "控制台会话读取失败")
		return
	}
	c.JSON(http.StatusOK, sessionStatus{Authenticated: username != nil, Username: username})
}

func (s *Server) login(c *gin.Context) {
	var payload loginRequest
	if err := bindRequestJSON(c, &payload); err != nil {
		writeError(c, http.StatusUnprocessableEntity, "登录参数无效")
		return
	}
	initialized, err := s.private.IsInitialized(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusInternalServerError, "控制台配置读取失败")
		return
	}
	if !initialized {
		writeError(c, http.StatusPreconditionRequired, "请先完成首次初始化")
		return
	}
	now := s.now()
	if retryAfter := s.loginThrottle.retryAfter(c.Request, now); retryAfter > 0 {
		seconds := int(retryAfter.Round(time.Second).Seconds())
		if seconds < 1 {
			seconds = 1
		}
		c.Header("Retry-After", strconv.Itoa(seconds))
		writeError(c, http.StatusTooManyRequests, "登录尝试过于频繁，请稍后再试")
		return
	}
	authenticated, err := s.private.Authenticate(c.Request.Context(), payload.Username, payload.Password)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "控制台认证读取失败")
		return
	}
	if !authenticated {
		s.loginThrottle.recordFailure(c.Request, now)
		writeError(c, http.StatusUnauthorized, "账号或密码错误")
		return
	}
	s.loginThrottle.recordSuccess(c.Request)
	username := strings.TrimSpace(payload.Username)
	if err := s.setSession(c, username); err != nil {
		writeError(c, http.StatusInternalServerError, "控制台会话创建失败")
		return
	}
	c.JSON(http.StatusOK, sessionStatus{Authenticated: true, Username: &username})
}

func (s *Server) logout(c *gin.Context) {
	token, _ := c.Cookie(sessionCookie)
	if err := s.private.RevokeSession(c.Request.Context(), token); err != nil {
		writeError(c, http.StatusInternalServerError, "控制台会话注销失败")
		return
	}
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     sessionCookie,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   s.config.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
	c.JSON(http.StatusOK, sessionStatus{Authenticated: false, Username: nil})
}

func (s *Server) updateProfile(c *gin.Context) {
	var payload profileRequest
	if err := bindRequestJSON(c, &payload); err != nil {
		writeError(c, http.StatusUnprocessableEntity, "个人资料参数无效")
		return
	}
	username, err := s.private.UpdateCredentials(
		c.Request.Context(),
		payload.CurrentPassword,
		payload.Username,
		payload.NewPassword,
	)
	if err != nil {
		writeError(c, http.StatusUnprocessableEntity, err.Error())
		return
	}
	if err := s.setSession(c, username); err != nil {
		writeError(c, http.StatusInternalServerError, "控制台会话创建失败")
		return
	}
	c.JSON(http.StatusOK, sessionStatus{Authenticated: true, Username: &username})
}

func (s *Server) health(c *gin.Context) {
	mode, err := s.business.Mode(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusConflict, "运行配置无效")
		return
	}
	if !runtimepolicy.Valid(mode) {
		writeError(c, http.StatusConflict, "运行模式无效："+mode)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok", "mode": mode})
}

func (s *Server) trafficRanking(c *gin.Context) {
	timeRange := queryDefault(c, "time_range", "24h")
	durations := map[string]time.Duration{
		"1h": time.Hour, "6h": 6 * time.Hour, "24h": 24 * time.Hour,
		"7d": 7 * 24 * time.Hour, "30d": 30 * 24 * time.Hour,
	}
	duration, found := durations[timeRange]
	if !found {
		writeError(c, http.StatusUnprocessableEntity, "时间范围必须是 1h、6h、24h、7d 或 30d")
		return
	}
	sortBy := queryDefault(c, "sort_by", business.TrafficRankingSortTraffic)
	allowedSorts := map[string]bool{
		business.TrafficRankingSortTraffic: true, business.TrafficRankingSortStability: true,
		business.TrafficRankingSortSuccessRate: true, business.TrafficRankingSortLatency: true,
	}
	if !allowedSorts[sortBy] {
		writeError(c, http.StatusUnprocessableEntity, "排序方式必须是 traffic、stability、success_rate 或 latency")
		return
	}
	groupName := strings.TrimSpace(c.Query("group"))
	if len(groupName) > 255 {
		writeError(c, http.StatusUnprocessableEntity, "分组名称不能超过 255 个字符")
		return
	}
	endAt := s.now().UTC()
	result, err := s.business.TrafficRanking(c.Request.Context(), business.TrafficRankingQuery{
		StartAt: endAt.Add(-duration), EndAt: endAt, GroupName: groupName, SortBy: sortBy,
	})
	if err != nil {
		slog.Error("流量排行读取失败", "error", err)
		writeError(c, http.StatusInternalServerError, "流量排行读取失败")
		return
	}
	c.JSON(http.StatusOK, result)
}

func (s *Server) overview(c *gin.Context) {
	summary, err := s.business.OverviewSummary(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusInternalServerError, "控制台概览读取失败")
		return
	}
	snapshot, err := s.business.RuntimeSnapshot(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusInternalServerError, "运行配置读取失败")
		return
	}
	c.JSON(http.StatusOK, overviewResponse{
		DatabasePath:      s.config.DataDB,
		DatabaseAvailable: summary.Available,
		AccountCount:      summary.Accounts,
		GroupCount:        summary.Groups,
		OpenAlerts:        summary.Alerts,
		RecentRuns:        summary.Runs,
		LastActivity:      summary.LastActivity,
		Mode:              snapshot.Mode,
	})
}

func (s *Server) runtimeConfig(c *gin.Context) {
	snapshot, err := s.business.RuntimeSnapshot(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusInternalServerError, "运行配置读取失败")
		return
	}
	s.runtimeConfigFromSnapshot(c, snapshot)
}

func (s *Server) updateRuntimeMode(c *gin.Context) {
	var payload runtimeModeRequest
	if err := bindRequestJSON(c, &payload); err != nil {
		writeError(c, http.StatusUnprocessableEntity, "运行模式参数无效")
		return
	}
	ready, err := s.business.Ready(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusInternalServerError, "业务库状态读取失败")
		return
	}
	if !ready {
		writeError(c, http.StatusConflict, "Console 业务库尚未就绪")
		return
	}
	snapshot, err := s.business.SetMode(c.Request.Context(), payload.Mode)
	if err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	s.runtimeConfigFromSnapshot(c, snapshot)
}

func (s *Server) updateAdminTarget(c *gin.Context) {
	var payload adminTargetRequest
	if err := bindRequestJSON(c, &payload); err != nil {
		writeError(c, http.StatusUnprocessableEntity, "管理目标参数无效")
		return
	}
	initialized, err := s.private.IsInitialized(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusInternalServerError, "控制台配置读取失败")
		return
	}
	if !initialized {
		writeError(c, http.StatusConflict, "请先完成控制台初始化")
		return
	}
	guarded, release, err := mutationguard.Acquire(c.Request.Context(), s.business, mutationguard.ManagementTarget())
	if err != nil {
		writeError(c, http.StatusConflict, "管理目标正在被远端操作使用，请稍后重试")
		return
	}
	defer func() {
		if err := release(); err != nil {
			slog.Error("管理目标配置租约释放失败", "error", err)
		}
	}()
	if err := s.private.ConfigureTarget(
		guarded,
		payload.AdminBaseURL,
		payload.AdminKey,
		payload.RequestTimeoutSeconds,
	); err != nil {
		writeError(c, http.StatusUnprocessableEntity, err.Error())
		return
	}
	snapshot, err := s.business.RuntimeSnapshot(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusInternalServerError, "运行配置读取失败")
		return
	}
	s.runtimeConfigFromSnapshot(c, snapshot)
}

func (s *Server) updateProbeSettings(c *gin.Context) {
	var payload probeSettingsRequest
	if err := bindRequestJSON(c, &payload); err != nil || payload.Enabled == nil {
		writeError(c, http.StatusUnprocessableEntity, "主动探测设置参数无效")
		return
	}
	ready, err := s.business.Ready(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusInternalServerError, "业务库状态读取失败")
		return
	}
	if !ready {
		writeError(c, http.StatusConflict, "Console 业务库尚未就绪")
		return
	}
	if err := s.business.SetProbeEnabled(c.Request.Context(), *payload.Enabled); err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	snapshot, err := s.business.RuntimeSnapshot(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusInternalServerError, "运行配置读取失败")
		return
	}
	s.runtimeConfigFromSnapshot(c, snapshot)
}

func (s *Server) updateAccountDefaults(c *gin.Context) {
	var payload accountDefaultsRequest
	if err := bindRequestJSON(c, &payload); err != nil {
		writeError(c, http.StatusUnprocessableEntity, "账号默认参数无效")
		return
	}
	if _, err := s.private.ConfigureAccountDefaults(c.Request.Context(), payload.Concurrency, payload.Priority); err != nil {
		writeError(c, http.StatusUnprocessableEntity, err.Error())
		return
	}
	snapshot, err := s.business.RuntimeSnapshot(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusInternalServerError, "运行配置读取失败")
		return
	}
	s.runtimeConfigFromSnapshot(c, snapshot)
}

func (s *Server) notificationStatus(c *gin.Context) {
	status, err := s.readNotificationStatus(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusInternalServerError, "通知配置读取失败")
		return
	}
	c.JSON(http.StatusOK, status)
}

func (s *Server) notificationQueue(c *gin.Context) {
	reader, available := s.business.(notificationQueueDetailReader)
	if !available {
		writeError(c, http.StatusNotImplemented, "告警队列明细不可用")
		return
	}
	status, err := s.private.NotificationPublicStatus(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusInternalServerError, "通知配置读取失败")
		return
	}
	channelKey := business.NotificationChannelKey("qqbot", status.HomeChannel)
	details, err := reader.NotificationQueueDetails(c.Request.Context(), channelKey, status.Configured)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "告警队列明细读取失败")
		return
	}
	c.JSON(http.StatusOK, details)
}

func (s *Server) configureNotification(c *gin.Context) {
	var payload notificationConfigRequest
	if err := bindRequestJSON(c, &payload); err != nil {
		writeError(c, http.StatusUnprocessableEntity, "通知配置参数无效")
		return
	}
	effectiveSecret := payload.ClientSecret
	if strings.TrimSpace(effectiveSecret) == "" {
		current, err := s.private.NotificationSettings(c.Request.Context())
		if err != nil {
			writeError(c, http.StatusInternalServerError, "通知配置读取失败")
			return
		}
		effectiveSecret = current.ClientSecret
	}
	if _, err := configstore.ValidateNotificationSettings(
		payload.AppID,
		effectiveSecret,
		payload.HomeChannel,
		payload.HomeChannelType,
	); err != nil {
		writeError(c, http.StatusUnprocessableEntity, err.Error())
		return
	}
	if err := s.business.EnableNotificationChannel(c.Request.Context(), "qqbot"); err != nil {
		writeError(c, http.StatusUnprocessableEntity, err.Error())
		return
	}
	if err := s.private.ConfigureNotifications(
		c.Request.Context(),
		payload.AppID,
		payload.ClientSecret,
		payload.HomeChannel,
		payload.HomeChannelType,
	); err != nil {
		writeError(c, http.StatusInternalServerError, "通知私有配置保存失败")
		return
	}
	status, err := s.readNotificationStatus(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusInternalServerError, "通知配置读取失败")
		return
	}
	c.JSON(http.StatusOK, status)
}

func (s *Server) discoverNotificationTarget(c *gin.Context) {
	if s.notificationTarget == nil {
		writeError(c, http.StatusServiceUnavailable, "QQBot 目标获取服务尚未就绪")
		return
	}
	var payload notificationTargetDiscoveryRequest
	if err := bindRequestJSON(c, &payload); err != nil {
		writeError(c, http.StatusUnprocessableEntity, "QQBot 目标获取参数无效")
		return
	}
	current, err := s.private.NotificationSettings(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusInternalServerError, "通知配置读取失败")
		return
	}
	appID := strings.TrimSpace(payload.AppID)
	if appID == "" {
		appID = current.AppID
	}
	clientSecret := strings.TrimSpace(payload.ClientSecret)
	if clientSecret == "" {
		clientSecret = current.ClientSecret
	}
	task, err := s.notificationTarget.Enqueue(c.Request.Context(), notificationtarget.Request{
		AppID: appID, ClientSecret: clientSecret, TargetType: payload.TargetType,
	})
	if err != nil {
		status := http.StatusUnprocessableEntity
		if errors.Is(err, notificationtarget.ErrDiscoveryActive) {
			status = http.StatusConflict
		}
		writeError(c, status, err.Error())
		return
	}
	c.JSON(http.StatusAccepted, task)
}

func (s *Server) cancelNotificationTargetDiscovery(c *gin.Context) {
	if s.notificationTarget == nil {
		writeError(c, http.StatusServiceUnavailable, "QQBot 目标获取服务尚未就绪")
		return
	}
	if !s.notificationTarget.Cancel(c.Param("task_id")) {
		writeError(c, http.StatusNotFound, "目标获取任务不存在或已经结束")
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"cancelled": true})
}

func (s *Server) readNotificationStatus(ctx context.Context) (notificationStatusResponse, error) {
	status, err := s.private.NotificationPublicStatus(ctx)
	if err != nil {
		return notificationStatusResponse{}, err
	}
	result := notificationStatusResponse{NotificationStatus: status}
	if reader, available := s.business.(notificationQueueReader); available {
		channelKey := business.NotificationChannelKey("qqbot", status.HomeChannel)
		snapshot, snapshotErr := reader.NotificationQueueSnapshot(ctx, channelKey, status.Configured)
		if snapshotErr != nil {
			return notificationStatusResponse{}, snapshotErr
		}
		result.Queues.ProducerFiring = snapshot.ProducerFiring
		result.Queues.ProducerRecovered = snapshot.ProducerRecovered
		result.Queues.ConsumerPending = snapshot.ConsumerPending
		result.Queues.ConsumerFailed = snapshot.ConsumerFailed
	}
	if reader, available := s.notifier.(notificationQueueStatusReader); available {
		result.Queues.ConsumerActive = reader.Status().ConsumerActive
	}
	return result, nil
}

func (s *Server) testNotification(c *gin.Context) {
	if s.notifier == nil {
		writeError(c, http.StatusServiceUnavailable, "通知服务尚未就绪")
		return
	}
	var payload notificationTestRequest
	if err := bindRequestJSON(c, &payload); err != nil || payload.DryRun == nil {
		writeError(c, http.StatusUnprocessableEntity, "通知测试参数无效")
		return
	}
	result, err := s.notifier.Test(c.Request.Context(), payload.Message, *payload.DryRun)
	if err != nil {
		status := http.StatusBadGateway
		if strings.Contains(err.Error(), "尚未配置") || strings.Contains(err.Error(), "长度") {
			status = http.StatusConflict
		}
		writeError(c, status, err.Error())
		return
	}
	c.JSON(http.StatusOK, result)
}

func (s *Server) accounts(c *gin.Context) {
	rows, err := s.business.Accounts(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusInternalServerError, "账号列表读取失败")
		return
	}
	s.enrichRecentResults(c.Request.Context(), rows)
	c.JSON(http.StatusOK, rows)
}

func (s *Server) account(c *gin.Context) {
	accountID := strings.TrimSpace(c.Param("account_id"))
	if !positiveNumericID(accountID) {
		writeError(c, http.StatusUnprocessableEntity, "账号必须使用有效的稳定 ID")
		return
	}
	row, err := s.business.Account(c.Request.Context(), accountID)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(c, http.StatusNotFound, "账号不存在")
		return
	}
	if err != nil {
		writeError(c, http.StatusInternalServerError, "账号详情读取失败")
		return
	}
	if len(row.RecentResults) > 0 {
		enriched := []business.AccountStatus{row.AccountStatus}
		s.enrichRecentResults(c.Request.Context(), enriched)
		row.RecentResults = enriched[0].RecentResults
	}
	c.JSON(http.StatusOK, row)
}

func (s *Server) accountDeletePreview(c *gin.Context) {
	if s.accountDelete == nil {
		writeError(c, http.StatusServiceUnavailable, "账号删除服务尚未就绪")
		return
	}
	preview, err := s.accountDelete.Preview(c.Request.Context(), c.Param("account_id"))
	if err != nil {
		status := http.StatusConflict
		if errors.Is(err, sql.ErrNoRows) {
			status = http.StatusNotFound
		}
		writeError(c, status, err.Error())
		return
	}
	c.JSON(http.StatusOK, preview)
}

func (s *Server) accountDeleteBatchPreview(c *gin.Context) {
	if s.accountDelete == nil {
		writeError(c, http.StatusServiceUnavailable, "账号删除服务尚未就绪")
		return
	}
	payload, err := decodeRequestObject(c)
	if err != nil || len(payload) != 1 {
		writeError(c, http.StatusUnprocessableEntity, "批量删除预览参数必须只包含 account_ids")
		return
	}
	accountIDs, err := stableAccountIDs(payload["account_ids"], 50)
	if err != nil {
		writeError(c, http.StatusUnprocessableEntity, err.Error())
		return
	}
	preview, err := s.accountDelete.PreviewBatch(c.Request.Context(), accountIDs)
	if err != nil {
		status := http.StatusConflict
		if errors.Is(err, sql.ErrNoRows) {
			status = http.StatusNotFound
		}
		writeError(c, status, err.Error())
		return
	}
	c.JSON(http.StatusOK, preview)
}

func (s *Server) deleteAccounts(c *gin.Context) {
	if s.accountDelete == nil {
		writeError(c, http.StatusServiceUnavailable, "账号删除服务尚未就绪")
		return
	}
	payload, err := decodeRequestObject(c)
	if err != nil || len(payload) != 1 {
		writeError(c, http.StatusUnprocessableEntity, "账号批量删除参数必须只包含 confirmations")
		return
	}
	confirmations, err := accountDeleteConfirmations(payload["confirmations"])
	if err != nil {
		writeError(c, http.StatusUnprocessableEntity, err.Error())
		return
	}
	actor, err := s.requestActor(c)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "控制台会话读取失败")
		return
	}
	task, err := s.accountDelete.EnqueueBatch(c.Request.Context(), confirmations, actor)
	if err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(http.StatusOK, task)
}

func stableAccountIDs(raw any, maximum int) ([]string, error) {
	rawIDs, ok := raw.([]any)
	if !ok || len(rawIDs) == 0 {
		return nil, errors.New("请至少提供一个账号 ID")
	}
	if len(rawIDs) > maximum {
		return nil, fmt.Errorf("单次最多处理 %d 个账号", maximum)
	}
	result := make([]string, 0, len(rawIDs))
	seen := make(map[string]struct{}, len(rawIDs))
	for _, rawID := range rawIDs {
		accountID, ok := rawID.(string)
		accountID = strings.TrimSpace(accountID)
		if !ok || !positiveNumericID(accountID) {
			return nil, errors.New("账号必须全部使用稳定数字 ID")
		}
		if _, found := seen[accountID]; found {
			return nil, fmt.Errorf("账号 ID 不能重复：%s", accountID)
		}
		seen[accountID] = struct{}{}
		result = append(result, accountID)
	}
	return result, nil
}

func accountDeleteConfirmations(raw any) ([]accountdelete.Confirmation, error) {
	values, ok := raw.([]any)
	if !ok || len(values) == 0 {
		return nil, errors.New("请至少提供一个账号删除确认范围")
	}
	if len(values) > 50 {
		return nil, errors.New("单次最多删除 50 个账号")
	}
	result := make([]accountdelete.Confirmation, 0, len(values))
	for _, rawValue := range values {
		value, ok := rawValue.(map[string]any)
		if !ok || len(value) != 3 {
			return nil, errors.New("每个账号删除确认必须包含 account_id、management_base_url 和 binding")
		}
		accountID, err := requiredText(value, "account_id", 1, 64)
		if err != nil || !positiveNumericID(accountID) {
			return nil, errors.New("批量删除确认中的账号 ID 无效")
		}
		managementBaseURL, err := requiredText(value, "management_base_url", 1, 2048)
		if err != nil {
			return nil, errors.New("批量删除确认中的管理目标地址无效")
		}
		confirmation := accountdelete.Confirmation{
			AccountID: accountID, ManagementBaseURL: managementBaseURL,
		}
		if value["binding"] != nil {
			binding, bindingOK := value["binding"].(map[string]any)
			if !bindingOK || len(binding) != 6 {
				return nil, errors.New("批量删除确认中的账号绑定范围不完整")
			}
			bindingID, bindingErr := positiveJSONInteger(binding["id"], "binding.id", 1, 1<<62)
			upstreamID, upstreamErr := requiredText(binding, "upstream_id", 1, 255)
			upstreamHost, hostErr := requiredText(binding, "upstream_host", 1, 2048)
			authHost, authErr := requiredText(binding, "auth_host", 1, 2048)
			keyID, keyErr := requiredText(binding, "upstream_key_id", 1, 255)
			keyName, nameErr := requiredText(binding, "upstream_key_name", 0, 255)
			if bindingErr != nil || upstreamErr != nil || hostErr != nil || authErr != nil || keyErr != nil || nameErr != nil {
				return nil, errors.New("批量删除确认中的账号绑定范围无效")
			}
			confirmation.Binding = &accountdelete.Binding{
				ID: bindingID, UpstreamID: upstreamID, UpstreamHost: upstreamHost,
				AuthHost: authHost, UpstreamKeyID: keyID, UpstreamKeyName: keyName,
			}
		}
		result = append(result, confirmation)
	}
	return result, nil
}

func (s *Server) deleteAccount(c *gin.Context) {
	if s.accountDelete == nil {
		writeError(c, http.StatusServiceUnavailable, "账号删除服务尚未就绪")
		return
	}
	payload, err := decodeRequestObject(c)
	if err != nil || (len(payload) != 2 && len(payload) != 7) {
		writeError(c, http.StatusUnprocessableEntity, "账号删除参数必须包含确认账号 ID、管理目标以及预览返回的可选绑定范围")
		return
	}
	confirmation, err := requiredText(payload, "confirmation_account_id", 1, 64)
	if err != nil || confirmation != strings.TrimSpace(c.Param("account_id")) || !positiveNumericID(confirmation) {
		writeError(c, http.StatusUnprocessableEntity, "确认账号 ID 与删除目标不一致")
		return
	}
	managementBaseURL, err := requiredText(payload, "expected_management_base_url", 1, 2048)
	if err != nil {
		writeError(c, http.StatusUnprocessableEntity, "预期管理目标地址无效")
		return
	}
	var expectedBinding *accountdelete.Binding
	if len(payload) == 7 {
		bindingID, bindingErr := positiveJSONInteger(payload["expected_binding_id"], "expected_binding_id", 1, 1<<62)
		if bindingErr != nil {
			writeError(c, http.StatusUnprocessableEntity, "预期 Binding ID 必须是稳定正整数")
			return
		}
		upstreamID, upstreamErr := requiredText(payload, "expected_upstream_id", 1, 255)
		if upstreamErr != nil {
			writeError(c, http.StatusUnprocessableEntity, "预期稳定上游身份 ID 无效")
			return
		}
		keyID, keyErr := requiredText(payload, "expected_upstream_key_id", 1, 255)
		if keyErr != nil {
			writeError(c, http.StatusUnprocessableEntity, "预期上游 Key ID 无效")
			return
		}
		upstreamHost, upstreamHostErr := requiredText(payload, "expected_upstream_host", 1, 2048)
		if upstreamHostErr != nil {
			writeError(c, http.StatusUnprocessableEntity, "预期上游地址无效")
			return
		}
		authHost, authHostErr := requiredText(payload, "expected_auth_host", 1, 2048)
		if authHostErr != nil {
			writeError(c, http.StatusUnprocessableEntity, "预期鉴权 Host 无效")
			return
		}
		expectedBinding = &accountdelete.Binding{
			ID: bindingID, UpstreamID: upstreamID, UpstreamHost: upstreamHost, AuthHost: authHost, UpstreamKeyID: keyID,
		}
	}
	actor, err := s.requestActor(c)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "控制台会话读取失败")
		return
	}
	task, err := s.accountDelete.Enqueue(c.Request.Context(), confirmation, expectedBinding, managementBaseURL, actor)
	if err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(http.StatusOK, task)
}

func (s *Server) enrichRecentResults(ctx context.Context, accounts []business.AccountStatus) {
	policy := map[string]any{}
	if current, err := s.business.ControlPolicy(ctx); err == nil && current != nil {
		policy = current
	}
	for accountIndex := range accounts {
		for resultIndex := range accounts[accountIndex].RecentResults {
			result := &accounts[accountIndex].RecentResults[resultIndex]
			classification, err := routing.ClassifySample(routing.Sample{
				Result: pointerText(result.Result), FailureReason: pointerText(result.FailureReason),
				Source: result.Source, LatencyP95: result.ClassificationLatency, Payload: result.ClassificationPayload,
			}, policy)
			if err != nil {
				continue
			}
			eventType := string(classification.Event)
			score := classification.Score
			result.EventType, result.Score = &eventType, &score
		}
	}
}

func pointerText(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func (s *Server) setAccountControl(c *gin.Context) {
	accountID := strings.TrimSpace(c.Param("account_id"))
	if !positiveNumericID(accountID) {
		writeError(c, http.StatusUnprocessableEntity, "账号必须使用有效的稳定 ID")
		return
	}
	payload, err := decodeRequestObject(c)
	if err != nil || len(payload) != 1 {
		writeError(c, http.StatusUnprocessableEntity, "账号控制参数必须只包含 action")
		return
	}
	action, ok := payload["action"].(string)
	if !ok {
		writeError(c, http.StatusUnprocessableEntity, "action 必须是字符串")
		return
	}
	action = strings.TrimSpace(action)
	allowed := map[string]struct{}{
		"pause": {}, "resume": {}, "exclude": {}, "include": {}, "fuse": {}, "recover": {},
	}
	if _, valid := allowed[action]; !valid {
		writeError(c, http.StatusUnprocessableEntity, "账号控制 action 无效")
		return
	}
	if s.accountTasks == nil {
		writeError(c, http.StatusServiceUnavailable, "账号操作服务尚未就绪")
		return
	}
	requiresTarget := action != "exclude" && action != "include"
	if _, ok := s.accountMutationPreflight(c, accountID, requiresTarget); !ok {
		return
	}
	actor, err := s.requestActor(c)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "控制台会话读取失败")
		return
	}
	task, err := s.accountTasks.EnqueueControl(c.Request.Context(), accountID, action, actor)
	if err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(http.StatusOK, task)
}

func (s *Server) accountModels(c *gin.Context) {
	accountID := strings.TrimSpace(c.Param("account_id"))
	if !positiveNumericID(accountID) {
		writeError(c, http.StatusUnprocessableEntity, "账号必须使用有效的稳定 ID")
		return
	}
	if s.accountTasks == nil {
		writeError(c, http.StatusServiceUnavailable, "账号操作服务尚未就绪")
		return
	}
	if _, ok := s.accountMutationPreflight(c, accountID, true); !ok {
		return
	}
	models, err := s.accountTasks.Models(c.Request.Context(), accountID)
	if err != nil {
		writeError(c, http.StatusBadGateway, "账号模型读取失败："+err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"models": models})
}

func (s *Server) setAccountTestModel(c *gin.Context) {
	accountID := strings.TrimSpace(c.Param("account_id"))
	if !positiveNumericID(accountID) {
		writeError(c, http.StatusUnprocessableEntity, "账号必须使用有效的稳定 ID")
		return
	}
	payload, err := decodeRequestObject(c)
	if err != nil || len(payload) != 1 {
		writeError(c, http.StatusUnprocessableEntity, "账号探测模型参数必须只包含 model")
		return
	}
	raw, present := payload["model"]
	if !present {
		writeError(c, http.StatusUnprocessableEntity, "账号探测模型参数缺少 model")
		return
	}
	var model *string
	if raw != nil {
		value, ok := raw.(string)
		if !ok || utf8.RuneCountInString(value) > 256 {
			writeError(c, http.StatusUnprocessableEntity, "探测模型必须是长度不超过 256 的字符串或 null")
			return
		}
		model = &value
	}
	if _, ok := s.accountMutationPreflight(c, accountID, false); !ok {
		return
	}
	actor, err := s.requestActor(c)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "控制台会话读取失败")
		return
	}
	if err := s.business.SetAccountTestModel(c.Request.Context(), accountID, model, actor); err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"saved": true})
}

func (s *Server) syncAccountFields(c *gin.Context) {
	accountID := strings.TrimSpace(c.Param("account_id"))
	if !positiveNumericID(accountID) {
		writeError(c, http.StatusUnprocessableEntity, "账号必须使用有效的稳定 ID")
		return
	}
	payload, err := decodeRequestObject(c)
	if err != nil || len(payload) == 0 {
		writeError(c, http.StatusUnprocessableEntity, "至少提供一个需要同步的账号字段")
		return
	}
	patch, err := accountFieldPatch(payload)
	if err != nil {
		writeError(c, http.StatusUnprocessableEntity, err.Error())
		return
	}
	if s.accountTasks == nil {
		writeError(c, http.StatusServiceUnavailable, "账号操作任务服务尚未就绪")
		return
	}
	mode, ok := s.accountMutationPreflight(c, accountID, true)
	if !ok {
		return
	}
	if mode != runtimepolicy.Full {
		writeError(c, http.StatusConflict, "账号字段同步需要完全模式；账号成本请使用同步倍率")
		return
	}
	actor, err := s.requestActor(c)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "控制台会话读取失败")
		return
	}
	task, err := s.accountTasks.EnqueueFields(c.Request.Context(), accountID, patch, actor)
	if err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(http.StatusOK, task)
}

func (s *Server) saveAccountSettings(c *gin.Context) {
	accountID := strings.TrimSpace(c.Param("account_id"))
	if !positiveNumericID(accountID) {
		writeError(c, http.StatusUnprocessableEntity, "账号必须使用有效的稳定 ID")
		return
	}
	var request accountSettingsRequest
	if err := bindRequestJSON(c, &request); err != nil || request.Paused == nil || request.Excluded == nil {
		writeError(c, http.StatusUnprocessableEntity, "账号设置参数无效")
		return
	}
	if request.TestModel != nil && utf8.RuneCountInString(*request.TestModel) > 256 {
		writeError(c, http.StatusUnprocessableEntity, "探测模型不能超过 256 个字符")
		return
	}
	if s.accountTasks == nil {
		writeError(c, http.StatusServiceUnavailable, "账号设置任务服务尚未就绪")
		return
	}
	mode, ok := s.accountMutationPreflight(c, accountID, true)
	if !ok {
		return
	}
	if mode != runtimepolicy.Full {
		writeError(c, http.StatusConflict, "账号设置需要完全模式")
		return
	}
	actor, err := s.requestActor(c)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "控制台会话读取失败")
		return
	}
	task, err := s.accountTasks.EnqueueSettings(c.Request.Context(), accountID, accountops.SettingsInput{
		Priority: request.Priority, LoadFactor: request.LoadFactor, Concurrency: request.Concurrency,
		TestModel: request.TestModel, Paused: *request.Paused, Excluded: *request.Excluded,
	}, actor)
	if err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(http.StatusOK, task)
}

func (s *Server) setAccountManualPriority(c *gin.Context) {
	accountID := strings.TrimSpace(c.Param("account_id"))
	if !positiveNumericID(accountID) {
		writeError(c, http.StatusUnprocessableEntity, "账号必须使用有效的稳定 ID")
		return
	}
	payload, err := decodeRequestObject(c)
	if err != nil || len(payload) != 4 {
		writeError(c, http.StatusUnprocessableEntity, "人工优先位参数必须包含 priority、load_factor、concurrency 和 sync_balance_multiplier")
		return
	}
	priority, err := positiveJSONInteger(payload["priority"], "priority", 1, 1000)
	if err != nil {
		writeError(c, http.StatusUnprocessableEntity, err.Error())
		return
	}
	loadFactor, ok := payload["load_factor"].(string)
	if !ok || strings.TrimSpace(loadFactor) == "" {
		writeError(c, http.StatusUnprocessableEntity, "load_factor 必须是有效数字字符串")
		return
	}
	concurrency, err := positiveJSONInteger(payload["concurrency"], "concurrency", 1, 10_000_000)
	if err != nil {
		writeError(c, http.StatusUnprocessableEntity, err.Error())
		return
	}
	syncBalanceMultiplier, ok := payload["sync_balance_multiplier"].(bool)
	if !ok {
		writeError(c, http.StatusUnprocessableEntity, "sync_balance_multiplier 必须是布尔值")
		return
	}
	if s.accountTasks == nil {
		writeError(c, http.StatusServiceUnavailable, "账号操作任务服务尚未就绪")
		return
	}
	mode, ok := s.accountMutationPreflight(c, accountID, true)
	if !ok {
		return
	}
	if mode != runtimepolicy.Full {
		writeError(c, http.StatusConflict, "设置人工优先位需要完全模式")
		return
	}
	actor, err := s.requestActor(c)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "控制台会话读取失败")
		return
	}
	task, err := s.accountTasks.EnqueueManualPriority(c.Request.Context(), accountID, priority, loadFactor, concurrency, syncBalanceMultiplier, actor)
	if err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(http.StatusOK, task)
}

func (s *Server) clearAccountManualPriority(c *gin.Context) {
	accountID := strings.TrimSpace(c.Param("account_id"))
	if !positiveNumericID(accountID) {
		writeError(c, http.StatusUnprocessableEntity, "账号必须使用有效的稳定 ID")
		return
	}
	mode, ok := s.accountMutationPreflight(c, accountID, true)
	if !ok {
		return
	}
	if mode != runtimepolicy.Full {
		writeError(c, http.StatusConflict, "取消人工优先位需要完全模式")
		return
	}
	actor, err := s.requestActor(c)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "控制台会话读取失败")
		return
	}
	if s.accountTasks == nil {
		writeError(c, http.StatusServiceUnavailable, "账号操作任务服务尚未就绪")
		return
	}
	task, err := s.accountTasks.EnqueueClearManualPriority(c.Request.Context(), accountID, actor)
	if err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(http.StatusOK, task)
}

func accountFieldPatch(payload map[string]any) (accountops.FieldPatch, error) {
	allowed := map[string]struct{}{
		"name": {}, "priority": {}, "load_factor": {}, "concurrency": {}, "upstream_host": {}, "base_url": {}, "notes": {},
	}
	for key := range payload {
		if _, present := allowed[key]; !present {
			return accountops.FieldPatch{}, fmt.Errorf("账号字段同步参数包含未知字段：%s", key)
		}
	}
	result := accountops.FieldPatch{}
	if raw, present := payload["name"]; present {
		value, err := nullableTextField(raw, "name", 1, 255)
		if err != nil || value == nil || strings.TrimSpace(*value) == "" {
			return accountops.FieldPatch{}, errors.New("账号名称必须是长度在 1 到 255 之间的非空字符串")
		}
		result.NamePresent, result.Name = true, value
	}
	if raw, present := payload["priority"]; present {
		value, err := positiveJSONInteger(raw, "priority", 1, 10_000_000)
		if err != nil {
			return accountops.FieldPatch{}, err
		}
		result.PriorityPresent, result.Priority = true, &value
	}
	if raw, present := payload["load_factor"]; present {
		value, err := nullableTextField(raw, "load_factor", 1, 128)
		if err != nil || value == nil {
			return accountops.FieldPatch{}, errors.New("load_factor 必须是长度在 1 到 128 之间的字符串")
		}
		result.LoadFactorPresent, result.LoadFactor = true, value
	}
	if raw, present := payload["concurrency"]; present {
		value, err := positiveJSONInteger(raw, "concurrency", 1, 10_000_000)
		if err != nil {
			return accountops.FieldPatch{}, err
		}
		result.ConcurrencyPresent, result.Concurrency = true, &value
	}
	if raw, present := payload["upstream_host"]; present {
		value, err := nullableTextField(raw, "upstream_host", 1, 2048)
		if err != nil || value == nil {
			return accountops.FieldPatch{}, errors.New("upstream_host 必须是有效的上游 Host")
		}
		normalized := configstore.CanonicalHost(*value)
		if normalized == "" || strings.ContainsAny(normalized, "/\\?#") {
			return accountops.FieldPatch{}, errors.New("upstream_host 必须是有效的上游 Host")
		}
		result.UpstreamHostPresent, result.UpstreamHost = true, &normalized
	}
	if raw, present := payload["base_url"]; present {
		value, err := nullableTextField(raw, "base_url", 1, 2048)
		if err != nil || value == nil {
			return accountops.FieldPatch{}, errors.New("base_url 必须是完整的 HTTP/HTTPS 地址")
		}
		normalized := strings.TrimRight(strings.TrimSpace(*value), "/")
		parsed, parseErr := url.Parse(normalized)
		if parseErr != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil {
			return accountops.FieldPatch{}, errors.New("base_url 必须是完整的 HTTP/HTTPS 地址")
		}
		result.BaseURLPresent, result.BaseURL = true, &normalized
	}
	if raw, present := payload["notes"]; present {
		value, err := nullableTextField(raw, "notes", 0, 65536)
		if err != nil {
			return accountops.FieldPatch{}, err
		}
		result.NotesPresent, result.Notes = true, value
	}
	return result, nil
}

func positiveJSONInteger(raw any, field string, minimum, maximum int64) (int64, error) {
	number, ok := raw.(json.Number)
	if !ok {
		return 0, fmt.Errorf("%s 必须是整数", field)
	}
	value, err := number.Int64()
	if err != nil || value < minimum || value > maximum {
		return 0, fmt.Errorf("%s 必须是 %d 到 %d 之间的整数", field, minimum, maximum)
	}
	return value, nil
}

func (s *Server) accountMutationPreflight(c *gin.Context, accountID string, requireTarget bool) (string, bool) {
	mode, err := s.business.Mode(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusInternalServerError, "运行模式读取失败")
		return "", false
	}
	if _, valid := runtimepolicy.For(mode); !valid {
		writeError(c, http.StatusConflict, "运行模式无效："+mode)
		return "", false
	}
	if _, err := s.business.Account(c.Request.Context(), accountID); errors.Is(err, sql.ErrNoRows) {
		writeError(c, http.StatusNotFound, "账号不存在")
		return "", false
	} else if err != nil {
		writeError(c, http.StatusInternalServerError, "账号详情读取失败")
		return "", false
	}
	if requireTarget && !s.requireAdminTarget(c) {
		return "", false
	}
	return mode, true
}

func (s *Server) requireAdminTarget(c *gin.Context) bool {
	if s.private == nil {
		writeError(c, http.StatusServiceUnavailable, "控制台配置服务尚未就绪")
		return false
	}
	if _, err := s.private.TargetSettings(c.Request.Context()); err != nil {
		writeError(c, http.StatusConflict, "Sub2API 管理目标不可用："+err.Error())
		return false
	}
	return true
}

func (s *Server) syncManagement(c *gin.Context) {
	if s.managementTasks == nil {
		writeError(c, http.StatusServiceUnavailable, "管理同步任务服务尚未就绪")
		return
	}
	actor, err := s.requestActor(c)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "控制台会话读取失败")
		return
	}
	task, err := s.managementTasks.EnqueueSync(c.Request.Context(), actor)
	if err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(http.StatusOK, task)
}

func (s *Server) revalidateAccounts(c *gin.Context) {
	s.enqueueAccountMaintenance(c, "revalidate")
}

func (s *Server) validateAccountBaseURLs(c *gin.Context) {
	s.enqueueAccountMaintenance(c, "base-url")
}

func (s *Server) checkAccountConfiguration(c *gin.Context) {
	s.enqueueAccountMaintenance(c, "configuration-check")
}

func (s *Server) repairAccountBaseURLs(c *gin.Context) {
	s.enqueueAccountMaintenance(c, "base-url-repair")
}

func (s *Server) repairAccountUpstreamHosts(c *gin.Context) {
	s.enqueueAccountMaintenance(c, "upstream-hosts")
}

func (s *Server) syncAccountRates(c *gin.Context) {
	s.enqueueAccountMaintenance(c, "rates")
}

func (s *Server) repairAccountNames(c *gin.Context) {
	s.enqueueAccountMaintenance(c, "names")
}

func (s *Server) repairAccountDefaults(c *gin.Context) {
	s.enqueueAccountMaintenance(c, "defaults")
}

func (s *Server) cleanupMissingBindings(c *gin.Context) {
	s.enqueueAccountMaintenance(c, "cleanup")
}

func (s *Server) enqueueAccountMaintenance(c *gin.Context, operation string) {
	if s.accountMaintenance == nil {
		writeError(c, http.StatusServiceUnavailable, "账号维护任务服务尚未就绪")
		return
	}
	payload, err := decodeRequestObject(c)
	if err != nil || len(payload) != 1 {
		writeError(c, http.StatusUnprocessableEntity, "账号维护参数必须只包含 account_ids")
		return
	}
	rawIDs, ok := payload["account_ids"].([]any)
	if !ok || len(rawIDs) == 0 {
		writeError(c, http.StatusUnprocessableEntity, "请提供当前筛选结果中的账号 ID")
		return
	}
	accountIDs := make([]string, 0, len(rawIDs))
	for _, raw := range rawIDs {
		id, ok := raw.(string)
		id = strings.TrimSpace(id)
		if !ok || !positiveNumericID(id) {
			writeError(c, http.StatusUnprocessableEntity, "账号必须全部使用稳定数字 ID")
			return
		}
		accountIDs = append(accountIDs, id)
	}
	actor, err := s.requestActor(c)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "控制台会话读取失败")
		return
	}
	var task taskstore.Task
	switch operation {
	case "rates":
		task, err = s.accountMaintenance.EnqueueAccountRateSync(c.Request.Context(), accountIDs, actor)
	case "base-url":
		task, err = s.accountMaintenance.EnqueueAccountBaseURLValidation(c.Request.Context(), accountIDs, actor)
	case "configuration-check":
		task, err = s.accountMaintenance.EnqueueAccountConfigurationCheck(c.Request.Context(), accountIDs, actor)
	case "base-url-repair":
		task, err = s.accountMaintenance.EnqueueAccountBaseURLRepair(c.Request.Context(), accountIDs, actor)
	case "upstream-hosts":
		task, err = s.accountMaintenance.EnqueueAccountUpstreamHostRepair(c.Request.Context(), accountIDs, actor)
	case "names":
		task, err = s.accountMaintenance.EnqueueAccountNameRepair(c.Request.Context(), accountIDs, actor)
	case "defaults":
		task, err = s.accountMaintenance.EnqueueAccountDefaultsRepair(c.Request.Context(), accountIDs, actor)
	case "cleanup":
		task, err = s.accountMaintenance.EnqueueMissingBindingCleanup(c.Request.Context(), accountIDs, actor)
	default:
		task, err = s.accountMaintenance.EnqueueAccountRevalidation(c.Request.Context(), accountIDs, actor)
	}
	if err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(http.StatusOK, task)
}

func (s *Server) prepareOnboarding(c *gin.Context) {
	if s.onboarding == nil || s.upstreamSync == nil || s.upstreamConfigs == nil {
		writeError(c, http.StatusServiceUnavailable, "账号添加准备服务尚未就绪")
		return
	}
	payload, err := decodeRequestObject(c)
	if err != nil || len(payload) != 1 {
		writeError(c, http.StatusUnprocessableEntity, "账号添加准备参数必须只包含 host")
		return
	}
	host, err := requiredText(payload, "host", 1, 255)
	if err != nil {
		writeError(c, http.StatusUnprocessableEntity, err.Error())
		return
	}
	actor, err := s.requestActor(c)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "控制台会话读取失败")
		return
	}
	syncResult, err := s.upstreamSync.SyncHost(c.Request.Context(), host, upstreamsync.Scope{Catalog: true, Balance: true}, actor)
	if err != nil {
		writeError(c, http.StatusBadGateway, "上游信息读取失败："+err.Error())
		return
	}
	if syncResult.Status == "failed" || syncResult.Status == "auth_failed" {
		reason := "上游信息读取失败"
		if syncResult.Reason != nil && strings.TrimSpace(*syncResult.Reason) != "" {
			reason = strings.TrimSpace(*syncResult.Reason)
		}
		writeError(c, http.StatusConflict, reason)
		return
	}
	upstream, err := s.upstreamConfigs.Get(c.Request.Context(), host)
	if errors.Is(err, upstreamconfig.ErrNotFound) {
		writeError(c, http.StatusNotFound, err.Error())
		return
	}
	if err != nil {
		writeError(c, http.StatusConflict, "上游配置读取失败："+err.Error())
		return
	}
	candidates, err := s.onboarding.Candidates(c.Request.Context(), host)
	if err != nil {
		writeError(c, http.StatusConflict, "可添加分组读取失败："+err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"upstream": upstream, "candidates": candidates})
}

func (s *Server) onboardingProbeModels(c *gin.Context) {
	if s.onboarding == nil {
		writeError(c, http.StatusServiceUnavailable, "接入前探活服务尚未就绪")
		return
	}
	host, groupID, _, ok := onboardingProbePayload(c, false)
	if !ok {
		return
	}
	models, err := s.onboarding.ProbeModels(c.Request.Context(), host, groupID)
	if err != nil {
		writeError(c, http.StatusBadGateway, "上游模型读取失败："+err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"models": models})
}

func (s *Server) onboardingKeyCleanupPreview(c *gin.Context) {
	if s.onboarding == nil {
		writeError(c, http.StatusServiceUnavailable, "上游 Key 清理服务尚未就绪")
		return
	}
	payload, err := decodeRequestObject(c)
	if err != nil || len(payload) != 1 {
		writeError(c, http.StatusUnprocessableEntity, "无绑定 Key 扫描参数必须只包含 host")
		return
	}
	host, err := requiredText(payload, "host", 1, 255)
	if err != nil {
		writeError(c, http.StatusUnprocessableEntity, err.Error())
		return
	}
	preview, err := s.onboarding.PreviewUnboundKeys(c.Request.Context(), host)
	if err != nil {
		writeError(c, http.StatusBadGateway, err.Error())
		return
	}
	c.JSON(http.StatusOK, preview)
}

func (s *Server) onboardingKeyCleanup(c *gin.Context) {
	if s.onboarding == nil {
		writeError(c, http.StatusServiceUnavailable, "上游 Key 清理服务尚未就绪")
		return
	}
	payload, err := decodeRequestObject(c)
	if err != nil || len(payload) != 2 {
		writeError(c, http.StatusUnprocessableEntity, "无绑定 Key 清理参数必须只包含 host 和 key_ids")
		return
	}
	host, err := requiredText(payload, "host", 1, 255)
	if err != nil {
		writeError(c, http.StatusUnprocessableEntity, err.Error())
		return
	}
	keyIDs, err := onboardingKeyIDList(payload, "key_ids", 500)
	if err != nil {
		writeError(c, http.StatusUnprocessableEntity, err.Error())
		return
	}
	actor, err := s.requestActor(c)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "控制台会话读取失败")
		return
	}
	task, err := s.onboarding.EnqueueKeyCleanup(c.Request.Context(), host, keyIDs, actor)
	if err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(http.StatusOK, task)
}

func (s *Server) onboardingProbe(c *gin.Context) {
	if s.onboarding == nil {
		writeError(c, http.StatusServiceUnavailable, "接入前探活服务尚未就绪")
		return
	}
	host, groupID, model, ok := onboardingProbePayload(c, true)
	if !ok {
		return
	}
	result, err := s.onboarding.Probe(c.Request.Context(), host, groupID, model)
	if err != nil {
		if result.RequestModel != "" {
			result.Status = "failed"
			result.Message = err.Error()
			c.JSON(http.StatusOK, result)
			return
		}
		writeError(c, http.StatusBadGateway, "接入前探活失败："+err.Error())
		return
	}
	c.JSON(http.StatusOK, result)
}

func (s *Server) cancelOnboardingProbe(c *gin.Context) {
	if s.onboarding == nil {
		writeError(c, http.StatusServiceUnavailable, "接入前探活服务尚未就绪")
		return
	}
	host, groupID, _, ok := onboardingProbePayload(c, false)
	if !ok {
		return
	}
	if err := s.onboarding.CancelProbe(c.Request.Context(), host, groupID); err != nil {
		writeError(c, http.StatusBadGateway, "临时测试 Key 清理失败："+err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"cancelled": true})
}

func onboardingProbePayload(c *gin.Context, requireModel bool) (string, string, string, bool) {
	payload, err := decodeRequestObject(c)
	expected := 2
	if requireModel {
		expected = 3
	}
	if err != nil || len(payload) != expected {
		fields := "host、group_id"
		if requireModel {
			fields += " 和 model"
		}
		writeError(c, http.StatusUnprocessableEntity, "接入前探活参数必须只包含 "+fields)
		return "", "", "", false
	}
	host, err := requiredText(payload, "host", 1, 255)
	if err != nil {
		writeError(c, http.StatusUnprocessableEntity, err.Error())
		return "", "", "", false
	}
	groupID, err := requiredText(payload, "group_id", 1, 255)
	if err != nil {
		writeError(c, http.StatusUnprocessableEntity, err.Error())
		return "", "", "", false
	}
	model := ""
	if requireModel {
		model, err = requiredText(payload, "model", 1, 255)
		if err != nil {
			writeError(c, http.StatusUnprocessableEntity, err.Error())
			return "", "", "", false
		}
	}
	return host, groupID, model, true
}

func (s *Server) createOnboarding(c *gin.Context) {
	if s.onboarding == nil {
		writeError(c, http.StatusServiceUnavailable, "账号添加服务尚未就绪")
		return
	}
	payload, err := decodeRequestObject(c)
	if err != nil {
		writeError(c, http.StatusUnprocessableEntity, "账号添加参数必须是 JSON 对象")
		return
	}
	request, err := parseOnboardingRequest(payload)
	if err != nil {
		writeError(c, http.StatusUnprocessableEntity, err.Error())
		return
	}
	request.Actor, err = s.requestActor(c)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "控制台会话读取失败")
		return
	}
	task, err := s.onboarding.Enqueue(c.Request.Context(), request)
	if err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(http.StatusOK, task)
}

func (s *Server) createOnboardingBatch(c *gin.Context) {
	if s.onboarding == nil {
		writeError(c, http.StatusServiceUnavailable, "账号添加服务尚未就绪")
		return
	}
	payload, err := decodeRequestObject(c)
	if err != nil {
		writeError(c, http.StatusUnprocessableEntity, "批量账号添加参数必须是 JSON 对象")
		return
	}
	requests, err := parseOnboardingBatchRequests(payload)
	if err != nil {
		writeError(c, http.StatusUnprocessableEntity, err.Error())
		return
	}
	actor, err := s.requestActor(c)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "控制台会话读取失败")
		return
	}
	for index := range requests {
		requests[index].Actor = actor
	}
	task, err := s.onboarding.EnqueueBatch(c.Request.Context(), requests)
	if err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(http.StatusOK, task)
}

func parseOnboardingBatchRequests(payload map[string]any) ([]onboarding.Request, error) {
	if len(payload) != 1 {
		return nil, errors.New("批量账号添加参数只允许包含 items")
	}
	rawItems, found := payload["items"]
	if !found {
		return nil, errors.New("items 为必填项")
	}
	items, ok := rawItems.([]any)
	if !ok {
		return nil, errors.New("items 必须是数组")
	}
	if len(items) == 0 || len(items) > 50 {
		return nil, errors.New("items 数量必须在 1 到 50 之间")
	}
	requests := make([]onboarding.Request, 0, len(items))
	for index, raw := range items {
		item, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("items[%d] 必须是对象", index)
		}
		request, err := parseOnboardingRequest(item)
		if err != nil {
			return nil, fmt.Errorf("items[%d]：%w", index, err)
		}
		requests = append(requests, request)
	}
	return requests, nil
}

func parseOnboardingRequest(payload map[string]any) (onboarding.Request, error) {
	allowed := map[string]struct{}{
		"host": {}, "upstream_type": {}, "base_url": {}, "platform": {}, "account_type": {}, "notes": {},
		"local_group_id": {}, "local_group_ids": {}, "account_ids": {}, "upstream_group_id": {}, "extra": {},
		"priority": {}, "concurrency": {}, "schedulable": {},
	}
	for key := range payload {
		if _, found := allowed[key]; !found {
			return onboarding.Request{}, errors.New("账号添加参数包含未知字段：" + key)
		}
	}
	host, err := requiredText(payload, "host", 1, 2048)
	if err != nil {
		return onboarding.Request{}, err
	}
	upstreamType, err := requiredText(payload, "upstream_type", 2, 64)
	if err != nil {
		return onboarding.Request{}, err
	}
	upstreamGroupID, err := requiredText(payload, "upstream_group_id", 1, 255)
	if err != nil {
		return onboarding.Request{}, err
	}
	localGroupIDs, err := onboardingStableIDList(payload, "local_group_ids", 50)
	if err != nil {
		return onboarding.Request{}, err
	}
	if len(localGroupIDs) == 0 {
		rawLocalID, present := payload["local_group_id"]
		localID := ""
		if number, ok := rawLocalID.(json.Number); present && ok {
			localID = number.String()
		}
		if !positiveNumericID(localID) {
			return onboarding.Request{}, errors.New("local_group_ids 必须至少包含一个稳定正整数")
		}
		localGroupIDs = []string{localID}
	}
	accountIDs, err := onboardingStableIDList(payload, "account_ids", 50)
	if err != nil {
		return onboarding.Request{}, err
	}
	result := onboarding.Request{
		Host: host, UpstreamType: strings.ToLower(upstreamType),
		LocalGroupID: localGroupIDs[0], LocalGroupIDs: localGroupIDs, AccountIDs: accountIDs,
		UpstreamGroupID: upstreamGroupID, Extra: map[string]any{},
	}
	for field, target := range map[string]**string{
		"base_url": &result.BaseURL, "platform": &result.Platform, "account_type": &result.AccountType, "notes": &result.Notes,
	} {
		if raw, found := payload[field]; found {
			if field == "platform" {
				result.PlatformPresent = true
			}
			if raw == nil {
				continue
			}
			value, ok := raw.(string)
			if !ok || utf8.RuneCountInString(value) > 4096 {
				return onboarding.Request{}, errors.New(field + " 必须是字符串或 null")
			}
			*target = &value
		}
	}
	if raw, found := payload["extra"]; found {
		if raw == nil {
			result.Extra = nil
		} else if value, ok := raw.(map[string]any); ok {
			result.Extra = value
		} else {
			return onboarding.Request{}, errors.New("extra 必须是对象或 null")
		}
	}
	if raw, found := payload["priority"]; found {
		number, ok := raw.(json.Number)
		if !ok {
			return onboarding.Request{}, errors.New("priority 必须是整数")
		}
		value, err := number.Int64()
		if err != nil || value < 1 || value > 10_000_000 {
			return onboarding.Request{}, errors.New("priority 必须是 1 到 10000000 之间的整数")
		}
		result.Priority = &value
	}
	if raw, found := payload["concurrency"]; found {
		number, ok := raw.(json.Number)
		if !ok {
			return onboarding.Request{}, errors.New("concurrency 必须是整数")
		}
		value, err := number.Int64()
		if err != nil || value < 1 || value > 10_000_000 {
			return onboarding.Request{}, errors.New("concurrency 必须是 1 到 10000000 之间的整数")
		}
		result.Concurrency = &value
	}
	if raw, found := payload["schedulable"]; found {
		value, ok := raw.(bool)
		if !ok {
			return onboarding.Request{}, errors.New("schedulable 必须是布尔值")
		}
		result.Schedulable = value
	}
	return result, nil
}

func onboardingStableIDList(payload map[string]any, field string, maximum int) ([]string, error) {
	raw, present := payload[field]
	if !present || raw == nil {
		return nil, nil
	}
	values, ok := raw.([]any)
	if !ok || len(values) == 0 || len(values) > maximum {
		return nil, fmt.Errorf("%s 必须是包含 1 到 %d 个稳定正整数的数组", field, maximum)
	}
	result := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, rawValue := range values {
		value := ""
		switch typed := rawValue.(type) {
		case json.Number:
			value = typed.String()
		case string:
			value = strings.TrimSpace(typed)
		}
		if !positiveNumericID(value) {
			return nil, fmt.Errorf("%s 必须只包含稳定正整数", field)
		}
		if _, duplicate := seen[value]; duplicate {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result, nil
}

func onboardingKeyIDList(payload map[string]any, field string, maximum int) ([]string, error) {
	raw, present := payload[field]
	values, ok := raw.([]any)
	if !present || !ok || len(values) == 0 || len(values) > maximum {
		return nil, fmt.Errorf("%s 必须是包含 1 到 %d 个稳定 Key ID 的数组", field, maximum)
	}
	result := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, rawValue := range values {
		value, ok := rawValue.(string)
		value = strings.TrimSpace(value)
		if !ok || value == "" || len(value) > 255 {
			return nil, fmt.Errorf("%s 必须只包含有效的稳定 Key ID", field)
		}
		if _, duplicate := seen[value]; duplicate {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result, nil
}

func (s *Server) upstreamDeletePreview(c *gin.Context) {
	if s.upstreamDelete == nil {
		writeError(c, http.StatusServiceUnavailable, "上游删除服务尚未就绪")
		return
	}
	preview, err := s.upstreamDelete.Preview(c.Request.Context(), c.Param("host"))
	if err != nil {
		status := http.StatusConflict
		if strings.Contains(err.Error(), "不存在") {
			status = http.StatusNotFound
		}
		writeError(c, status, err.Error())
		return
	}
	c.JSON(http.StatusOK, preview)
}

func (s *Server) deleteUpstream(c *gin.Context) {
	if s.upstreamDelete == nil {
		writeError(c, http.StatusServiceUnavailable, "上游删除服务尚未就绪")
		return
	}
	payload, err := decodeRequestObject(c)
	if err != nil || len(payload) != 2 {
		writeError(c, http.StatusUnprocessableEntity, "上游删除参数必须只包含确认 Host 和预期账号 ID")
		return
	}
	confirmation, err := requiredText(payload, "confirmation_host", 1, 255)
	if err != nil || configstore.CanonicalHost(confirmation) != configstore.CanonicalHost(c.Param("host")) {
		writeError(c, http.StatusUnprocessableEntity, "确认 Host 与删除目标不一致")
		return
	}
	rawIDs, present := payload["expected_account_ids"]
	items, ok := rawIDs.([]any)
	if !present || !ok || len(items) > 100000 {
		writeError(c, http.StatusUnprocessableEntity, "预期账号 ID 必须是数组")
		return
	}
	expected := make([]string, 0, len(items))
	seen := map[string]struct{}{}
	for _, raw := range items {
		value, ok := raw.(string)
		value = strings.TrimSpace(value)
		if !ok || !positiveNumericID(value) {
			writeError(c, http.StatusUnprocessableEntity, "预期账号必须全部使用稳定数字 ID")
			return
		}
		if _, found := seen[value]; found {
			writeError(c, http.StatusUnprocessableEntity, "预期账号 ID 不能重复")
			return
		}
		seen[value] = struct{}{}
		expected = append(expected, value)
	}
	actor, err := s.requestActor(c)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "控制台会话读取失败")
		return
	}
	task, err := s.upstreamDelete.Enqueue(c.Request.Context(), c.Param("host"), expected, actor)
	if err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(http.StatusOK, task)
}

func (s *Server) syncAllUpstreamBalances(c *gin.Context) {
	s.enqueueAllUpstreamSync(c, upstreamsync.Scope{Balance: true}, "upstream-balances-sync")
}

func (s *Server) repairUpstreamNames(c *gin.Context) {
	s.enqueueAllUpstreamSync(c, upstreamsync.Scope{Name: true}, "upstream-name-repair")
}

func (s *Server) syncAllUpstreamGroups(c *gin.Context) {
	s.enqueueAllUpstreamSync(c, upstreamsync.Scope{Catalog: true}, "upstream-groups-sync")
}

func (s *Server) syncAllUpstreams(c *gin.Context) {
	s.enqueueAllUpstreamSync(c, upstreamsync.Scope{Catalog: true, Balance: true}, "upstream-sync")
}

func (s *Server) enqueueAllUpstreamSync(c *gin.Context, scope upstreamsync.Scope, operation string) {
	if s.upstreamSync == nil {
		writeError(c, http.StatusServiceUnavailable, "上游同步任务服务尚未就绪")
		return
	}
	if !s.requireUpstreamDataCollectionMode(c) {
		return
	}
	actor, err := s.requestActor(c)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "控制台会话读取失败")
		return
	}
	task, err := s.upstreamSync.EnqueueAll(c.Request.Context(), scope, actor, operation)
	if err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(http.StatusOK, task)
}

func (s *Server) syncUpstreamBalance(c *gin.Context) {
	s.enqueueHostUpstreamSync(c, upstreamsync.Scope{Balance: true}, "balance-sync")
}

func (s *Server) syncUpstreamRates(c *gin.Context) {
	payload, err := decodeRequestObject(c)
	if err != nil {
		writeError(c, http.StatusUnprocessableEntity, "倍率同步参数必须是 JSON 对象")
		return
	}
	for key := range payload {
		if key != "host" && key != "key_id" {
			writeError(c, http.StatusUnprocessableEntity, "倍率同步参数包含未知字段："+key)
			return
		}
	}
	bodyHost, err := requiredText(payload, "host", 1, 255)
	if err != nil {
		writeError(c, http.StatusUnprocessableEntity, err.Error())
		return
	}
	if configstore.CanonicalHost(bodyHost) != configstore.CanonicalHost(c.Param("host")) {
		writeError(c, http.StatusUnprocessableEntity, "Host 路径与请求体不一致")
		return
	}
	scope := upstreamsync.Scope{Catalog: true, Balance: true}
	if raw, present := payload["key_id"]; present {
		keyID, ok := raw.(string)
		if !ok || strings.TrimSpace(keyID) == "" || utf8.RuneCountInString(keyID) > 255 {
			writeError(c, http.StatusUnprocessableEntity, "已提交的 Key ID 不能是空值且长度不能超过 255")
			return
		}
		keyID = strings.TrimSpace(keyID)
		scope.KeyID = &keyID
	}
	s.enqueueHostUpstreamSync(c, scope, "rate-sync")
}

func (s *Server) enqueueHostUpstreamSync(c *gin.Context, scope upstreamsync.Scope, operation string) {
	if s.upstreamSync == nil {
		writeError(c, http.StatusServiceUnavailable, "上游同步任务服务尚未就绪")
		return
	}
	if !s.requireUpstreamDataCollectionMode(c) {
		return
	}
	actor, err := s.requestActor(c)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "控制台会话读取失败")
		return
	}
	task, err := s.upstreamSync.EnqueueHost(c.Request.Context(), c.Param("host"), scope, actor, operation)
	if err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(http.StatusOK, task)
}

func (s *Server) requireUpstreamDataCollectionMode(c *gin.Context) bool {
	mode, err := s.business.Mode(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusInternalServerError, "运行模式读取失败")
		return false
	}
	capabilities, valid := runtimepolicy.For(mode)
	if !valid {
		writeError(c, http.StatusConflict, "运行模式无效："+mode)
		return false
	}
	if !capabilities.AutomaticUpstreamSync {
		writeError(c, http.StatusConflict, "当前运行模式不允许采集上游数据")
		return false
	}
	return true
}

func (s *Server) groups(c *gin.Context) {
	rows, err := s.business.Groups(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusInternalServerError, "分组列表读取失败")
		return
	}
	c.JSON(http.StatusOK, rows)
}

func (s *Server) groupAllocation(c *gin.Context) {
	groupID := strings.TrimSpace(c.Param("group_id"))
	if !positiveNumericID(groupID) {
		writeError(c, http.StatusUnprocessableEntity, "分组必须使用已登记的稳定数字 ID")
		return
	}
	allocation, err := s.business.GroupAllocation(c.Request.Context(), groupID)
	if errors.Is(err, business.ErrGroupNotFound) {
		writeError(c, http.StatusNotFound, err.Error())
		return
	}
	if err != nil {
		writeError(c, http.StatusInternalServerError, "分组分配详情读取失败")
		return
	}
	c.JSON(http.StatusOK, allocation)
}

func (s *Server) groupProbeModels(c *gin.Context) {
	groupID := strings.TrimSpace(c.Param("group_id"))
	if !positiveNumericID(groupID) {
		writeError(c, http.StatusUnprocessableEntity, "分组必须使用已登记的稳定数字 ID")
		return
	}
	models, err := s.business.GroupProbeModels(c.Request.Context(), groupID)
	if errors.Is(err, business.ErrGroupNotFound) {
		writeError(c, http.StatusNotFound, err.Error())
		return
	}
	if err != nil {
		writeError(c, http.StatusInternalServerError, "分组探测模型读取失败")
		return
	}
	c.JSON(http.StatusOK, models)
}

func (s *Server) policy(c *gin.Context) {
	snapshot, err := s.business.PolicySnapshot(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusInternalServerError, "调度策略读取失败")
		return
	}
	c.JSON(http.StatusOK, snapshot)
}

func (s *Server) updatePolicy(c *gin.Context) {
	patch, err := decodeRequestObject(c)
	if err != nil {
		writeError(c, http.StatusUnprocessableEntity, "策略更新参数必须是 JSON 对象")
		return
	}
	actor, err := s.requestActor(c)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "控制台会话读取失败")
		return
	}
	snapshot, err := s.business.UpdatePolicy(c.Request.Context(), patch, actor)
	if err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(http.StatusOK, snapshot)
}

func (s *Server) restoreRoutingControl(c *gin.Context) {
	if s.routingControl == nil {
		writeError(c, http.StatusServiceUnavailable, "交还控制权服务尚未就绪")
		return
	}
	actor, err := s.requestActor(c)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "控制台会话读取失败")
		return
	}
	result, err := s.routingControl.RestoreControl(c.Request.Context(), actor)
	if err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	status := "succeeded"
	if result.Failed > 0 {
		status = "failed"
	}
	s.recordRuntimeEventBestEffort(c.Request.Context(), "routing.control.restored", status,
		fmt.Sprintf("交还调度控制权：恢复 %d，失败 %d", result.Restored, result.Failed), map[string]any{
			"actor": actor, "restored": result.Restored, "failed": result.Failed, "remote_write": result.RemoteWrite,
		})
	c.JSON(http.StatusOK, result)
}

func (s *Server) pricingSnapshot(c *gin.Context) {
	if s.pricing == nil {
		writeError(c, http.StatusServiceUnavailable, "价格管理服务尚未就绪")
		return
	}
	snapshot, err := s.pricing.Snapshot(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(http.StatusOK, snapshot)
}

func (s *Server) pricingChanges(c *gin.Context) {
	if s.pricing == nil {
		writeError(c, http.StatusServiceUnavailable, "价格管理服务尚未就绪")
		return
	}
	records, err := s.pricing.Changes(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusInternalServerError, "价格变更记录读取失败")
		return
	}
	c.JSON(http.StatusOK, records)
}

func (s *Server) updatePricingConfig(c *gin.Context) {
	if s.pricing == nil {
		writeError(c, http.StatusServiceUnavailable, "价格管理服务尚未就绪")
		return
	}
	var request pricing.Config
	if err := bindRequestJSON(c, &request); err != nil {
		writeError(c, http.StatusUnprocessableEntity, "价格管理配置必须是完整 JSON 对象")
		return
	}
	actor, err := s.requestActor(c)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "控制台会话读取失败")
		return
	}
	snapshot, err := s.pricing.UpdateConfig(c.Request.Context(), request, actor)
	if err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(http.StatusOK, snapshot)
}

func (s *Server) applyPricing(c *gin.Context) {
	if s.pricing == nil {
		writeError(c, http.StatusServiceUnavailable, "价格管理服务尚未就绪")
		return
	}
	actor, err := s.requestActor(c)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "控制台会话读取失败")
		return
	}
	task, err := s.pricing.Enqueue(c.Request.Context(), actor)
	if err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(http.StatusAccepted, task)
}

func (s *Server) calculatePricingRevenue(c *gin.Context) {
	if s.pricing == nil {
		writeError(c, http.StatusServiceUnavailable, "收入核算服务尚未就绪")
		return
	}
	var request pricing.RevenueRequest
	if err := bindRequestJSON(c, &request); err != nil {
		writeError(c, http.StatusUnprocessableEntity, "收入核算参数必须是 JSON 对象")
		return
	}
	actor, err := s.requestActor(c)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "控制台会话读取失败")
		return
	}
	task, err := s.pricing.EnqueueRevenue(c.Request.Context(), request, actor)
	if err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(http.StatusAccepted, task)
}

func (s *Server) latestPricingRevenue(c *gin.Context) {
	if s.tasks == nil {
		writeError(c, http.StatusServiceUnavailable, "收益分析历史尚未就绪")
		return
	}
	task, err := s.tasks.LatestByOperation(c.Request.Context(), "revenue-calculation", "succeeded")
	if errors.Is(err, taskstore.ErrNotFound) {
		c.JSON(http.StatusOK, nil)
		return
	}
	if err != nil {
		writeError(c, http.StatusInternalServerError, "最近收益分析读取失败")
		return
	}
	c.JSON(http.StatusOK, task)
}

func (s *Server) pricingBackups(c *gin.Context) {
	if s.pricing == nil {
		writeError(c, http.StatusServiceUnavailable, "价格管理服务尚未就绪")
		return
	}
	backups, err := s.pricing.Backups(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(http.StatusOK, backups)
}

func (s *Server) createPricingBackup(c *gin.Context) {
	if s.pricing == nil {
		writeError(c, http.StatusServiceUnavailable, "价格管理服务尚未就绪")
		return
	}
	payload, err := decodeRequestObject(c)
	if err != nil {
		writeError(c, http.StatusUnprocessableEntity, err.Error())
		return
	}
	if len(payload) != 1 {
		writeError(c, http.StatusUnprocessableEntity, "创建价格分组备份参数必须只包含 name")
		return
	}
	name, err := requiredText(payload, "name", 1, 80)
	if err != nil {
		writeError(c, http.StatusUnprocessableEntity, err.Error())
		return
	}
	actor, err := s.requestActor(c)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "控制台会话读取失败")
		return
	}
	backup, err := s.pricing.CreateBackup(c.Request.Context(), name, actor)
	if err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(http.StatusCreated, backup)
}

func (s *Server) deletePricingBackup(c *gin.Context) {
	if s.pricing == nil {
		writeError(c, http.StatusServiceUnavailable, "价格管理服务尚未就绪")
		return
	}
	backupID := strings.TrimSpace(c.Param("backup_id"))
	if backupID == "" || len(backupID) > 128 {
		writeError(c, http.StatusUnprocessableEntity, "备份 ID 无效")
		return
	}
	if err := s.pricing.DeleteBackup(c.Request.Context(), backupID); err != nil {
		if errors.Is(err, business.ErrPricingBackupNotFound) {
			writeError(c, http.StatusNotFound, err.Error())
			return
		}
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": backupID, "deleted": true})
}

func (s *Server) restorePricingBackup(c *gin.Context) {
	if s.pricing == nil {
		writeError(c, http.StatusServiceUnavailable, "价格管理服务尚未就绪")
		return
	}
	backupID := strings.TrimSpace(c.Param("backup_id"))
	if backupID == "" || len(backupID) > 128 {
		writeError(c, http.StatusUnprocessableEntity, "备份 ID 无效")
		return
	}
	actor, err := s.requestActor(c)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "控制台会话读取失败")
		return
	}
	task, err := s.pricing.EnqueueRestore(c.Request.Context(), backupID, actor)
	if err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(http.StatusAccepted, task)
}

func (s *Server) updateGroupPolicy(c *gin.Context) {
	groupID := strings.TrimSpace(c.Param("group_id"))
	patch, err := decodeRequestObject(c)
	if err != nil {
		writeError(c, http.StatusUnprocessableEntity, "分组策略参数必须是 JSON 对象")
		return
	}
	actor, err := s.requestActor(c)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "控制台会话读取失败")
		return
	}
	row, err := s.business.UpdateGroupPolicy(c.Request.Context(), groupID, patch, actor)
	if errors.Is(err, business.ErrGroupNotFound) {
		writeError(c, http.StatusNotFound, err.Error())
		return
	}
	if err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(http.StatusOK, row)
}

func (s *Server) clearGroupPolicy(c *gin.Context) {
	groupID := strings.TrimSpace(c.Param("group_id"))
	actor, err := s.requestActor(c)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "控制台会话读取失败")
		return
	}
	row, err := s.business.ClearGroupPolicy(c.Request.Context(), groupID, actor)
	if errors.Is(err, business.ErrGroupNotFound) {
		writeError(c, http.StatusNotFound, err.Error())
		return
	}
	if err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(http.StatusOK, row)
}

func (s *Server) setGroupExcluded(c *gin.Context) {
	groupID := strings.TrimSpace(c.Param("group_id"))
	payload, err := decodeRequestObject(c)
	if err != nil || len(payload) != 1 {
		writeError(c, http.StatusUnprocessableEntity, "分组排除参数必须只包含 excluded")
		return
	}
	excluded, ok := payload["excluded"].(bool)
	if !ok {
		writeError(c, http.StatusUnprocessableEntity, "excluded 必须是布尔值")
		return
	}
	actor, err := s.requestActor(c)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "控制台会话读取失败")
		return
	}
	row, err := s.business.SetGroupExcluded(c.Request.Context(), groupID, excluded, actor)
	if errors.Is(err, business.ErrGroupNotFound) {
		writeError(c, http.StatusNotFound, err.Error())
		return
	}
	if err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(http.StatusOK, row)
}

func bindRequestJSON(c *gin.Context, target any) error {
	if err := requireJSONContentType(c.Request); err != nil {
		return err
	}
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err != nil {
			return err
		}
		return errors.New("request body contains trailing JSON")
	}
	return binding.Validator.ValidateStruct(target)
}

func decodeRequestObject(c *gin.Context) (map[string]any, error) {
	if err := requireJSONContentType(c.Request); err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(c.Request.Body)
	decoder.UseNumber()
	var value map[string]any
	if err := decoder.Decode(&value); err != nil || value == nil {
		return nil, errors.New("request body is not an object")
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, errors.New("request body contains trailing JSON")
	}
	return value, nil
}

func requireJSONContentType(request *http.Request) error {
	mediaType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil || !strings.EqualFold(mediaType, "application/json") {
		return errors.New("Content-Type must be application/json")
	}
	return nil
}

func (s *Server) requestActor(c *gin.Context) (string, error) {
	username, err := s.sessionUser(c)
	if err != nil {
		return "", err
	}
	if username != nil && strings.TrimSpace(*username) != "" {
		return strings.TrimSpace(*username), nil
	}
	return "console", nil
}

func (s *Server) detectUpstream(c *gin.Context) {
	if s.upstreamDetect == nil {
		writeError(c, http.StatusServiceUnavailable, "上游识别服务尚未就绪")
		return
	}
	payload, err := decodeRequestObject(c)
	if err != nil || len(payload) != 1 {
		writeError(c, http.StatusUnprocessableEntity, "上游识别参数必须只包含 base_url")
		return
	}
	baseURL, ok := payload["base_url"].(string)
	if !ok || utf8.RuneCountInString(baseURL) < 3 || utf8.RuneCountInString(baseURL) > 2048 {
		writeError(c, http.StatusUnprocessableEntity, "上游地址长度必须在 3 到 2048 之间")
		return
	}
	result, err := s.upstreamDetect.Detect(c.Request.Context(), baseURL)
	if err != nil {
		writeError(c, http.StatusUnprocessableEntity, err.Error())
		return
	}
	c.JSON(http.StatusOK, result)
}

func (s *Server) upstreams(c *gin.Context) {
	result, err := s.business.Upstreams(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusInternalServerError, "上游列表读取失败")
		return
	}
	c.JSON(http.StatusOK, result)
}

func (s *Server) upstreamConfiguration(c *gin.Context) {
	if s.upstreamConfigs == nil {
		writeError(c, http.StatusServiceUnavailable, "上游配置服务尚未就绪")
		return
	}
	result, err := s.upstreamConfigs.Get(c.Request.Context(), c.Param("host"))
	if errors.Is(err, upstreamconfig.ErrNotFound) {
		writeError(c, http.StatusNotFound, err.Error())
		return
	}
	if err != nil {
		writeError(c, http.StatusConflict, "上游配置读取失败")
		return
	}
	c.JSON(http.StatusOK, result)
}

func (s *Server) createUpstream(c *gin.Context) {
	if s.upstreamConfigs == nil {
		writeError(c, http.StatusServiceUnavailable, "上游配置服务尚未就绪")
		return
	}
	payload, err := decodeRequestObject(c)
	if err != nil {
		writeError(c, http.StatusUnprocessableEntity, "上游添加参数必须是 JSON 对象")
		return
	}
	input, err := parseUpstreamInput(payload, true)
	if err != nil {
		writeError(c, http.StatusUnprocessableEntity, err.Error())
		return
	}
	actor, err := s.requestActor(c)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "控制台会话读取失败")
		return
	}
	result, err := s.upstreamConfigs.Create(c.Request.Context(), input, actor)
	writeUpstreamMutation(c, result, err)
}

func (s *Server) updateUpstreamConfiguration(c *gin.Context) {
	if s.upstreamConfigs == nil {
		writeError(c, http.StatusServiceUnavailable, "上游配置服务尚未就绪")
		return
	}
	payload, err := decodeRequestObject(c)
	if err != nil {
		writeError(c, http.StatusUnprocessableEntity, "上游编辑参数必须是 JSON 对象")
		return
	}
	input, err := parseUpstreamInput(payload, false)
	if err != nil {
		writeError(c, http.StatusUnprocessableEntity, err.Error())
		return
	}
	actor, err := s.requestActor(c)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "控制台会话读取失败")
		return
	}
	result, err := s.upstreamConfigs.Update(c.Request.Context(), c.Param("host"), input, actor)
	writeUpstreamMutation(c, result, err)
}

func writeUpstreamMutation(c *gin.Context, result upstreamconfig.Configuration, err error) {
	var inputError *upstreamconfig.InputError
	switch {
	case err == nil:
		c.JSON(http.StatusOK, result)
	case errors.Is(err, upstreamconfig.ErrNotFound):
		writeError(c, http.StatusNotFound, err.Error())
	case errors.Is(err, upstreamconfig.ErrConflict):
		writeError(c, http.StatusConflict, err.Error())
	case errors.As(err, &inputError):
		writeError(c, http.StatusUnprocessableEntity, inputError.Error())
	default:
		writeError(c, http.StatusInternalServerError, "上游配置保存失败")
	}
}

func parseUpstreamInput(payload map[string]any, creating bool) (upstreamconfig.Input, error) {
	allowed := map[string]struct{}{
		"host": {}, "name": {}, "base_url": {}, "account_base_url": {}, "upstream_type": {}, "auth_mode": {}, "recharge_rate": {},
		"access_token": {}, "refresh_token": {}, "admin_key": {}, "user_id": {}, "headers": {}, "cookies": {},
		"username": {}, "password": {}, "save_to_vault": {}, "entry": {},
	}
	for key := range payload {
		if _, found := allowed[key]; !found {
			return upstreamconfig.Input{}, fmt.Errorf("上游配置包含未知字段：%s", key)
		}
	}
	baseURL, err := requiredText(payload, "base_url", 3, 2048)
	if err != nil {
		return upstreamconfig.Input{}, err
	}
	accountBaseURL := baseURL
	if raw, present := payload["account_base_url"]; present {
		value, valueErr := nullableTextField(raw, "account_base_url", 3, 2048)
		if valueErr != nil || value == nil {
			return upstreamconfig.Input{}, errors.New("account_base_url 必须是完整的 HTTP/HTTPS 地址")
		}
		accountBaseURL = *value
	}
	platform, err := requiredText(payload, "upstream_type", 2, 40)
	if err != nil {
		return upstreamconfig.Input{}, err
	}
	authMode, err := requiredText(payload, "auth_mode", 2, 80)
	if err != nil {
		return upstreamconfig.Input{}, err
	}
	recharge, err := requiredText(payload, "recharge_rate", 1, 64)
	if err != nil {
		return upstreamconfig.Input{}, err
	}
	result := upstreamconfig.Input{
		BaseURL: baseURL, AccountBaseURL: accountBaseURL, UpstreamType: platform, AuthMode: authMode, RechargeRate: recharge,
		Headers: map[string]string{}, Cookies: map[string]string{}, Present: map[string]bool{},
	}
	if creating {
		result.Host, err = requiredText(payload, "host", 1, 255)
		if err != nil {
			return upstreamconfig.Input{}, err
		}
		name, nameErr := requiredText(payload, "name", 1, 100)
		if nameErr != nil {
			return upstreamconfig.Input{}, nameErr
		}
		result.Name = &name
	} else if raw, present := payload["name"]; present {
		result.Name, err = nullableTextField(raw, "name", 1, 100)
		if err != nil {
			return upstreamconfig.Input{}, err
		}
	}
	for _, field := range []string{"access_token", "refresh_token", "admin_key", "user_id", "username", "password", "entry"} {
		raw, present := payload[field]
		if !present {
			continue
		}
		maximum := 65536
		if field == "user_id" || field == "entry" {
			maximum = 255
		}
		value, fieldErr := nullableTextField(raw, field, 0, maximum)
		if fieldErr != nil {
			return upstreamconfig.Input{}, fieldErr
		}
		result.Present[field] = true
		switch field {
		case "access_token":
			result.AccessToken = value
		case "refresh_token":
			result.RefreshToken = value
		case "admin_key":
			result.AdminKey = value
		case "user_id":
			result.UserID = value
		case "username":
			result.Username = value
		case "password":
			result.Password = value
		case "entry":
			result.Entry = value
		}
	}
	for _, field := range []string{"headers", "cookies"} {
		raw, present := payload[field]
		if !present {
			continue
		}
		value, fieldErr := nullableStringMap(raw, field)
		if fieldErr != nil {
			return upstreamconfig.Input{}, fieldErr
		}
		result.Present[field] = true
		if field == "headers" {
			result.Headers = value
		} else {
			result.Cookies = value
		}
	}
	if raw, present := payload["save_to_vault"]; present {
		value, ok := raw.(bool)
		if !ok {
			return upstreamconfig.Input{}, errors.New("save_to_vault 必须是布尔值")
		}
		result.SaveToVault = value
		result.Present["save_to_vault"] = true
	}
	return result, nil
}

func parseVaultEntry(payload map[string]any) (configstore.VaultEntry, map[string]bool, error) {
	allowed := map[string]struct{}{"entry": {}, "username": {}, "password": {}, "hosts": {}, "headers": {}}
	for key := range payload {
		if _, found := allowed[key]; !found {
			return configstore.VaultEntry{}, nil, fmt.Errorf("密码箱参数包含未知字段：%s", key)
		}
	}
	name, err := requiredText(payload, "entry", 1, 255)
	if err != nil {
		return configstore.VaultEntry{}, nil, err
	}
	result := configstore.VaultEntry{Entry: name, Hosts: []string{}, Headers: map[string]string{}}
	present := map[string]bool{}
	for _, field := range []string{"username", "password"} {
		raw, found := payload[field]
		if !found {
			continue
		}
		value, fieldErr := nullableTextField(raw, field, 0, 65536)
		if fieldErr != nil {
			return configstore.VaultEntry{}, nil, fieldErr
		}
		present[field] = true
		if field == "username" {
			result.Username = value
		} else {
			result.Password = value
		}
	}
	if raw, found := payload["hosts"]; found {
		values, ok := raw.([]any)
		if !ok || len(values) > 100 {
			return configstore.VaultEntry{}, nil, errors.New("hosts 必须是最多包含 100 项的字符串数组")
		}
		for _, value := range values {
			text, ok := value.(string)
			if !ok || strings.TrimSpace(text) == "" || utf8.RuneCountInString(text) > 255 {
				return configstore.VaultEntry{}, nil, errors.New("hosts 必须是最多包含 100 项的非空字符串数组")
			}
			result.Hosts = append(result.Hosts, text)
		}
		present["hosts"] = true
	}
	if raw, found := payload["headers"]; found {
		result.Headers, err = nullableStringMap(raw, "headers")
		if err != nil {
			return configstore.VaultEntry{}, nil, err
		}
		present["headers"] = true
	}
	return result, present, nil
}

func requiredText(payload map[string]any, field string, minimum, maximum int) (string, error) {
	raw, present := payload[field]
	if !present {
		return "", fmt.Errorf("%s 为必填字段", field)
	}
	value, ok := raw.(string)
	length := utf8.RuneCountInString(value)
	if !ok || length < minimum || length > maximum {
		return "", fmt.Errorf("%s 长度必须在 %d 到 %d 之间", field, minimum, maximum)
	}
	return value, nil
}

func nullableTextField(raw any, field string, minimum, maximum int) (*string, error) {
	if raw == nil {
		return nil, nil
	}
	value, ok := raw.(string)
	length := utf8.RuneCountInString(value)
	if !ok || length < minimum || length > maximum {
		return nil, fmt.Errorf("%s 必须是 null 或长度在 %d 到 %d 之间的字符串", field, minimum, maximum)
	}
	return &value, nil
}

func nullableStringMap(raw any, field string) (map[string]string, error) {
	if raw == nil {
		return map[string]string{}, nil
	}
	object, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s 必须是字符串对象或 null", field)
	}
	if len(object) > 100 {
		return nil, fmt.Errorf("%s 最多包含 100 项", field)
	}
	result := make(map[string]string, len(object))
	for key, rawValue := range object {
		value, ok := rawValue.(string)
		if !ok || utf8.RuneCountInString(key) > 255 || len(value) > 65536 {
			return nil, fmt.Errorf("%s 的名称和值必须是有效字符串", field)
		}
		result[key] = value
	}
	return result, nil
}

type privateAuthConfigResponse struct {
	AuthRecords  []privateAuthRecordIndex        `json:"auth_records"`
	VaultEntries []configstore.VaultEntrySummary `json:"vault_entries"`
}

type privateAuthRecordIndex struct {
	Host       string `json:"host"`
	Configured bool   `json:"configured"`
	HasHeaders bool   `json:"has_headers"`
}

func (s *Server) authRecoveryConfiguration(c *gin.Context) {
	authRecords, err := s.private.AuthRecordIndex(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusConflict, "私有授权记录读取失败")
		return
	}
	vaultEntries, err := s.private.VaultEntryIndex(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusConflict, "密码箱索引读取失败")
		return
	}
	result := privateAuthConfigResponse{AuthRecords: []privateAuthRecordIndex{}, VaultEntries: vaultEntries}
	for _, record := range authRecords {
		result.AuthRecords = append(result.AuthRecords, privateAuthRecordIndex{
			Host:       record.Host,
			Configured: record.HasAccessToken || record.HasRefreshToken || record.HasAdminKey || len(record.HeaderNames) > 0 || len(record.CookieNames) > 0,
			HasHeaders: len(record.HeaderNames) > 0,
		})
	}
	c.JSON(http.StatusOK, result)
}

func (s *Server) configureVaultEntry(c *gin.Context) {
	payload, err := decodeRequestObject(c)
	if err != nil {
		writeError(c, http.StatusUnprocessableEntity, "密码箱参数必须是 JSON 对象")
		return
	}
	entry, present, err := parseVaultEntry(payload)
	if err != nil {
		writeError(c, http.StatusUnprocessableEntity, err.Error())
		return
	}
	guarded, release, err := s.acquireVaultMutation(c.Request.Context(), entry.Entry)
	if err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	defer release()
	if err := s.private.SaveVaultEntry(guarded, entry, present); err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"entry": strings.TrimSpace(entry.Entry), "configured": true})
}

func (s *Server) deleteVaultEntry(c *gin.Context) {
	entry := strings.TrimSpace(c.Query("entry"))
	if entry == "" || utf8.RuneCountInString(entry) > 255 {
		writeError(c, http.StatusUnprocessableEntity, "凭据名称长度必须在 1 到 255 之间")
		return
	}
	guarded, release, err := s.acquireVaultMutation(c.Request.Context(), entry)
	if err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	defer release()
	deleted, err := s.private.DeleteVaultEntry(guarded, entry)
	if err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"entry": entry, "deleted": deleted})
}

func (s *Server) acquireVaultMutation(ctx context.Context, entry string) (context.Context, func(), error) {
	guarded, release, err := mutationguard.Acquire(ctx, s.business, mutationguard.Vault(entry))
	if err != nil {
		return nil, nil, err
	}
	released := false
	return guarded, func() {
		if released {
			return
		}
		released = true
		if err := release(); err != nil {
			slog.Error("密码箱变更租约释放失败", "entry", strings.TrimSpace(entry), "error", err)
		}
	}, nil
}

func (s *Server) verifyManualAuth(c *gin.Context) {
	if s.authRecovery == nil {
		writeError(c, http.StatusServiceUnavailable, "鉴权验证服务尚未就绪")
		return
	}
	payload, err := decodeRequestObject(c)
	if err != nil {
		writeError(c, http.StatusUnprocessableEntity, "手动鉴权参数必须是 JSON 对象")
		return
	}
	input, err := parseManualAuthInput(payload)
	if err != nil {
		writeError(c, http.StatusUnprocessableEntity, err.Error())
		return
	}
	actor, err := s.requestActor(c)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "控制台会话读取失败")
		return
	}
	result, err := s.authRecovery.VerifyManual(c.Request.Context(), input, actor)
	if err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(http.StatusOK, result)
}

func (s *Server) runAuthRecovery(c *gin.Context) {
	if s.authRecovery == nil {
		writeError(c, http.StatusServiceUnavailable, "鉴权恢复服务尚未就绪")
		return
	}
	payload, err := decodeRequestObject(c)
	if err != nil {
		writeError(c, http.StatusUnprocessableEntity, "鉴权恢复参数必须是 JSON 对象")
		return
	}
	for key := range payload {
		if key != "host" && key != "entry" && key != "accept_login_agreement" {
			writeError(c, http.StatusUnprocessableEntity, "鉴权恢复参数包含未知字段："+key)
			return
		}
	}
	host, err := requiredText(payload, "host", 1, 255)
	if err != nil {
		writeError(c, http.StatusUnprocessableEntity, err.Error())
		return
	}
	entry := ""
	if raw, present := payload["entry"]; present {
		value, ok := raw.(string)
		if !ok || utf8.RuneCountInString(strings.TrimSpace(value)) > 255 {
			writeError(c, http.StatusUnprocessableEntity, "entry 必须是长度不超过 255 的字符串")
			return
		}
		entry = strings.TrimSpace(value)
	}
	acceptLoginAgreement := false
	if raw, present := payload["accept_login_agreement"]; present {
		value, ok := raw.(bool)
		if !ok {
			writeError(c, http.StatusUnprocessableEntity, "accept_login_agreement 必须是布尔值")
			return
		}
		acceptLoginAgreement = value
	}
	actor, err := s.requestActor(c)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "控制台会话读取失败")
		return
	}
	task, err := s.authRecovery.Enqueue(c.Request.Context(), host, entry, acceptLoginAgreement, actor)
	if err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(http.StatusOK, task)
}

func (s *Server) runAuthRecoveryBatch(c *gin.Context) {
	if s.authRecovery == nil {
		writeError(c, http.StatusServiceUnavailable, "鉴权恢复服务尚未就绪")
		return
	}
	payload, err := decodeRequestObject(c)
	if err != nil {
		writeError(c, http.StatusUnprocessableEntity, "批量鉴权恢复参数必须是 JSON 对象")
		return
	}
	if len(payload) != 1 {
		writeError(c, http.StatusUnprocessableEntity, "批量鉴权恢复只允许 hosts 字段")
		return
	}
	hosts, err := authRecoveryHosts(payload["hosts"])
	if err != nil {
		writeError(c, http.StatusUnprocessableEntity, err.Error())
		return
	}
	actor, err := s.requestActor(c)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "控制台会话读取失败")
		return
	}
	task, err := s.authRecovery.EnqueueBatch(c.Request.Context(), hosts, actor)
	if err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(http.StatusOK, task)
}

func authRecoveryHosts(raw any) ([]string, error) {
	values, ok := raw.([]any)
	if !ok || len(values) == 0 || len(values) > 100 {
		return nil, errors.New("hosts 必须是包含 1 到 100 项的字符串数组")
	}
	hosts := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, rawHost := range values {
		host, ok := rawHost.(string)
		host = configstore.CanonicalHost(host)
		if !ok || host == "" || utf8.RuneCountInString(host) > 255 {
			return nil, errors.New("hosts 必须全部是长度不超过 255 的有效 Host")
		}
		if _, found := seen[host]; found {
			return nil, errors.New("hosts 不能包含重复 Host：" + host)
		}
		seen[host] = struct{}{}
		hosts = append(hosts, host)
	}
	return hosts, nil
}

func (s *Server) submitAuthCaptcha(c *gin.Context) {
	if s.authRecovery == nil {
		writeError(c, http.StatusServiceUnavailable, "图片验证码恢复服务尚未就绪")
		return
	}
	payload, err := decodeRequestObject(c)
	if err != nil {
		writeError(c, http.StatusUnprocessableEntity, "验证码参数必须是 JSON 对象")
		return
	}
	for key := range payload {
		if key != "challenge_id" && key != "captcha_code" {
			writeError(c, http.StatusUnprocessableEntity, "验证码参数包含未知字段："+key)
			return
		}
	}
	challengeID, err := requiredText(payload, "challenge_id", 1, 512)
	if err != nil {
		writeError(c, http.StatusUnprocessableEntity, err.Error())
		return
	}
	code, err := requiredText(payload, "captcha_code", 1, 32)
	if err != nil {
		writeError(c, http.StatusUnprocessableEntity, err.Error())
		return
	}
	actor, err := s.requestActor(c)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "控制台会话读取失败")
		return
	}
	result, err := s.authRecovery.SubmitCaptcha(c.Request.Context(), challengeID, code, actor)
	if err != nil {
		writeError(c, http.StatusUnprocessableEntity, err.Error())
		return
	}
	c.JSON(http.StatusOK, result)
}

func (s *Server) cancelAuthCaptcha(c *gin.Context) {
	if s.authRecovery == nil {
		writeError(c, http.StatusServiceUnavailable, "图片验证码恢复服务尚未就绪")
		return
	}
	payload, err := decodeRequestObject(c)
	if err != nil {
		writeError(c, http.StatusUnprocessableEntity, "验证码取消参数必须是 JSON 对象")
		return
	}
	for key := range payload {
		if key != "challenge_id" {
			writeError(c, http.StatusUnprocessableEntity, "验证码取消参数包含未知字段："+key)
			return
		}
	}
	challengeID, err := requiredText(payload, "challenge_id", 1, 512)
	if err != nil {
		writeError(c, http.StatusUnprocessableEntity, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"cancelled": s.authRecovery.CancelCaptcha(challengeID)})
}

func parseManualAuthInput(payload map[string]any) (authrecovery.ManualInput, error) {
	allowed := map[string]struct{}{
		"host": {}, "auth_mode": {}, "access_token": {}, "refresh_token": {}, "admin_key": {}, "user_id": {},
		"username": {}, "password": {}, "save_to_vault": {}, "accept_login_agreement": {}, "entry": {}, "headers": {},
	}
	for key := range payload {
		if _, present := allowed[key]; !present {
			return authrecovery.ManualInput{}, fmt.Errorf("手动鉴权参数包含未知字段：%s", key)
		}
	}
	host, err := requiredText(payload, "host", 1, 255)
	if err != nil {
		return authrecovery.ManualInput{}, err
	}
	result := authrecovery.ManualInput{Host: host, Present: map[string]bool{}}
	for _, field := range []string{"auth_mode", "access_token", "refresh_token", "admin_key", "user_id", "username", "password", "entry"} {
		raw, present := payload[field]
		if !present {
			continue
		}
		maximum := 65536
		if field == "auth_mode" {
			maximum = 80
		} else if field == "user_id" || field == "entry" {
			maximum = 255
		}
		value, fieldErr := nullableTextField(raw, field, 0, maximum)
		if fieldErr != nil {
			return authrecovery.ManualInput{}, fieldErr
		}
		result.Present[field] = true
		switch field {
		case "auth_mode":
			result.AuthMode = value
		case "access_token":
			result.AccessToken = value
		case "refresh_token":
			result.RefreshToken = value
		case "admin_key":
			result.AdminKey = value
		case "user_id":
			result.UserID = value
		case "username":
			result.Username = value
		case "password":
			result.Password = value
		case "entry":
			result.Entry = value
		}
	}
	if raw, present := payload["save_to_vault"]; present {
		value, ok := raw.(bool)
		if !ok {
			return authrecovery.ManualInput{}, errors.New("save_to_vault 必须是布尔值")
		}
		result.SaveToVault, result.Present["save_to_vault"] = value, true
	}
	if raw, present := payload["accept_login_agreement"]; present {
		value, ok := raw.(bool)
		if !ok {
			return authrecovery.ManualInput{}, errors.New("accept_login_agreement 必须是布尔值")
		}
		result.AcceptLoginAgreement, result.Present["accept_login_agreement"] = value, true
	}
	if raw, present := payload["headers"]; present {
		value, fieldErr := nullableStringMap(raw, "headers")
		if fieldErr != nil {
			return authrecovery.ManualInput{}, fieldErr
		}
		result.Headers, result.Present["headers"] = value, true
	}
	return result, nil
}

func (s *Server) upstreamGroups(c *gin.Context) {
	host := strings.TrimSpace(c.Param("host"))
	if host == "" {
		writeError(c, http.StatusUnprocessableEntity, "上游 Host 不能为空")
		return
	}
	includeBound := true
	if raw, present := c.GetQuery("include_bound"); present {
		parsed, err := strconv.ParseBool(strings.TrimSpace(raw))
		if err != nil {
			writeError(c, http.StatusUnprocessableEntity, "include_bound 必须是布尔值")
			return
		}
		includeBound = parsed
	}
	rows, err := s.business.UpstreamGroups(c.Request.Context(), host, includeBound)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(c, http.StatusNotFound, "上游 Host 不存在")
		return
	}
	if err != nil {
		writeError(c, http.StatusInternalServerError, "上游分组读取失败")
		return
	}
	c.JSON(http.StatusOK, rows)
}

func (s *Server) upstreamGroupHistory(c *gin.Context) {
	host := strings.TrimSpace(c.Param("host"))
	if host == "" {
		writeError(c, http.StatusUnprocessableEntity, "上游 Host 不能为空")
		return
	}
	limit := 200
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 500 {
			writeError(c, http.StatusUnprocessableEntity, "limit 必须在 1 到 500 之间")
			return
		}
		limit = parsed
	}
	rows, err := s.business.UpstreamGroupHistory(c.Request.Context(), host, limit)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(c, http.StatusNotFound, "上游 Host 不存在")
		return
	}
	if err != nil {
		writeError(c, http.StatusInternalServerError, "上游分组变化历史读取失败")
		return
	}
	c.JSON(http.StatusOK, rows)
}

func (s *Server) events(c *gin.Context) {
	limit, ok := optionalLimit(c)
	if !ok {
		return
	}
	rows, err := s.business.Events(c.Request.Context(), limit)
	writeRows(c, rows, err, "运行事件读取失败")
}

func (s *Server) runActiveProbe(c *gin.Context) {
	if s.probeTasks == nil {
		writeError(c, http.StatusServiceUnavailable, "主动探测任务服务尚未就绪")
		return
	}
	accountID, groupName, ok := scopedTaskRequest(c, "主动探测")
	if !ok {
		return
	}
	request := probe.Request{AccountID: accountID, GroupName: groupName}
	actor, err := s.requestActor(c)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "控制台会话读取失败")
		return
	}
	task, err := s.probeTasks.Enqueue(c.Request.Context(), request, actor)
	if err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(http.StatusOK, task)
}

func (s *Server) modelCheckCapabilities(c *gin.Context) {
	if s.modelChecks == nil {
		writeError(c, http.StatusServiceUnavailable, "模型检测服务尚未就绪")
		return
	}
	c.JSON(http.StatusOK, s.modelChecks.Capabilities())
}

func (s *Server) modelCheckAccountStatuses(c *gin.Context) {
	if s.modelChecks == nil {
		writeError(c, http.StatusServiceUnavailable, "模型检测服务尚未就绪")
		return
	}
	statuses, err := s.modelChecks.AccountStatuses(c.Request.Context())
	if err != nil {
		slog.Error("模型检测账号状态读取失败", "error", err)
		writeError(c, http.StatusInternalServerError, "模型检测账号状态读取失败")
		return
	}
	c.JSON(http.StatusOK, statuses)
}

func (s *Server) runModelCheck(c *gin.Context) {
	if s.modelChecks == nil {
		writeError(c, http.StatusServiceUnavailable, "模型检测服务尚未就绪")
		return
	}
	var payload modelCheckRequest
	if err := bindRequestJSON(c, &payload); err != nil {
		writeError(c, http.StatusUnprocessableEntity, "模型检测参数无效")
		return
	}
	task, err := s.modelChecks.Enqueue(c.Request.Context(), modelcheck.Request{
		AccountIDs: payload.AccountIDs, Models: payload.Models,
		Rounds: payload.Rounds, TimeoutSeconds: payload.TimeoutSeconds,
	})
	if err != nil {
		writeError(c, http.StatusUnprocessableEntity, err.Error())
		return
	}
	c.JSON(http.StatusOK, task)
}

func (s *Server) runInspection(c *gin.Context) {
	if s.inspectionTasks == nil {
		writeError(c, http.StatusServiceUnavailable, "巡检任务服务尚未就绪")
		return
	}
	accountID, groupName, ok := scopedTaskRequest(c, "巡检")
	if !ok {
		return
	}
	request := inspection.RunRequest{AccountID: accountID, GroupName: groupName}
	actor, err := s.requestActor(c)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "控制台会话读取失败")
		return
	}
	request.Actor = actor
	task, err := s.inspectionTasks.Enqueue(c.Request.Context(), request)
	if err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(http.StatusOK, task)
}

func scopedTaskRequest(c *gin.Context, operation string) (*string, *string, bool) {
	payload, err := decodeRequestObject(c)
	if err != nil {
		writeError(c, http.StatusUnprocessableEntity, operation+"参数必须是 JSON 对象")
		return nil, nil, false
	}
	var accountID, groupName *string
	for key, raw := range payload {
		switch key {
		case "account_id":
			value, stringValue := raw.(string)
			value = strings.TrimSpace(value)
			if !stringValue || !positiveNumericID(value) || utf8.RuneCountInString(value) > 32 {
				writeError(c, http.StatusUnprocessableEntity, "账号必须使用有效的稳定 ID")
				return nil, nil, false
			}
			accountID = &value
		case "group_name":
			value, stringValue := raw.(string)
			value = strings.TrimSpace(value)
			if !stringValue || value == "" || utf8.RuneCountInString(value) > 120 {
				writeError(c, http.StatusUnprocessableEntity, "已提交的分组不能是空值且长度不能超过 120")
				return nil, nil, false
			}
			groupName = &value
		default:
			writeError(c, http.StatusUnprocessableEntity, operation+"参数包含未知字段："+key)
			return nil, nil, false
		}
	}
	return accountID, groupName, true
}

func (s *Server) autoInspectionStatus(c *gin.Context) {
	if s.inspection == nil {
		writeError(c, http.StatusServiceUnavailable, "自动巡检服务尚未就绪")
		return
	}
	status, err := s.inspection.Status(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(http.StatusOK, status)
}

func (s *Server) updateAutoInspection(c *gin.Context) {
	if s.inspection == nil {
		writeError(c, http.StatusServiceUnavailable, "自动巡检服务尚未就绪")
		return
	}
	var payload autoInspectionRequest
	if err := bindRequestJSON(c, &payload); err != nil || payload.Enabled == nil {
		writeError(c, http.StatusUnprocessableEntity, "自动巡检配置参数无效")
		return
	}
	rateInterval := payload.AccountRateSyncIntervalSeconds
	if rateInterval == 0 {
		rateInterval = business.DefaultAutoInspectionConfig().AccountRateSyncIntervalSeconds
	}
	status, err := s.inspection.UpdateConfig(c.Request.Context(), business.AutoInspectionConfig{
		Enabled: *payload.Enabled, IntervalSeconds: payload.IntervalSeconds,
		AccountRateSyncIntervalSeconds: rateInterval,
		AccountRateSyncBatchSize:       payload.AccountRateSyncBatchSize,
		AccountRateSyncBatchPercent:    payload.AccountRateSyncBatchPercent,
	})
	if err != nil {
		writeError(c, http.StatusUnprocessableEntity, err.Error())
		return
	}
	actor, err := s.requestActor(c)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "控制台会话读取失败")
		return
	}
	state := "关闭"
	if *payload.Enabled {
		state = "开启"
	}
	s.recordRuntimeEventBestEffort(c.Request.Context(), "inspection.automation.updated", "succeeded", "自动巡检已"+state, map[string]any{
		"actor": actor, "enabled": *payload.Enabled, "interval_seconds": payload.IntervalSeconds,
		"account_rate_sync_interval_seconds": rateInterval,
		"account_rate_sync_batch_size":       payload.AccountRateSyncBatchSize,
		"account_rate_sync_batch_percent":    payload.AccountRateSyncBatchPercent,
	})
	c.JSON(http.StatusOK, status)
}

func (s *Server) cancelAutoInspection(c *gin.Context) {
	if s.inspection == nil {
		writeError(c, http.StatusServiceUnavailable, "自动巡检服务尚未就绪")
		return
	}
	status, canceled, err := s.inspection.Cancel(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	actor, actorErr := s.requestActor(c)
	if actorErr != nil {
		slog.Error("自动巡检停止后读取操作人失败", "error", actorErr)
	} else {
		s.recordRuntimeEventBestEffort(c.Request.Context(), "inspection.automation.cancelled", "succeeded", "自动巡检已停止", map[string]any{
			"actor": actor, "active_run_cancelled": canceled,
		})
	}
	c.JSON(http.StatusOK, gin.H{"canceled": canceled, "status": status})
}

func (s *Server) resumeAutoInspection(c *gin.Context) {
	if s.inspection == nil {
		writeError(c, http.StatusServiceUnavailable, "自动巡检服务尚未就绪")
		return
	}
	status, err := s.inspection.Resume(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	actor, actorErr := s.requestActor(c)
	if actorErr != nil {
		slog.Error("自动巡检启动后读取操作人失败", "error", actorErr)
	} else {
		s.recordRuntimeEventBestEffort(c.Request.Context(), "inspection.automation.resumed", "succeeded", "自动巡检已启动", map[string]any{"actor": actor})
	}
	c.JSON(http.StatusOK, status)
}

func (s *Server) clearAutoInspectionHistory(c *gin.Context) {
	if s.inspection == nil {
		writeError(c, http.StatusServiceUnavailable, "自动巡检服务尚未就绪")
		return
	}
	deleted, err := s.inspection.ClearHistory(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": deleted})
}

func (s *Server) autoInspectionEvents(c *gin.Context) {
	if s.inspection == nil {
		writeError(c, http.StatusServiceUnavailable, "自动巡检服务尚未就绪")
		return
	}
	if !s.acquireSSESlot() {
		writeError(c, http.StatusTooManyRequests, "实时连接数量已达上限，请稍后重试")
		return
	}
	defer s.releaseSSESlot()
	updates, unsubscribe := s.inspection.Subscribe()
	defer unsubscribe()
	c.Header("Content-Type", "text/event-stream; charset=utf-8")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)
	controller := http.NewResponseController(c.Writer)
	ping := time.NewTicker(20 * time.Second)
	defer ping.Stop()
	writeStatus := func() error {
		status, err := s.inspection.Status(c.Request.Context())
		if err != nil {
			return err
		}
		_ = controller.SetWriteDeadline(time.Now().Add(30 * time.Second))
		if err := writeSSEEvent(c.Writer, "status", status); err != nil {
			return err
		}
		c.Writer.Flush()
		return nil
	}
	if err := writeStatus(); err != nil {
		slog.Error("自动巡检状态推送失败", "error", err)
		_ = writeSSEError(c.Writer, "自动巡检状态读取失败")
		c.Writer.Flush()
		return
	}
	for {
		select {
		case <-c.Request.Context().Done():
			return
		case <-updates:
			if err := writeStatus(); err != nil {
				return
			}
		case <-ping.C:
			_ = controller.SetWriteDeadline(time.Now().Add(30 * time.Second))
			if _, err := io.WriteString(c.Writer, "event: ping\ndata: {}\n\n"); err != nil {
				return
			}
			c.Writer.Flush()
		}
	}
}

func writeSSEEvent(writer io.Writer, event string, value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(writer, "event: %s\ndata: %s\n\n", event, payload)
	return err
}

func (s *Server) requestTrace(c *gin.Context) {
	if s.traceReader == nil {
		writeError(c, http.StatusServiceUnavailable, "请求追踪服务尚未就绪")
		return
	}
	requestID := strings.TrimSpace(strings.TrimPrefix(c.Param("request_id"), "/"))
	if requestID == "" || utf8.RuneCountInString(requestID) > 512 {
		writeError(c, http.StatusUnprocessableEntity, "request_id 长度必须在 1 到 512 之间")
		return
	}
	trace, err := s.traceReader.RequestTrace(c.Request.Context(), requestID)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			writeError(c, http.StatusGatewayTimeout, "请求追踪超时，请稍后重试或确认上游运维监控可用")
			return
		}
		writeError(c, http.StatusBadGateway, "请求追踪读取失败："+err.Error())
		return
	}
	c.JSON(http.StatusOK, trace)
}

func (s *Server) systemLogs(c *gin.Context) {
	if s.systemLogReader == nil {
		writeError(c, http.StatusServiceUnavailable, "系统日志查询服务尚未就绪")
		return
	}
	timeRange := queryDefault(c, "time_range", "1h")
	allowedRanges := map[string]bool{"5m": true, "30m": true, "1h": true, "6h": true, "24h": true, "7d": true, "30d": true}
	if !allowedRanges[timeRange] {
		writeError(c, http.StatusUnprocessableEntity, "时间范围必须是 5m、30m、1h、6h、24h、7d 或 30d")
		return
	}
	startTime, err := optionalRFC3339Query(c, "start_time")
	if err != nil {
		writeError(c, http.StatusUnprocessableEntity, err.Error())
		return
	}
	endTime, err := optionalRFC3339Query(c, "end_time")
	if err != nil {
		writeError(c, http.StatusUnprocessableEntity, err.Error())
		return
	}
	if startTime != "" && endTime != "" {
		start, _ := time.Parse(time.RFC3339Nano, startTime)
		end, _ := time.Parse(time.RFC3339Nano, endTime)
		if start.After(end) || end.Sub(start) > 30*24*time.Hour {
			writeError(c, http.StatusUnprocessableEntity, "自定义时间范围必须按先后顺序且不超过 30 天")
			return
		}
	}
	for _, name := range []string{"user_id", "api_key_id", "account_id"} {
		if err := validateOptionalPositiveID(c.Query(name), name); err != nil {
			writeError(c, http.StatusUnprocessableEntity, err.Error())
			return
		}
	}
	page, err := positiveQueryInteger(c, "page", 1, 1, 1_000_000)
	if err != nil {
		writeError(c, http.StatusUnprocessableEntity, err.Error())
		return
	}
	pageSize, err := positiveQueryInteger(c, "page_size", 20, 1, 100)
	if err != nil {
		writeError(c, http.StatusUnprocessableEntity, err.Error())
		return
	}
	result, err := s.systemLogReader.SearchSystemLogs(c.Request.Context(), opstraffic.SystemLogQuery{
		TimeRange: timeRange, StartTime: startTime, EndTime: endTime,
		Host: c.Query("host"), Level: c.Query("level"), Component: c.Query("component"),
		RequestID: c.Query("request_id"), ClientRequestID: c.Query("client_request_id"),
		UserID: c.Query("user_id"), APIKeyID: c.Query("api_key_id"), AccountID: c.Query("account_id"),
		Platform: c.Query("platform"), Model: c.Query("model"), Keyword: c.Query("q"),
		Page: page, PageSize: pageSize,
	})
	if errors.Is(err, context.DeadlineExceeded) {
		writeError(c, http.StatusGatewayTimeout, "系统日志查询超时，请缩短时间范围后重试")
		return
	}
	if err != nil {
		writeError(c, http.StatusBadGateway, "系统日志查询失败："+err.Error())
		return
	}
	c.JSON(http.StatusOK, result)
}

func optionalRFC3339Query(c *gin.Context, name string) (string, error) {
	raw := strings.TrimSpace(c.Query(name))
	if raw == "" {
		return "", nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return "", fmt.Errorf("%s 必须是有效时间", name)
	}
	return parsed.UTC().Format(time.RFC3339Nano), nil
}

func validateOptionalPositiveID(raw, name string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	value, err := strconv.ParseUint(raw, 10, 64)
	if err != nil || value == 0 {
		return fmt.Errorf("%s 必须是正整数", name)
	}
	return nil
}

func (s *Server) alerts(c *gin.Context) {
	limit, ok := optionalLimit(c)
	if !ok {
		return
	}
	rows, err := s.business.Alerts(c.Request.Context(), limit)
	writeRows(c, rows, err, "告警列表读取失败")
}

func (s *Server) clearAlerts(c *gin.Context) {
	deleted, err := s.business.ClearAlerts(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusInternalServerError, "告警记录清空失败")
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": deleted})
}

func (s *Server) alertPolicy(c *gin.Context) {
	policy, err := s.business.AlertPolicy(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(http.StatusOK, policy)
}

func (s *Server) updateAlertPolicy(c *gin.Context) {
	payload, err := decodeRequestObject(c)
	if err != nil {
		writeError(c, http.StatusUnprocessableEntity, "告警策略参数必须是 JSON 对象")
		return
	}
	policy, err := s.business.UpdateAlertPolicy(c.Request.Context(), payload)
	if err != nil {
		writeError(c, http.StatusUnprocessableEntity, err.Error())
		return
	}
	c.JSON(http.StatusOK, policy)
}

func (s *Server) evaluateAlerts(c *gin.Context) {
	if s.alertTasks == nil {
		writeError(c, http.StatusServiceUnavailable, "告警检测任务服务尚未就绪")
		return
	}
	task, err := s.alertTasks.Enqueue(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusInternalServerError, "告警检测任务创建失败")
		return
	}
	c.JSON(http.StatusOK, task)
}

func optionalLimit(c *gin.Context) (*int, bool) {
	defaultLimit := 200
	return optionalLimitDefault(c, &defaultLimit)
}

func optionalLimitDefault(c *gin.Context, fallback *int) (*int, bool) {
	raw, present := c.GetQuery("limit")
	if !present || strings.TrimSpace(raw) == "" {
		return fallback, true
	}
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value < 0 || value > 1000 {
		writeError(c, http.StatusUnprocessableEntity, "limit 必须是 0 到 1000 之间的整数")
		return nil, false
	}
	return &value, true
}

func writeRows(c *gin.Context, rows any, err error, detail string) {
	if err != nil {
		writeError(c, http.StatusInternalServerError, detail)
		return
	}
	c.JSON(http.StatusOK, rows)
}

func (s *Server) newAPIWorkspace(c *gin.Context) {
	if s.newAPIManagement == nil {
		writeError(c, http.StatusServiceUnavailable, "New API 管理服务尚未就绪")
		return
	}
	workspace, err := s.newAPIManagement.Workspace(c.Request.Context(), c.Query("platform_id"))
	if err != nil {
		writeError(c, http.StatusInternalServerError, "New API 管理数据读取失败")
		return
	}
	c.JSON(http.StatusOK, workspace)
}

func (s *Server) saveNewAPIPlatform(c *gin.Context) {
	if s.newAPIManagement == nil {
		writeError(c, http.StatusServiceUnavailable, "New API 管理服务尚未就绪")
		return
	}
	var request newAPIPlatformRequest
	if err := bindRequestJSON(c, &request); err != nil {
		writeError(c, http.StatusUnprocessableEntity, "New API 平台配置参数无效")
		return
	}
	result, err := s.newAPIManagement.SavePlatform(c.Request.Context(), newapimanagement.PlatformInput{
		ID: request.ID, Name: request.Name, BaseURL: request.BaseURL, AdminKey: request.AdminKey, UserID: request.UserID,
	})
	if err != nil {
		writeNewAPIError(c, err, http.StatusInternalServerError)
		return
	}
	c.JSON(http.StatusOK, result)
}

func (s *Server) deleteNewAPIPlatform(c *gin.Context) {
	if s.newAPIManagement == nil {
		writeError(c, http.StatusServiceUnavailable, "New API 管理服务尚未就绪")
		return
	}
	platformID := strings.TrimSpace(c.Param("platform_id"))
	if platformID == "" || len(platformID) > 64 {
		writeError(c, http.StatusUnprocessableEntity, "New API 平台 ID 无效")
		return
	}
	deleted, err := s.newAPIManagement.DeletePlatform(c.Request.Context(), platformID)
	if err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	if !deleted {
		writeError(c, http.StatusNotFound, "New API 平台不存在")
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": true})
}

func (s *Server) refreshNewAPIPlatform(c *gin.Context) {
	if s.newAPIManagement == nil {
		writeError(c, http.StatusServiceUnavailable, "New API 管理服务尚未就绪")
		return
	}
	result, err := s.newAPIManagement.Refresh(c.Request.Context(), c.Param("platform_id"))
	if err != nil {
		writeNewAPIError(c, err, http.StatusBadGateway)
		return
	}
	c.JSON(http.StatusOK, result)
}

func (s *Server) managementModelPrices(c *gin.Context) {
	if s.newAPIManagement == nil {
		writeError(c, http.StatusServiceUnavailable, "New API 管理服务尚未就绪")
		return
	}
	models, err := s.newAPIManagement.ManagementModelPrices(c.Request.Context(), c.Param("platform_id"))
	if err != nil {
		writeNewAPIError(c, err, http.StatusBadGateway)
		return
	}
	c.JSON(http.StatusOK, gin.H{"models": models})
}

func (s *Server) remoteModelPricingSource(c *gin.Context) {
	if s.newAPIManagement == nil {
		writeError(c, http.StatusServiceUnavailable, "New API 管理服务尚未就绪")
		return
	}
	source, err := s.newAPIManagement.RemoteModelPricingSource(c.Request.Context(), c.Param("platform_id"))
	if err != nil {
		writeNewAPIError(c, err, http.StatusBadGateway)
		return
	}
	c.JSON(http.StatusOK, source)
}

func (s *Server) saveNewAPIGroupBindings(c *gin.Context) {
	if s.newAPIManagement == nil {
		writeError(c, http.StatusServiceUnavailable, "New API 管理服务尚未就绪")
		return
	}
	var request newAPIGroupBindingsRequest
	if err := bindRequestJSON(c, &request); err != nil {
		writeError(c, http.StatusUnprocessableEntity, "New API 分组绑定参数无效")
		return
	}
	result, err := s.newAPIManagement.SaveBindings(c.Request.Context(), c.Param("platform_id"), request.Bindings)
	if err != nil {
		writeNewAPIError(c, err, http.StatusInternalServerError)
		return
	}
	c.JSON(http.StatusOK, result)
}

func (s *Server) createNewAPIChannel(c *gin.Context) {
	if s.newAPIManagement == nil {
		writeError(c, http.StatusServiceUnavailable, "New API 管理服务尚未就绪")
		return
	}
	var request newAPIChannelRequest
	if err := bindRequestJSON(c, &request); err != nil {
		writeError(c, http.StatusUnprocessableEntity, "New API 渠道参数无效")
		return
	}
	result, err := s.newAPIManagement.CreateChannel(c.Request.Context(), c.Param("platform_id"), newapimanagement.ChannelInput{
		Sub2APIGroupID: request.Sub2APIGroupID, KeyID: request.KeyID, BaseURL: request.BaseURL, Models: request.Models,
		NewAPIGroups: request.NewAPIGroups,
	})
	if err != nil {
		writeNewAPIError(c, err, http.StatusInternalServerError)
		return
	}
	c.JSON(http.StatusCreated, result)
}

func (s *Server) createNewAPIChannelKey(c *gin.Context) {
	if s.newAPIManagement == nil {
		writeError(c, http.StatusServiceUnavailable, "New API 管理服务尚未就绪")
		return
	}
	var request newAPIChannelKeyRequest
	if err := bindRequestJSON(c, &request); err != nil {
		writeError(c, http.StatusUnprocessableEntity, "Sub2API 密钥创建参数无效")
		return
	}
	result, err := s.newAPIManagement.CreateChannelKey(c.Request.Context(), c.Param("platform_id"), newapimanagement.ChannelKeyInput{
		Sub2APIGroupID: request.Sub2APIGroupID, CredentialSource: request.CredentialSource,
		VaultEntry: request.VaultEntry, Username: request.Username, Password: request.Password,
	})
	if err != nil {
		writeNewAPIError(c, err, http.StatusInternalServerError)
		return
	}
	c.JSON(http.StatusCreated, result)
}

func (s *Server) fetchNewAPIChannelModels(c *gin.Context) {
	if s.newAPIManagement == nil {
		writeError(c, http.StatusServiceUnavailable, "New API 管理服务尚未就绪")
		return
	}
	var request newAPIChannelModelsRequest
	if err := bindRequestJSON(c, &request); err != nil {
		writeError(c, http.StatusUnprocessableEntity, "Sub2API 模型获取参数无效")
		return
	}
	models, err := s.newAPIManagement.FetchChannelModels(c.Request.Context(), c.Param("platform_id"), newapimanagement.ChannelModelsInput{
		Sub2APIGroupID: request.Sub2APIGroupID, KeyID: request.KeyID, BaseURL: request.BaseURL,
	})
	if err != nil {
		writeNewAPIError(c, err, http.StatusInternalServerError)
		return
	}
	c.JSON(http.StatusOK, gin.H{"models": models})
}

func (s *Server) saveNewAPIModelPrices(c *gin.Context) {
	if s.newAPIManagement == nil {
		writeError(c, http.StatusServiceUnavailable, "New API 管理服务尚未就绪")
		return
	}
	var request newAPIModelPricesRequest
	if err := bindRequestJSON(c, &request); err != nil {
		writeError(c, http.StatusUnprocessableEntity, "New API 模型价格参数无效")
		return
	}
	result, err := s.newAPIManagement.SaveModelPrices(c.Request.Context(), c.Param("platform_id"), request.Prices)
	if err != nil {
		writeNewAPIError(c, err, http.StatusInternalServerError)
		return
	}
	c.JSON(http.StatusOK, result)
}

func (s *Server) taskDetail(c *gin.Context) {
	if s.tasks == nil {
		writeError(c, http.StatusServiceUnavailable, "任务服务尚未就绪")
		return
	}
	taskID := strings.TrimSpace(c.Param("task_id"))
	if taskID == "" || utf8.RuneCountInString(taskID) > 255 {
		writeError(c, http.StatusUnprocessableEntity, "任务 ID 长度必须在 1 到 255 之间")
		return
	}
	task, err := s.tasks.Get(c.Request.Context(), taskID)
	if errors.Is(err, taskstore.ErrNotFound) {
		writeError(c, http.StatusNotFound, taskstore.ErrNotFound.Error())
		return
	}
	if err != nil {
		writeError(c, http.StatusInternalServerError, "任务状态读取失败")
		return
	}
	c.JSON(http.StatusOK, task)
}

func (s *Server) taskEvents(c *gin.Context) {
	if s.tasks == nil {
		writeError(c, http.StatusServiceUnavailable, "任务服务尚未就绪")
		return
	}
	if !s.acquireSSESlot() {
		writeError(c, http.StatusTooManyRequests, "实时连接数量已达上限，请稍后重试")
		return
	}
	defer s.releaseSSESlot()
	taskID := strings.TrimSpace(c.Param("task_id"))
	if taskID == "" || utf8.RuneCountInString(taskID) > 255 {
		writeError(c, http.StatusUnprocessableEntity, "任务 ID 长度必须在 1 到 255 之间")
		return
	}
	task, err := s.tasks.Get(c.Request.Context(), taskID)
	if errors.Is(err, taskstore.ErrNotFound) {
		writeError(c, http.StatusNotFound, taskstore.ErrNotFound.Error())
		return
	}
	if err != nil {
		writeError(c, http.StatusInternalServerError, "任务状态读取失败")
		return
	}
	c.Header("Content-Type", "text/event-stream; charset=utf-8")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	controller := http.NewResponseController(c.Writer)
	_ = controller.SetWriteDeadline(time.Now().Add(30 * time.Second))
	if err := writeTaskEvent(c.Writer, task); err != nil {
		return
	}
	c.Writer.Flush()
	if terminalTaskStatus(task.Status) {
		return
	}
	// Task state is persisted in SQLite; polling more frequently than once per
	// second only adds database load and does not improve user-visible latency.
	ticker := time.NewTicker(taskSSEPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-c.Request.Context().Done():
			return
		case <-ticker.C:
			task, err = s.tasks.Get(c.Request.Context(), taskID)
			if err != nil {
				slog.Error("任务 SSE 状态读取失败", "task_id", taskID, "error", err)
				_ = writeSSEError(c.Writer, "任务状态读取失败")
				c.Writer.Flush()
				return
			}
			_ = controller.SetWriteDeadline(time.Now().Add(30 * time.Second))
			if err := writeTaskEvent(c.Writer, task); err != nil {
				return
			}
			c.Writer.Flush()
			if terminalTaskStatus(task.Status) {
				return
			}
		}
	}
}

func writeTaskEvent(writer io.Writer, task taskstore.Task) error {
	payload, err := json.Marshal(task)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(writer, "data: %s\n\n", payload)
	return err
}

func writeSSEError(writer io.Writer, detail string) error {
	payload, err := json.Marshal(map[string]string{"detail": detail})
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(writer, "event: error\ndata: %s\n\n", payload)
	return err
}

func terminalTaskStatus(status string) bool {
	switch status {
	case "waiting_input", "succeeded", "partial", "failed", "cancelled":
		return true
	default:
		return false
	}
}

func (s *Server) acquireSSESlot() bool {
	if s.sseSlots == nil {
		return true
	}
	select {
	case s.sseSlots <- struct{}{}:
		return true
	default:
		return false
	}
}

func (s *Server) releaseSSESlot() {
	if s.sseSlots == nil {
		return
	}
	<-s.sseSlots
}

func (s *Server) logsPage(c *gin.Context) {
	if s.logs == nil {
		writeError(c, http.StatusServiceUnavailable, "运行日志服务尚未就绪")
		return
	}
	page, err := positiveQueryInteger(c, "page", 1, 1, 1_000_000)
	if err != nil {
		writeError(c, http.StatusUnprocessableEntity, err.Error())
		return
	}
	pageSize, err := positiveQueryInteger(c, "page_size", 20, 1, 200)
	if err != nil {
		writeError(c, http.StatusUnprocessableEntity, err.Error())
		return
	}
	result, err := s.logs.Query(c.Request.Context(), consolelogs.Query{
		Kind: queryDefault(c, "kind", "all"), State: queryDefault(c, "state", "all"),
		Level: queryDefault(c, "level", "all"), Group: c.Query("group"), GroupID: c.Query("group_id"),
		Search: c.Query("search"), Page: page, PageSize: pageSize,
	})
	if err != nil {
		writeError(c, http.StatusUnprocessableEntity, err.Error())
		return
	}
	c.JSON(http.StatusOK, result)
}

func (s *Server) logCleanupStatus(c *gin.Context) {
	if s.logMaintenance == nil {
		writeError(c, http.StatusServiceUnavailable, "日志清理服务尚未就绪")
		return
	}
	status, err := s.logMaintenance.Status(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(http.StatusOK, status)
}

func (s *Server) updateLogCleanup(c *gin.Context) {
	if s.logMaintenance == nil {
		writeError(c, http.StatusServiceUnavailable, "日志清理服务尚未就绪")
		return
	}
	var payload logCleanupRequest
	if err := bindRequestJSON(c, &payload); err != nil || payload.Enabled == nil {
		writeError(c, http.StatusUnprocessableEntity, "日志清理配置参数无效")
		return
	}
	status, err := s.logMaintenance.Update(c.Request.Context(), *payload.Enabled, payload.RetentionDays)
	if err != nil {
		writeError(c, http.StatusUnprocessableEntity, err.Error())
		return
	}
	c.JSON(http.StatusOK, status)
}

func (s *Server) clearLogs(c *gin.Context) {
	if s.logMaintenance == nil {
		writeError(c, http.StatusServiceUnavailable, "日志清理服务尚未就绪")
		return
	}
	retentionDays, err := positiveQueryInteger(c, "retention_days", 0, 1, 3650)
	if err != nil || retentionDays == 0 {
		writeError(c, http.StatusUnprocessableEntity, "retention_days 必须在 1 到 3650 之间")
		return
	}
	result, err := s.logMaintenance.ClearExpired(c.Request.Context(), retentionDays)
	if err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(http.StatusOK, result)
}

func positiveQueryInteger(c *gin.Context, name string, fallback, minimum, maximum int) (int, error) {
	raw, present := c.GetQuery(name)
	if !present {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || parsed < minimum || parsed > maximum {
		return 0, fmt.Errorf("%s 必须在 %d 到 %d 之间", name, minimum, maximum)
	}
	return parsed, nil
}

func queryDefault(c *gin.Context, name, fallback string) string {
	value := strings.TrimSpace(c.Query(name))
	if value == "" {
		return fallback
	}
	return value
}

func positiveNumericID(value string) bool {
	number, err := strconv.ParseUint(value, 10, 64)
	return err == nil && number > 0 && strconv.FormatUint(number, 10) == value
}

func (s *Server) runtimeConfigFromSnapshot(c *gin.Context, snapshot business.RuntimeSnapshot) {
	ready, err := s.business.Ready(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusInternalServerError, "业务库状态读取失败")
		return
	}
	if !ready {
		writeError(c, http.StatusConflict, "Console 业务库尚未就绪")
		return
	}
	setup, err := s.private.PublicStatus(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusInternalServerError, "控制台配置读取失败")
		return
	}
	privateSettings, err := s.private.RuntimeSettings(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusInternalServerError, "控制台运行设置读取失败")
		return
	}
	configKeys := mergeConfigKeys(snapshot.Keys, privateSettings.Keys)
	errorsFound := append([]string{}, snapshot.ConfigurationErrors...)
	errorsFound = append(errorsFound, setup.ConfigurationErrors...)
	probesEnabled, probeErr := s.business.ProbeEnabled(c.Request.Context())
	if probeErr != nil {
		errorsFound = append(errorsFound, "probe.enabled")
	}
	if privateSettings.RequestTimeoutConfigurationError != "" {
		errorsFound = append(errorsFound, privateSettings.RequestTimeoutConfigurationError)
	}
	c.JSON(http.StatusOK, runtimeConfigResponse{
		DatabasePath:              s.config.DataDB,
		DataDatabasePath:          s.config.DataDB,
		DatabaseAvailable:         snapshot.Available,
		DataDatabaseAvailable:     true,
		Mode:                      snapshot.Mode,
		ConfigKeys:                configKeys,
		SecretValuesHidden:        true,
		ProbesEnabled:             probesEnabled,
		AdminBaseURL:              privateSettings.AdminBaseURL,
		RequestTimeoutSeconds:     privateSettings.RequestTimeoutSeconds,
		AccountDefaultConcurrency: privateSettings.AccountDefaultConcurrency,
		AccountDefaultPriority:    privateSettings.AccountDefaultPriority,
		Initialized:               setup.Initialized,
		TargetConfigured:          setup.TargetConfigured,
		ConsoleUsername:           configstore.MaskUsername(setup.Username),
		ConfigurationErrors:       uniqueStrings(errorsFound),
	})
}

func mergeConfigKeys(raw any, privateKeys []string) any {
	items, ok := raw.([]any)
	if !ok {
		if stringsList, stringsOK := raw.([]string); stringsOK {
			result := append([]string{}, stringsList...)
			for _, key := range privateKeys {
				result = append(result, "console/"+key)
			}
			sort.Strings(result)
			return uniqueStrings(result)
		}
		return raw
	}
	result := make([]string, 0, len(items)+len(privateKeys))
	for _, item := range items {
		result = append(result, fmt.Sprint(item))
	}
	for _, key := range privateKeys {
		result = append(result, "console/"+key)
	}
	sort.Strings(result)
	return uniqueStrings(result)
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if _, found := seen[value]; found {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func (s *Server) authorize() gin.HandlerFunc {
	return func(c *gin.Context) {
		if s.validAdminBearer(c.Request) {
			c.Next()
			return
		}
		initialized, err := s.private.IsInitialized(c.Request.Context())
		if err != nil {
			writeError(c, http.StatusInternalServerError, "控制台配置读取失败")
			c.Abort()
			return
		}
		if !initialized {
			if s.config.AdminToken != "" {
				writeError(c, http.StatusUnauthorized, "控制台认证失败")
			} else {
				writeError(c, http.StatusPreconditionRequired, "请先完成首次初始化")
			}
			c.Abort()
			return
		}
		username, err := s.sessionUser(c)
		if err != nil {
			writeError(c, http.StatusInternalServerError, "控制台会话读取失败")
			c.Abort()
			return
		}
		if username == nil {
			writeError(c, http.StatusUnauthorized, "请先登录控制台")
			c.Abort()
			return
		}
		c.Set("session_user", *username)
		c.Next()
	}
}

func (s *Server) validAdminBearer(request *http.Request) bool {
	return constantTimeTokenEqual("Bearer "+s.config.AdminToken, request.Header.Get("Authorization"), s.config.AdminToken != "")
}

func (s *Server) validSetupCredential(request *http.Request) bool {
	return s.validAdminBearer(request) || constantTimeTokenEqual(s.config.SetupToken, request.Header.Get("X-Setup-Token"), s.config.SetupToken != "")
}

func constantTimeTokenEqual(expected, supplied string, configured bool) bool {
	return configured && len(expected) == len(supplied) && subtle.ConstantTimeCompare([]byte(expected), []byte(supplied)) == 1
}

func (s *Server) setupTokenRequired(request *http.Request) bool {
	if s.config.SetupToken != "" {
		return true
	}
	peer, ok := requestPeerAddress(request)
	if !ok || !peer.IsLoopback() || s.loginThrottle.fromTrustedProxy(request) {
		return true
	}
	requestOrigin, ok := s.requestOrigin(request)
	if !ok {
		return true
	}
	parsedRequestOrigin, err := url.Parse(requestOrigin)
	if err != nil || !localBrowserHostname(parsedRequestOrigin.Hostname()) {
		return true
	}
	for _, rawURL := range []string{request.Header.Get("Origin"), request.Header.Get("Referer")} {
		rawURL = strings.TrimSpace(rawURL)
		if rawURL == "" {
			continue
		}
		origin, valid := normalizedURLOrigin(rawURL, false)
		if !valid {
			return true
		}
		parsed, err := url.Parse(origin)
		if err != nil || !localBrowserHostname(parsed.Hostname()) {
			return true
		}
	}
	return false
}

func localBrowserHostname(hostname string) bool {
	hostname = strings.TrimSpace(strings.ToLower(hostname))
	if hostname == "localhost" {
		return true
	}
	address, err := netip.ParseAddr(hostname)
	return err == nil && address.Unmap().IsLoopback()
}

func (s *Server) setSession(c *gin.Context, username string) error {
	token, err := s.private.CreateSession(c.Request.Context(), username, sessionTTL, s.now())
	if err != nil {
		return err
	}
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		MaxAge:   int(sessionTTL.Seconds()),
		HttpOnly: true,
		Secure:   s.config.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
	return nil
}

func (s *Server) sessionUser(c *gin.Context) (*string, error) {
	token, err := c.Cookie(sessionCookie)
	if err != nil && !errors.Is(err, http.ErrNoCookie) {
		return nil, err
	}
	return s.private.SessionUser(c.Request.Context(), token, s.now())
}

func (s *Server) recordRuntimeEventBestEffort(ctx context.Context, eventType, status, summary string, payload map[string]any) {
	if _, err := s.business.RecordRuntimeEvent(ctx, eventType, status, summary, payload); err != nil {
		slog.Error("运行事件保存失败", "event_type", eventType, "status", status, "error", err)
	}
}

func (s *Server) cors() gin.HandlerFunc {
	allowed := make(map[string]struct{}, len(s.config.Origins))
	for _, rawOrigin := range s.config.Origins {
		if origin, ok := normalizedURLOrigin(rawOrigin, true); ok {
			allowed[origin] = struct{}{}
		}
	}
	return func(c *gin.Context) {
		rawOrigin := strings.TrimSpace(c.GetHeader("Origin"))
		origin, validOrigin := normalizedURLOrigin(rawOrigin, true)
		originAllowed := rawOrigin != "" && validOrigin && s.originAllowed(c.Request, origin, allowed)
		if originAllowed {
			c.Header("Access-Control-Allow-Origin", rawOrigin)
			c.Header("Access-Control-Allow-Credentials", "true")
			c.Header("Vary", "Origin")
		}
		if c.Request.Method == http.MethodOptions {
			if rawOrigin != "" && !originAllowed {
				writeError(c, http.StatusForbidden, "请求来源不可信")
				c.Abort()
				return
			}
			c.Header("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
			c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Setup-Token")
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		if requestChangesState(c.Request.Method) && !s.validAdminBearer(c.Request) {
			trusted := originAllowed
			if rawOrigin == "" {
				if referer := strings.TrimSpace(c.GetHeader("Referer")); referer != "" {
					refererOrigin, ok := normalizedURLOrigin(referer, false)
					trusted = ok && s.originAllowed(c.Request, refererOrigin, allowed)
				} else if _, err := c.Request.Cookie(sessionCookie); err == nil {
					trusted = false
				} else {
					trusted = true
				}
			}
			if !trusted {
				writeError(c, http.StatusForbidden, "请求来源不可信")
				c.Abort()
				return
			}
		}
		c.Next()
	}
}

func requestChangesState(method string) bool {
	return method != http.MethodGet && method != http.MethodHead && method != http.MethodOptions
}

func (s *Server) originAllowed(request *http.Request, origin string, configured map[string]struct{}) bool {
	if _, ok := configured[origin]; ok {
		return true
	}
	requestOrigin, ok := s.requestOrigin(request)
	return ok && origin == requestOrigin
}

func (s *Server) requestOrigin(request *http.Request) (string, bool) {
	scheme := "http"
	if request.TLS != nil {
		scheme = "https"
	}
	if s.loginThrottle.fromTrustedProxy(request) {
		forwarded := strings.TrimSpace(strings.Split(request.Header.Get("X-Forwarded-Proto"), ",")[0])
		if forwarded == "http" || forwarded == "https" {
			scheme = forwarded
		}
	}
	return normalizedURLOrigin(scheme+"://"+request.Host, true)
}

func normalizedURLOrigin(raw string, originOnly bool) (string, bool) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil {
		return "", false
	}
	if originOnly && ((parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.Fragment != "") {
		return "", false
	}
	hostname := strings.ToLower(parsed.Hostname())
	if hostname == "" {
		return "", false
	}
	port := parsed.Port()
	if (parsed.Scheme == "http" && port == "80") || (parsed.Scheme == "https" && port == "443") {
		port = ""
	}
	host := hostname
	if port != "" {
		host = net.JoinHostPort(hostname, port)
	} else if strings.Contains(hostname, ":") {
		host = "[" + hostname + "]"
	}
	return parsed.Scheme + "://" + host, true
}

func writeNewAPIError(c *gin.Context, err error, fallbackStatus int) {
	status := fallbackStatus
	if errors.Is(err, context.DeadlineExceeded) {
		status = http.StatusGatewayTimeout
	} else {
		var networkError net.Error
		if errors.As(err, &networkError) && networkError.Timeout() {
			status = http.StatusGatewayTimeout
		} else {
			switch newapimanagement.KindOf(err) {
			case newapimanagement.ErrorValidation:
				status = http.StatusUnprocessableEntity
			case newapimanagement.ErrorNotFound:
				status = http.StatusNotFound
			case newapimanagement.ErrorConflict:
				status = http.StatusConflict
			case newapimanagement.ErrorUnavailable:
				status = http.StatusServiceUnavailable
			case newapimanagement.ErrorUpstream:
				status = http.StatusBadGateway
			}
		}
	}
	writeError(c, status, err.Error())
}

func writeError(c *gin.Context, status int, detail string) {
	if status >= http.StatusInternalServerError {
		slog.Error("API 请求处理失败", "method", c.Request.Method, "path", c.FullPath(), "status", status, "error", redact.Secrets(detail))
		switch status {
		case http.StatusBadGateway:
			detail = "上游服务请求失败，请检查连接配置后重试"
		case http.StatusServiceUnavailable:
			detail = "服务暂不可用，请稍后重试"
		case http.StatusGatewayTimeout:
			detail = "上游服务请求超时，请稍后重试"
		default:
			detail = "服务处理失败，请稍后重试"
		}
	}
	code := "internal_error"
	switch {
	case status == http.StatusBadRequest:
		code = "bad_request"
	case status == http.StatusUnauthorized:
		code = "unauthorized"
	case status == http.StatusForbidden:
		code = "forbidden"
	case status == http.StatusNotFound:
		code = "not_found"
	case status == http.StatusConflict:
		code = "conflict"
	case status == http.StatusUnprocessableEntity:
		code = "validation_error"
	case status == http.StatusTooManyRequests:
		code = "rate_limited"
	case status == http.StatusBadGateway:
		code = "upstream_error"
	case status == http.StatusServiceUnavailable:
		code = "service_unavailable"
	case status == http.StatusGatewayTimeout:
		code = "upstream_timeout"
	case status >= 500:
		code = "internal_error"
	}
	c.JSON(status, gin.H{"code": code, "detail": detail})
}
