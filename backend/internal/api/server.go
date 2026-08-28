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
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountops"
	"github.com/MIEnchating/sub2api-console/backend/internal/authrecovery"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/config"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/inspection"
	consolelogs "github.com/MIEnchating/sub2api-console/backend/internal/logs"
	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
	"github.com/MIEnchating/sub2api-console/backend/internal/notification"
	"github.com/MIEnchating/sub2api-console/backend/internal/onboarding"
	"github.com/MIEnchating/sub2api-console/backend/internal/probe"
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

var configureJSONBinding sync.Once

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
	Groups(context.Context) ([]business.GroupStatus, error)
	GroupAllocation(context.Context, string) (business.GroupAllocation, error)
	ControlPolicy(context.Context) (map[string]any, error)
	PolicySnapshot(context.Context) (business.PolicySnapshot, error)
	UpdatePolicy(context.Context, map[string]any, string) (business.PolicySnapshot, error)
	SetAccountControl(context.Context, string, string, string) (business.PolicySnapshot, error)
	SetAccountTestModel(context.Context, string, *string, string) error
	UpdateGroupPolicy(context.Context, string, map[string]any, string) (business.GroupStatus, error)
	ClearGroupPolicy(context.Context, string, string) (business.GroupStatus, error)
	SetGroupExcluded(context.Context, string, bool, string) (business.GroupStatus, error)
	Upstreams(context.Context) (business.UpstreamSummary, error)
	UpstreamGroups(context.Context, string, bool) ([]business.UpstreamGroup, error)
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
	EnqueueAccountRevalidation(context.Context, []string, string) (taskstore.Task, error)
	EnqueueAccountNameRepair(context.Context, []string, string) (taskstore.Task, error)
	EnqueueMissingBindingCleanup(context.Context, []string, string) (taskstore.Task, error)
}

type AccountTaskEnqueuer interface {
	EnqueueFields(context.Context, string, accountops.FieldPatch, string) (taskstore.Task, error)
	Models(context.Context, string) ([]string, error)
}

type ProbeTaskEnqueuer interface {
	Enqueue(context.Context, probe.Request, string) (taskstore.Task, error)
}

type ModelCheckService interface {
	Capabilities() modelcheck.Capabilities
	Enqueue(context.Context, modelcheck.Request) (taskstore.Task, error)
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
	Enqueue(context.Context, onboarding.Request) (taskstore.Task, error)
	EnqueueBatch(context.Context, []onboarding.Request) (taskstore.Task, error)
}

type UpstreamDeleteService interface {
	Preview(context.Context, string) (business.UpstreamDeletePreview, error)
	Enqueue(context.Context, string, []string, string) (taskstore.Task, error)
}

type AuthRecoveryService interface {
	VerifyManual(context.Context, authrecovery.ManualInput, string) (authrecovery.ManualResult, error)
	Enqueue(context.Context, string, string, string) (taskstore.Task, error)
	SubmitCaptcha(context.Context, string, string, string) (authrecovery.CaptchaCompletion, error)
	CancelCaptcha(string) bool
}

type Dependencies struct {
	Notification       NotificationTester
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
	ProbeTasks         ProbeTaskEnqueuer
	ModelChecks        ModelCheckService
	UpstreamDetect     UpstreamDetector
	UpstreamConfigs    UpstreamConfigurationService
	UpstreamSync       UpstreamSyncTaskEnqueuer
	UpstreamDelete     UpstreamDeleteService
	AuthRecovery       AuthRecoveryService
	Onboarding         OnboardingService
	RequestTrace       RequestTraceReader
}

type Server struct {
	config             config.Config
	private            *configstore.Store
	business           Business
	notifier           NotificationTester
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
	probeTasks         ProbeTaskEnqueuer
	modelChecks        ModelCheckService
	upstreamDetect     UpstreamDetector
	upstreamConfigs    UpstreamConfigurationService
	upstreamSync       UpstreamSyncTaskEnqueuer
	upstreamDelete     UpstreamDeleteService
	authRecovery       AuthRecoveryService
	onboarding         OnboardingService
	traceReader        RequestTraceReader
	loginThrottle      *loginThrottle
	now                func() time.Time
}

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

type notificationTestRequest struct {
	Message string `json:"message" binding:"required,min=1,max=4000"`
	DryRun  *bool  `json:"dry_run" binding:"required"`
}

type autoInspectionRequest struct {
	Enabled         *bool `json:"enabled" binding:"required"`
	IntervalSeconds int   `json:"interval_seconds" binding:"required,min=15,max=86400"`
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
	DatabasePath          string   `json:"database_path"`
	DataDatabasePath      string   `json:"data_database_path"`
	DatabaseAvailable     bool     `json:"database_available"`
	DataDatabaseAvailable bool     `json:"data_database_available"`
	Mode                  string   `json:"mode"`
	ConfigKeys            any      `json:"config_keys"`
	SecretValuesHidden    bool     `json:"secret_values_hidden"`
	ProbesEnabled         bool     `json:"probes_enabled"`
	AdminBaseURL          *string  `json:"admin_base_url"`
	RequestTimeoutSeconds int      `json:"request_timeout_seconds"`
	Initialized           bool     `json:"initialized"`
	TargetConfigured      bool     `json:"target_configured"`
	ConsoleUsername       *string  `json:"console_username"`
	ConfigurationErrors   []string `json:"configuration_errors"`
}

func New(cfg config.Config, private *configstore.Store, business Business, dependencies ...Dependencies) *gin.Engine {
	configureJSONBinding.Do(gin.EnableJsonDecoderDisallowUnknownFields)
	var services Dependencies
	if len(dependencies) > 0 {
		services = dependencies[0]
	}
	server := &Server{
		config:             cfg,
		private:            private,
		business:           business,
		notifier:           services.Notification,
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
		probeTasks:         services.ProbeTasks,
		modelChecks:        services.ModelChecks,
		upstreamDetect:     services.UpstreamDetect,
		upstreamConfigs:    services.UpstreamConfigs,
		upstreamSync:       services.UpstreamSync,
		upstreamDelete:     services.UpstreamDelete,
		authRecovery:       services.AuthRecovery,
		onboarding:         services.Onboarding,
		traceReader:        services.RequestTrace,
		loginThrottle:      newLoginThrottle(),
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
	authorized.POST("/config/target", server.updateAdminTarget)
	authorized.GET("/notifications/status", server.notificationStatus)
	authorized.GET("/notifications/queue", server.notificationQueue)
	authorized.POST("/notifications/config", server.configureNotification)
	authorized.POST("/notifications/test", server.testNotification)
	authorized.GET("/accounts", server.accounts)
	authorized.GET("/accounts/:account_id", server.account)
	authorized.POST("/accounts/:account_id/control", server.setAccountControl)
	authorized.GET("/accounts/:account_id/models", server.accountModels)
	authorized.PUT("/accounts/:account_id/test-model", server.setAccountTestModel)
	authorized.POST("/accounts/:account_id/sync", server.syncAccountFields)
	authorized.POST("/management/sync", server.syncManagement)
	authorized.POST("/management/accounts/revalidate", server.revalidateAccounts)
	authorized.POST("/management/accounts/names/repair", server.repairAccountNames)
	authorized.POST("/management/accounts/missing-bindings/cleanup", server.cleanupMissingBindings)
	authorized.POST("/onboarding", server.createOnboarding)
	authorized.POST("/onboarding/batch", server.createOnboardingBatch)
	authorized.POST("/onboarding/prepare", server.prepareOnboarding)
	authorized.POST("/onboarding/probe/models", server.onboardingProbeModels)
	authorized.POST("/onboarding/probe", server.onboardingProbe)
	authorized.GET("/groups", server.groups)
	authorized.GET("/groups/:group_id/allocation", server.groupAllocation)
	authorized.PUT("/groups/:group_id/policy", server.updateGroupPolicy)
	authorized.DELETE("/groups/:group_id/policy", server.clearGroupPolicy)
	authorized.PUT("/groups/:group_id/excluded", server.setGroupExcluded)
	authorized.GET("/policy", server.policy)
	authorized.PUT("/policy", server.updatePolicy)
	authorized.POST("/policy/restore-control", server.restoreRoutingControl)
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
	authorized.GET("/upstreams/:host/delete-preview", server.upstreamDeletePreview)
	authorized.POST("/upstreams/:host/delete", server.deleteUpstream)
	authorized.GET("/auth-recovery/config", server.authRecoveryConfiguration)
	authorized.POST("/auth-recovery/vault-entry", server.configureVaultEntry)
	authorized.DELETE("/auth-recovery/vault-entry", server.deleteVaultEntry)
	authorized.POST("/auth-recovery/manual", server.verifyManualAuth)
	authorized.POST("/auth-recovery/run", server.runAuthRecovery)
	authorized.POST("/auth-recovery/captcha/submit", server.submitAuthCaptcha)
	authorized.POST("/auth-recovery/captcha/cancel", server.cancelAuthCaptcha)
	authorized.GET("/events", server.events)
	authorized.POST("/inspection/run", server.runInspection)
	authorized.POST("/inspection/probe", server.runActiveProbe)
	authorized.GET("/model-checks/capabilities", server.modelCheckCapabilities)
	authorized.POST("/model-checks", server.runModelCheck)
	authorized.GET("/inspection/automation", server.autoInspectionStatus)
	authorized.PUT("/inspection/automation", server.updateAutoInspection)
	authorized.GET("/inspection/automation/events", server.autoInspectionEvents)
	authorized.POST("/inspection/automation/cancel", server.cancelAutoInspection)
	authorized.POST("/inspection/automation/resume", server.resumeAutoInspection)
	authorized.DELETE("/inspection/automation/history", server.clearAutoInspectionHistory)
	authorized.GET("/usage/trace/*request_id", server.requestTrace)
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
		ConfigurationErrors: status.ConfigurationErrors,
	})
}

func (s *Server) initialize(c *gin.Context) {
	var payload initializeRequest
	if err := c.ShouldBindJSON(&payload); err != nil {
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
	if err := c.ShouldBindJSON(&payload); err != nil {
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
	if err := c.ShouldBindJSON(&payload); err != nil {
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
	if err := c.ShouldBindJSON(&payload); err != nil {
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
	if err := c.ShouldBindJSON(&payload); err != nil {
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
	if err := s.private.ConfigureTarget(
		c.Request.Context(),
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
	if err := c.ShouldBindJSON(&payload); err != nil || payload.Enabled == nil {
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
	if err := c.ShouldBindJSON(&payload); err != nil {
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
	if err := c.ShouldBindJSON(&payload); err != nil || payload.DryRun == nil {
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
	if s.inspectionTasks == nil {
		writeError(c, http.StatusServiceUnavailable, "巡检任务服务尚未就绪")
		return
	}
	if _, ok := s.accountMutationPreflight(c, accountID, true); !ok {
		return
	}
	actor, err := s.requestActor(c)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "控制台会话读取失败")
		return
	}
	if _, err := s.business.SetAccountControl(c.Request.Context(), accountID, action, actor); err != nil {
		if strings.Contains(err.Error(), "不存在") {
			writeError(c, http.StatusNotFound, err.Error())
			return
		}
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	task, err := s.inspectionTasks.Enqueue(c.Request.Context(), inspection.RunRequest{
		AccountID: &accountID,
		Actor:     actor,
	})
	if err != nil {
		writeError(c, http.StatusConflict, "账号控制已保存，但巡检任务创建失败："+err.Error())
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
	if mode == runtimepolicy.Monitoring && (!patch.MultiplierPresent || patch.NamePresent || patch.PriorityPresent || patch.LoadFactorPresent || patch.ConcurrencyPresent || patch.NotesPresent) {
		writeError(c, http.StatusConflict, "监控模式只允许同步账号倍率")
		return
	}
	if mode != runtimepolicy.Monitoring && mode != runtimepolicy.Full {
		writeError(c, http.StatusConflict, "账号字段同步需要监控模式或完全模式")
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

func accountFieldPatch(payload map[string]any) (accountops.FieldPatch, error) {
	allowed := map[string]struct{}{
		"name": {}, "priority": {}, "load_factor": {}, "concurrency": {}, "multiplier": {}, "notes": {},
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
	if raw, present := payload["multiplier"]; present {
		value, err := nullableTextField(raw, "multiplier", 1, 128)
		if err != nil || value == nil {
			return accountops.FieldPatch{}, errors.New("multiplier 必须是长度在 1 到 128 之间的字符串")
		}
		result.MultiplierPresent, result.Multiplier = true, value
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

func (s *Server) repairAccountNames(c *gin.Context) {
	s.enqueueAccountMaintenance(c, "names")
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
	case "names":
		task, err = s.accountMaintenance.EnqueueAccountNameRepair(c.Request.Context(), accountIDs, actor)
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
		"host": {}, "upstream_type": {}, "platform": {}, "account_type": {}, "notes": {}, "multiplier": {},
		"local_group_id": {}, "upstream_group_id": {}, "extra": {}, "priority": {}, "schedulable": {},
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
	multiplier, err := requiredText(payload, "multiplier", 1, 128)
	if err != nil {
		return onboarding.Request{}, err
	}
	upstreamGroupID, err := requiredText(payload, "upstream_group_id", 1, 255)
	if err != nil {
		return onboarding.Request{}, err
	}
	rawLocalID, present := payload["local_group_id"]
	localID := ""
	if number, ok := rawLocalID.(json.Number); present && ok {
		localID = number.String()
	}
	if !positiveNumericID(localID) {
		return onboarding.Request{}, errors.New("local_group_id 必须是稳定正整数")
	}
	result := onboarding.Request{
		Host: host, UpstreamType: strings.ToLower(upstreamType), Multiplier: multiplier,
		LocalGroupID: localID, UpstreamGroupID: upstreamGroupID, Extra: map[string]any{},
	}
	for field, target := range map[string]**string{"platform": &result.Platform, "account_type": &result.AccountType, "notes": &result.Notes} {
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
		if err != nil {
			return onboarding.Request{}, errors.New("priority 必须是整数")
		}
		result.Priority = &value
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

func decodeRequestObject(c *gin.Context) (map[string]any, error) {
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
		"host": {}, "name": {}, "base_url": {}, "upstream_type": {}, "auth_mode": {}, "recharge_rate": {},
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
		BaseURL: baseURL, UpstreamType: platform, AuthMode: authMode, RechargeRate: recharge,
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
	if err := s.private.SaveVaultEntry(c.Request.Context(), entry, present); err != nil {
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
	deleted, err := s.private.DeleteVaultEntry(c.Request.Context(), entry)
	if err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"entry": entry, "deleted": deleted})
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
		if key != "host" && key != "entry" {
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
	actor, err := s.requestActor(c)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "控制台会话读取失败")
		return
	}
	task, err := s.authRecovery.Enqueue(c.Request.Context(), host, entry, actor)
	if err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(http.StatusOK, task)
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
		"username": {}, "password": {}, "save_to_vault": {}, "entry": {}, "headers": {},
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

func (s *Server) runModelCheck(c *gin.Context) {
	if s.modelChecks == nil {
		writeError(c, http.StatusServiceUnavailable, "模型检测服务尚未就绪")
		return
	}
	var payload modelCheckRequest
	if err := c.ShouldBindJSON(&payload); err != nil {
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
	if err := c.ShouldBindJSON(&payload); err != nil || payload.Enabled == nil {
		writeError(c, http.StatusUnprocessableEntity, "自动巡检配置参数无效")
		return
	}
	status, err := s.inspection.UpdateConfig(c.Request.Context(), business.AutoInspectionConfig{
		Enabled: *payload.Enabled, IntervalSeconds: payload.IntervalSeconds,
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
		_ = writeSSEError(c.Writer, err.Error())
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
	return optionalLimitDefault(c, nil)
}

func optionalLimitDefault(c *gin.Context, fallback *int) (*int, bool) {
	raw, present := c.GetQuery("limit")
	if !present || strings.TrimSpace(raw) == "" {
		return fallback, true
	}
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value < 0 || value > 100000 {
		writeError(c, http.StatusUnprocessableEntity, "limit 必须是 0 到 100000 之间的整数")
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
	if err := writeTaskEvent(c.Writer, task); err != nil {
		return
	}
	c.Writer.Flush()
	if terminalTaskStatus(task.Status) {
		return
	}
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-c.Request.Context().Done():
			return
		case <-ticker.C:
			task, err = s.tasks.Get(c.Request.Context(), taskID)
			if err != nil {
				_ = writeSSEError(c.Writer, "任务状态读取失败："+err.Error())
				c.Writer.Flush()
				return
			}
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
	case "waiting_input", "succeeded", "failed", "cancelled":
		return true
	default:
		return false
	}
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
	if err := c.ShouldBindJSON(&payload); err != nil || payload.Enabled == nil {
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
		DatabasePath:          s.config.DataDB,
		DataDatabasePath:      s.config.DataDB,
		DatabaseAvailable:     snapshot.Available,
		DataDatabaseAvailable: true,
		Mode:                  snapshot.Mode,
		ConfigKeys:            configKeys,
		SecretValuesHidden:    true,
		ProbesEnabled:         probesEnabled,
		AdminBaseURL:          privateSettings.AdminBaseURL,
		RequestTimeoutSeconds: privateSettings.RequestTimeoutSeconds,
		Initialized:           setup.Initialized,
		TargetConfigured:      setup.TargetConfigured,
		ConsoleUsername:       configstore.MaskUsername(setup.Username),
		ConfigurationErrors:   uniqueStrings(errorsFound),
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
		if s.config.AdminToken != "" {
			expected := "Bearer " + s.config.AdminToken
			supplied := c.GetHeader("Authorization")
			if len(expected) != len(supplied) || subtle.ConstantTimeCompare([]byte(expected), []byte(supplied)) != 1 {
				writeError(c, http.StatusUnauthorized, "控制台认证失败")
				c.Abort()
				return
			}
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
			writeError(c, http.StatusPreconditionRequired, "请先完成首次初始化")
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
	for _, origin := range s.config.Origins {
		allowed[origin] = struct{}{}
	}
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if _, found := allowed[origin]; found {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Access-Control-Allow-Credentials", "true")
			c.Header("Vary", "Origin")
		}
		if c.Request.Method == http.MethodOptions {
			c.Header("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
			c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

func writeError(c *gin.Context, status int, detail string) {
	c.JSON(status, gin.H{"detail": detail})
}
