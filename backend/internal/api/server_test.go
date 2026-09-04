package api

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

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
	"github.com/MIEnchating/sub2api-console/backend/internal/notification"
	"github.com/MIEnchating/sub2api-console/backend/internal/notificationtarget"
	"github.com/MIEnchating/sub2api-console/backend/internal/onboarding"
	"github.com/MIEnchating/sub2api-console/backend/internal/opstraffic"
	"github.com/MIEnchating/sub2api-console/backend/internal/pricing"
	"github.com/MIEnchating/sub2api-console/backend/internal/probe"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/upstreamconfig"
	"github.com/MIEnchating/sub2api-console/backend/internal/upstreamdetect"
	"github.com/MIEnchating/sub2api-console/backend/internal/upstreamsync"
)

type fakeBusiness struct {
	bootstrapError       error
	mode                 string
	notificationChannels *[]string
	probeEnabled         *bool
	accountRows          []business.AccountStatus
	accountDetail        *business.AccountDetail
	groupRows            []business.GroupStatus
	groupAllocation      business.GroupAllocation
	groupProbeModels     business.GroupProbeModels
	policySnapshot       business.PolicySnapshot
	policyUpdates        *[]map[string]any
	policyActors         *[]string
	groupPolicyUpdates   *[]map[string]any
	groupExcludedUpdates *[]bool
	upstreamSummary      business.UpstreamSummary
	upstreamGroupRows    []business.UpstreamGroup
	upstreamGroupHistory []business.UpstreamGroupChange
	runtimeEventIDs      *[]int64
	runtimeEventError    error
	alertPolicy          business.AlertPolicy
	alertRows            []business.AlertListItem
	clearedAlerts        int64
	trafficRanking       business.TrafficRanking
	trafficQueries       *[]business.TrafficRankingQuery
	trafficError         error
}

type fakePricingService struct {
	snapshot        pricing.Snapshot
	changes         []business.PricingChangeRecord
	updated         pricing.Config
	revenue         pricing.RevenueRequest
	enqueued        int
	backups         []business.PricingBackup
	deletedBackupID string
	deleteBackupErr error
}

func (service *fakePricingService) Snapshot(context.Context) (pricing.Snapshot, error) {
	return service.snapshot, nil
}

func (service *fakePricingService) Changes(context.Context) ([]business.PricingChangeRecord, error) {
	return service.changes, nil
}

func (service *fakePricingService) UpdateConfig(_ context.Context, config pricing.Config, _ string) (pricing.Snapshot, error) {
	service.updated = config
	service.snapshot.Config = config
	return service.snapshot, nil
}

func (service *fakePricingService) Enqueue(context.Context, string) (taskstore.Task, error) {
	service.enqueued++
	return taskstore.Task{ID: "pricing-task", Operation: "price-group-allocation", Status: "queued"}, nil
}

func (service *fakePricingService) EnqueueRevenue(_ context.Context, request pricing.RevenueRequest, _ string) (taskstore.Task, error) {
	service.enqueued++
	service.revenue = request
	return taskstore.Task{ID: "revenue-task", Operation: "revenue-calculation", Status: "queued"}, nil
}

func (service *fakePricingService) CreateBackup(_ context.Context, name, actor string) (business.PricingBackup, error) {
	backup := business.PricingBackup{ID: "backup-1", Name: name, Actor: actor, AccountCount: 2}
	service.backups = append(service.backups, backup)
	return backup, nil
}

func (service *fakePricingService) Backups(context.Context) ([]business.PricingBackup, error) {
	return service.backups, nil
}

func (service *fakePricingService) DeleteBackup(_ context.Context, backupID string) error {
	service.deletedBackupID = backupID
	return service.deleteBackupErr
}

func (service *fakePricingService) EnqueueRestore(_ context.Context, backupID, _ string) (taskstore.Task, error) {
	service.enqueued++
	return taskstore.Task{ID: "restore-task", Operation: "price-group-restore", Status: "queued", Result: map[string]any{"backup_id": backupID}}, nil
}

type fakeQueueBusiness struct {
	fakeBusiness
	details business.NotificationQueueDetails
	keys    *[]string
}

type vaultLeaseBusiness struct {
	fakeBusiness
	lease    chan struct{}
	attempts chan []string
}

func (business *vaultLeaseBusiness) AcquireMutationLease(
	ctx context.Context,
	_ string,
	resources []string,
	_ time.Time,
	_ time.Duration,
) (bool, error) {
	business.attempts <- append([]string{}, resources...)
	select {
	case business.lease <- struct{}{}:
		return true, nil
	case <-ctx.Done():
		return false, ctx.Err()
	}
}

func (*vaultLeaseBusiness) RenewMutationLease(context.Context, string, []string, time.Time, time.Duration) (bool, error) {
	return true, nil
}

func (business *vaultLeaseBusiness) ReleaseMutationLease(context.Context, string, []string) error {
	<-business.lease
	return nil
}

func (f fakeQueueBusiness) NotificationQueueDetails(_ context.Context, channelKey string, _ bool) (business.NotificationQueueDetails, error) {
	if f.keys != nil {
		*f.keys = append(*f.keys, channelKey)
	}
	return f.details, nil
}

func (f fakeBusiness) Bootstrap(context.Context) error { return f.bootstrapError }
func (f fakeBusiness) Mode(context.Context) (string, error) {
	return f.mode, nil
}
func (f fakeBusiness) Ready(context.Context) (bool, error) { return true, nil }
func (f fakeBusiness) RuntimeSnapshot(context.Context) (business.RuntimeSnapshot, error) {
	return business.RuntimeSnapshot{
		Available:           true,
		Keys:                []any{"config/test"},
		Mode:                f.mode,
		ConfigurationErrors: []string{},
	}, nil
}
func (f fakeBusiness) SetMode(_ context.Context, mode string) (business.RuntimeSnapshot, error) {
	return business.RuntimeSnapshot{
		Available:           true,
		Keys:                []any{"config/test"},
		Mode:                mode,
		ConfigurationErrors: []string{},
	}, nil
}
func (f fakeBusiness) OverviewSummary(context.Context) (business.OverviewSummary, error) {
	return business.OverviewSummary{Available: true, Accounts: 12, Groups: 3, Alerts: 2, Runs: 8}, nil
}
func (f fakeBusiness) EnableNotificationChannel(_ context.Context, channelType string) error {
	if f.notificationChannels != nil {
		*f.notificationChannels = append(*f.notificationChannels, channelType)
	}
	return nil
}
func (f fakeBusiness) SetProbeEnabled(_ context.Context, enabled bool) error {
	if f.probeEnabled != nil {
		*f.probeEnabled = enabled
	}
	return nil
}
func (f fakeBusiness) Accounts(context.Context) ([]business.AccountStatus, error) {
	return f.accountRows, nil
}
func (f fakeBusiness) Account(context.Context, string) (*business.AccountDetail, error) {
	if f.accountDetail == nil {
		return nil, sql.ErrNoRows
	}
	return f.accountDetail, nil
}
func (f fakeBusiness) TrafficRanking(_ context.Context, query business.TrafficRankingQuery) (business.TrafficRanking, error) {
	if f.trafficQueries != nil {
		*f.trafficQueries = append(*f.trafficQueries, query)
	}
	return f.trafficRanking, f.trafficError
}
func (f fakeBusiness) Groups(context.Context) ([]business.GroupStatus, error) {
	return f.groupRows, nil
}
func (f fakeBusiness) GroupAllocation(context.Context, string) (business.GroupAllocation, error) {
	return f.groupAllocation, nil
}
func (f fakeBusiness) GroupProbeModels(context.Context, string) (business.GroupProbeModels, error) {
	return f.groupProbeModels, nil
}
func (f fakeBusiness) ControlPolicy(context.Context) (map[string]any, error) {
	if f.policySnapshot.AdvancedPolicy == nil {
		return map[string]any{}, nil
	}
	return f.policySnapshot.AdvancedPolicy, nil
}
func (f fakeBusiness) PolicySnapshot(context.Context) (business.PolicySnapshot, error) {
	return f.policySnapshot, nil
}
func (f fakeBusiness) ProbeEnabled(context.Context) (bool, error) {
	if f.probeEnabled == nil {
		return true, nil
	}
	return *f.probeEnabled, nil
}

func TestRecentResultsExposeConfiguredHealthEventAndScore(t *testing.T) {
	result, reason := "失败", "HTTP 502 Bad Gateway"
	accounts := []business.AccountStatus{{
		RecentResults: []business.AccountRecentResult{{
			Result: &result, FailureReason: &reason, Source: "traffic", ClassificationPayload: map[string]any{"status_code": 502},
		}},
	}}
	server := &Server{business: fakeBusiness{policySnapshot: business.PolicySnapshot{AdvancedPolicy: map[string]any{}}}}

	server.enrichRecentResults(context.Background(), accounts)

	recent := accounts[0].RecentResults[0]
	if recent.EventType == nil || *recent.EventType != "gateway_error" || recent.Score == nil || *recent.Score != 25 {
		t.Fatalf("recent result classification=%#v", recent)
	}
}
func (f fakeBusiness) UpdatePolicy(_ context.Context, patch map[string]any, actor string) (business.PolicySnapshot, error) {
	if f.policyUpdates != nil {
		*f.policyUpdates = append(*f.policyUpdates, patch)
	}
	if f.policyActors != nil {
		*f.policyActors = append(*f.policyActors, actor)
	}
	return f.policySnapshot, nil
}
func (f fakeBusiness) SetAccountTestModel(context.Context, string, *string, string) error {
	return nil
}
func (f fakeBusiness) UpdateGroupPolicy(_ context.Context, _ string, patch map[string]any, _ string) (business.GroupStatus, error) {
	if f.groupPolicyUpdates != nil {
		*f.groupPolicyUpdates = append(*f.groupPolicyUpdates, patch)
	}
	if len(f.groupRows) == 0 {
		return business.GroupStatus{}, business.ErrGroupNotFound
	}
	return f.groupRows[0], nil
}
func (f fakeBusiness) ClearGroupPolicy(_ context.Context, _ string, _ string) (business.GroupStatus, error) {
	if len(f.groupRows) == 0 {
		return business.GroupStatus{}, business.ErrGroupNotFound
	}
	return f.groupRows[0], nil
}
func (f fakeBusiness) SetGroupExcluded(_ context.Context, _ string, excluded bool, _ string) (business.GroupStatus, error) {
	if f.groupExcludedUpdates != nil {
		*f.groupExcludedUpdates = append(*f.groupExcludedUpdates, excluded)
	}
	if len(f.groupRows) == 0 {
		return business.GroupStatus{}, business.ErrGroupNotFound
	}
	return f.groupRows[0], nil
}
func (f fakeBusiness) Upstreams(context.Context) (business.UpstreamSummary, error) {
	return f.upstreamSummary, nil
}
func (f fakeBusiness) UpstreamGroups(context.Context, string, bool) ([]business.UpstreamGroup, error) {
	return f.upstreamGroupRows, nil
}
func (f fakeBusiness) UpstreamGroupHistory(context.Context, string, int) ([]business.UpstreamGroupChange, error) {
	return f.upstreamGroupHistory, nil
}
func (f fakeBusiness) Events(context.Context, *int) ([]business.RunEvent, error) {
	return []business.RunEvent{}, nil
}
func (f fakeBusiness) HealthSamples(context.Context, *int, *string, *string) ([]business.HealthSample, error) {
	return []business.HealthSample{}, nil
}
func (f fakeBusiness) RoutingDecisions(context.Context, *int, *string, *string) ([]business.RoutingDecision, error) {
	return []business.RoutingDecision{}, nil
}
func (f fakeBusiness) RunRecords(context.Context, *int) ([]business.RunRecord, error) {
	return []business.RunRecord{}, nil
}
func (f fakeBusiness) OperationalSnapshots(context.Context, *string, *int) ([]business.OperationalSnapshot, error) {
	return []business.OperationalSnapshot{}, nil
}
func (f fakeBusiness) UsageRecords(context.Context, *int, *string, *string) ([]business.UsageRecord, error) {
	return []business.UsageRecord{}, nil
}
func (f fakeBusiness) Alerts(context.Context, *int) ([]business.AlertListItem, error) {
	return f.alertRows, nil
}
func (f fakeBusiness) ClearAlerts(context.Context) (int64, error) { return f.clearedAlerts, nil }
func (f fakeBusiness) AlertPolicy(context.Context) (business.AlertPolicy, error) {
	return f.alertPolicy, nil
}
func (f fakeBusiness) UpdateAlertPolicy(_ context.Context, _ map[string]any) (business.AlertPolicy, error) {
	return f.alertPolicy, nil
}
func (f fakeBusiness) AuditEvents(context.Context, *int, bool) ([]business.AuditEvent, error) {
	return []business.AuditEvent{}, nil
}
func (f fakeBusiness) RecordRuntimeEvent(_ context.Context, _ string, _ string, _ string, _ map[string]any) (int64, error) {
	if f.runtimeEventIDs != nil {
		*f.runtimeEventIDs = append(*f.runtimeEventIDs, -1)
	}
	return -1, f.runtimeEventError
}

type fakeNotifier struct {
	result   notification.TestResult
	messages *[]string
}

func (f fakeNotifier) Test(_ context.Context, message string, _ bool) (notification.TestResult, error) {
	if f.messages != nil {
		*f.messages = append(*f.messages, message)
	}
	return f.result, nil
}

type fakeInspectionController struct {
	status  inspection.Status
	configs *[]business.AutoInspectionConfig
}

type fakeLogMaintenance struct {
	status  consolelogs.CleanupStatus
	updates *[][2]int
	result  consolelogs.CleanupResult
}

func (f fakeLogMaintenance) Status(context.Context) (consolelogs.CleanupStatus, error) {
	return f.status, nil
}
func (f fakeLogMaintenance) Update(_ context.Context, enabled bool, days int) (consolelogs.CleanupStatus, error) {
	if f.updates != nil {
		enabledValue := 0
		if enabled {
			enabledValue = 1
		}
		*f.updates = append(*f.updates, [2]int{enabledValue, days})
	}
	f.status.Enabled, f.status.RetentionDays = enabled, days
	return f.status, nil
}
func (f fakeLogMaintenance) ClearExpired(_ context.Context, days int) (consolelogs.CleanupResult, error) {
	f.result.RetentionDays = days
	return f.result, nil
}

type fakeTaskRepository struct {
	rows []taskstore.Task
}

type sequenceTaskRepository struct {
	mu    sync.Mutex
	rows  []taskstore.Task
	reads int
}

type fakeManagementTasks struct {
	actors *[]string
	task   taskstore.Task
	err    error
}

type fakeNotificationTargetDiscovery struct {
	task       taskstore.Task
	err        error
	requests   *[]notificationtarget.Request
	cancelled  *[]string
	cancelOkay bool
}

func (service fakeNotificationTargetDiscovery) Enqueue(_ context.Context, request notificationtarget.Request) (taskstore.Task, error) {
	if service.requests != nil {
		*service.requests = append(*service.requests, request)
	}
	return service.task, service.err
}

func (service fakeNotificationTargetDiscovery) Cancel(taskID string) bool {
	if service.cancelled != nil {
		*service.cancelled = append(*service.cancelled, taskID)
	}
	return service.cancelOkay
}

type fakeAccountMaintenanceTasks struct {
	task                taskstore.Task
	err                 error
	revalidate          *[][]string
	baseURLs            *[][]string
	configurationChecks *[][]string
	baseURLRepairs      *[][]string
	hosts               *[][]string
	repair              *[][]string
	cleanup             *[][]string
	rates               *[][]string
	defaults            *[][]string
}

func (tasks fakeAccountMaintenanceTasks) EnqueueAccountRateSync(_ context.Context, accountIDs []string, _ string) (taskstore.Task, error) {
	if tasks.rates != nil {
		*tasks.rates = append(*tasks.rates, append([]string{}, accountIDs...))
	}
	return tasks.task, tasks.err
}

type fieldsCall struct {
	accountID string
	patch     accountops.FieldPatch
	actor     string
}

type fakeAccountTasks struct {
	task         taskstore.Task
	err          error
	controlCalls *[]string
	fieldsCalls  *[]fieldsCall
	manualCalls  *[]string
	clearCalls   *[]string
}

type accountDeleteCall struct {
	accountID         string
	binding           *accountdelete.Binding
	managementBaseURL string
	actor             string
}

type fakeAccountDelete struct {
	preview            accountdelete.Preview
	batchPreview       accountdelete.BatchPreview
	task               taskstore.Task
	err                error
	calls              *[]accountDeleteCall
	batchConfirmations *[][]accountdelete.Confirmation
}

func (service fakeAccountDelete) Preview(context.Context, string) (accountdelete.Preview, error) {
	return service.preview, service.err
}

func (service fakeAccountDelete) PreviewBatch(_ context.Context, accountIDs []string) (accountdelete.BatchPreview, error) {
	if len(service.batchPreview.Accounts) > 0 {
		return service.batchPreview, service.err
	}
	return accountdelete.BatchPreview{Accounts: []accountdelete.Preview{service.preview}, AccountCount: len(accountIDs)}, service.err
}

func (service fakeAccountDelete) Enqueue(
	_ context.Context,
	accountID string,
	binding *accountdelete.Binding,
	managementBaseURL string,
	actor string,
) (taskstore.Task, error) {
	if service.calls != nil {
		*service.calls = append(*service.calls, accountDeleteCall{
			accountID: accountID, binding: binding, managementBaseURL: managementBaseURL, actor: actor,
		})
	}
	return service.task, service.err
}

func (service fakeAccountDelete) EnqueueBatch(
	_ context.Context,
	confirmations []accountdelete.Confirmation,
	_ string,
) (taskstore.Task, error) {
	if service.batchConfirmations != nil {
		copy := append([]accountdelete.Confirmation{}, confirmations...)
		*service.batchConfirmations = append(*service.batchConfirmations, copy)
	}
	return service.task, service.err
}

func (tasks fakeAccountTasks) EnqueueControl(_ context.Context, accountID, action, actor string) (taskstore.Task, error) {
	if tasks.controlCalls != nil {
		*tasks.controlCalls = append(*tasks.controlCalls, accountID+":"+action+":"+actor)
	}
	return tasks.task, tasks.err
}

type probeCall struct {
	request probe.Request
	actor   string
}

type fakeProbeTasks struct {
	task  taskstore.Task
	err   error
	calls *[]probeCall
}

type fakeModelChecks struct {
	task     taskstore.Task
	err      error
	requests *[]modelcheck.Request
	statuses []modelcheck.AccountCheckStatus
}

func (service fakeModelChecks) Capabilities() modelcheck.Capabilities {
	return modelcheck.Capabilities{
		ClaudeStandards: []string{"claude-opus-5"},
		SolModels:       []string{"gpt-5.6-sol", "gpt-5.6-luna", "gpt-5.6-terra"},
	}
}

func (service fakeModelChecks) AccountStatuses(context.Context) ([]modelcheck.AccountCheckStatus, error) {
	return service.statuses, service.err
}

func (service fakeModelChecks) Enqueue(_ context.Context, request modelcheck.Request) (taskstore.Task, error) {
	if service.requests != nil {
		*service.requests = append(*service.requests, request)
	}
	return service.task, service.err
}

type upstreamSyncCall struct {
	host      string
	scope     upstreamsync.Scope
	actor     string
	operation string
}

type fakeUpstreamSyncTasks struct {
	task       taskstore.Task
	err        error
	hostResult *upstreamsync.HostResult
	allCalls   *[]upstreamSyncCall
	hostCalls  *[]upstreamSyncCall
}

type fakeOnboarding struct {
	task           taskstore.Task
	err            error
	candidates     []business.OnboardingCandidate
	hosts          *[]string
	models         []string
	probe          onboarding.ProbeResult
	cleanupPreview onboarding.KeyCleanupPreview
	cleanupCalls   *[]struct {
		host   string
		keyIDs []string
		actor  string
	}
}

type fakeTraceReader struct {
	trace business.RequestTrace
	err   error
	ids   *[]string
}

type fakeSystemLogReader struct {
	page    business.SystemLogPage
	err     error
	queries *[]opstraffic.SystemLogQuery
}

type fakeAuthRecovery struct {
	manual            authrecovery.ManualResult
	task              taskstore.Task
	captchaCompletion authrecovery.CaptchaCompletion
	manualCalls       *[]authrecovery.ManualInput
	runCalls          *[]upstreamSyncCall
	agreementCalls    *[]bool
	captchaSubmits    *[]upstreamSyncCall
	captchaCancels    *[]string
	batchCalls        *[][]string
}

func (f fakeAuthRecovery) VerifyManual(_ context.Context, input authrecovery.ManualInput, actor string) (authrecovery.ManualResult, error) {
	if f.manualCalls != nil {
		*f.manualCalls = append(*f.manualCalls, input)
	}
	return f.manual, nil
}
func (f fakeAuthRecovery) Enqueue(_ context.Context, host, entry string, acceptLoginAgreement bool, actor string) (taskstore.Task, error) {
	if f.runCalls != nil {
		*f.runCalls = append(*f.runCalls, upstreamSyncCall{host: host, actor: actor, operation: entry})
	}
	if f.agreementCalls != nil {
		*f.agreementCalls = append(*f.agreementCalls, acceptLoginAgreement)
	}
	return f.task, nil
}
func (f fakeAuthRecovery) EnqueueBatch(_ context.Context, hosts []string, actor string) (taskstore.Task, error) {
	if f.batchCalls != nil {
		*f.batchCalls = append(*f.batchCalls, append([]string{}, hosts...))
	}
	return f.task, nil
}
func (f fakeAuthRecovery) SubmitCaptcha(_ context.Context, challengeID, code, actor string) (authrecovery.CaptchaCompletion, error) {
	if f.captchaSubmits != nil {
		*f.captchaSubmits = append(*f.captchaSubmits, upstreamSyncCall{host: challengeID, actor: actor, operation: code})
	}
	return f.captchaCompletion, nil
}
func (f fakeAuthRecovery) CancelCaptcha(challengeID string) bool {
	if f.captchaCancels != nil {
		*f.captchaCancels = append(*f.captchaCancels, challengeID)
	}
	return true
}

type fakeUpstreamDetector struct {
	result upstreamdetect.Result
	urls   *[]string
	err    error
}

type fakeUpstreamConfigurations struct {
	configuration upstreamconfig.Configuration
	created       *[]upstreamconfig.Input
	updated       *[]upstreamconfig.Input
	authRecords   *[]upstreamconfig.Input
	err           error
}

func (f fakeUpstreamConfigurations) Get(context.Context, string) (upstreamconfig.Configuration, error) {
	return f.configuration, f.err
}
func (f fakeUpstreamConfigurations) Create(_ context.Context, input upstreamconfig.Input, _ string) (upstreamconfig.Configuration, error) {
	if f.created != nil {
		*f.created = append(*f.created, input)
	}
	return f.configuration, f.err
}
func (f fakeUpstreamConfigurations) Update(_ context.Context, _ string, input upstreamconfig.Input, _ string) (upstreamconfig.Configuration, error) {
	if f.updated != nil {
		*f.updated = append(*f.updated, input)
	}
	return f.configuration, f.err
}
func (f fakeUpstreamConfigurations) ConfigureAuthRecord(_ context.Context, input upstreamconfig.Input) (string, error) {
	if f.authRecords != nil {
		*f.authRecords = append(*f.authRecords, input)
	}
	return input.Host, f.err
}

func (detector fakeUpstreamDetector) Detect(_ context.Context, baseURL string) (upstreamdetect.Result, error) {
	if detector.urls != nil {
		*detector.urls = append(*detector.urls, baseURL)
	}
	return detector.result, detector.err
}

func (tasks fakeProbeTasks) Enqueue(_ context.Context, request probe.Request, actor string) (taskstore.Task, error) {
	if tasks.calls != nil {
		*tasks.calls = append(*tasks.calls, probeCall{request: request, actor: actor})
	}
	return tasks.task, tasks.err
}

func (tasks fakeAccountTasks) EnqueueFields(_ context.Context, accountID string, patch accountops.FieldPatch, actor string) (taskstore.Task, error) {
	if tasks.fieldsCalls != nil {
		*tasks.fieldsCalls = append(*tasks.fieldsCalls, fieldsCall{accountID: accountID, patch: patch, actor: actor})
	}
	return tasks.task, tasks.err
}

func (tasks fakeAccountTasks) EnqueueManualPriority(_ context.Context, accountID string, priority int64, loadFactor string, concurrency int64, syncBalanceMultiplier bool, actor string) (taskstore.Task, error) {
	if tasks.manualCalls != nil {
		*tasks.manualCalls = append(*tasks.manualCalls, fmt.Sprintf("%s:%d:%s:%d:%t:%s", accountID, priority, loadFactor, concurrency, syncBalanceMultiplier, actor))
	}
	return tasks.task, tasks.err
}

func (tasks fakeAccountTasks) EnqueueClearManualPriority(_ context.Context, accountID, actor string) (taskstore.Task, error) {
	if tasks.clearCalls != nil {
		*tasks.clearCalls = append(*tasks.clearCalls, accountID+":"+actor)
	}
	return tasks.task, tasks.err
}

func (tasks fakeAccountTasks) Models(context.Context, string) ([]string, error) {
	return []string{"gpt-5.1-codex"}, tasks.err
}

func (tasks fakeManagementTasks) EnqueueSync(_ context.Context, actor string) (taskstore.Task, error) {
	if tasks.actors != nil {
		*tasks.actors = append(*tasks.actors, actor)
	}
	return tasks.task, tasks.err
}

func (tasks fakeAccountMaintenanceTasks) EnqueueAccountRevalidation(_ context.Context, accountIDs []string, _ string) (taskstore.Task, error) {
	if tasks.revalidate != nil {
		*tasks.revalidate = append(*tasks.revalidate, append([]string{}, accountIDs...))
	}
	return tasks.task, tasks.err
}

func (tasks fakeAccountMaintenanceTasks) EnqueueAccountBaseURLValidation(_ context.Context, accountIDs []string, _ string) (taskstore.Task, error) {
	if tasks.baseURLs != nil {
		*tasks.baseURLs = append(*tasks.baseURLs, append([]string{}, accountIDs...))
	}
	return tasks.task, tasks.err
}

func (tasks fakeAccountMaintenanceTasks) EnqueueAccountConfigurationCheck(_ context.Context, accountIDs []string, _ string) (taskstore.Task, error) {
	if tasks.configurationChecks != nil {
		*tasks.configurationChecks = append(*tasks.configurationChecks, append([]string{}, accountIDs...))
	}
	return tasks.task, tasks.err
}

func (tasks fakeAccountMaintenanceTasks) EnqueueAccountBaseURLRepair(_ context.Context, accountIDs []string, _ string) (taskstore.Task, error) {
	if tasks.baseURLRepairs != nil {
		*tasks.baseURLRepairs = append(*tasks.baseURLRepairs, append([]string{}, accountIDs...))
	}
	return tasks.task, tasks.err
}

func (tasks fakeAccountMaintenanceTasks) EnqueueAccountUpstreamHostRepair(_ context.Context, accountIDs []string, _ string) (taskstore.Task, error) {
	if tasks.hosts != nil {
		*tasks.hosts = append(*tasks.hosts, append([]string{}, accountIDs...))
	}
	return tasks.task, tasks.err
}

func (tasks fakeAccountMaintenanceTasks) EnqueueAccountNameRepair(_ context.Context, accountIDs []string, _ string) (taskstore.Task, error) {
	if tasks.repair != nil {
		*tasks.repair = append(*tasks.repair, append([]string{}, accountIDs...))
	}
	return tasks.task, tasks.err
}

func (tasks fakeAccountMaintenanceTasks) EnqueueAccountDefaultsRepair(_ context.Context, accountIDs []string, _ string) (taskstore.Task, error) {
	if tasks.defaults != nil {
		*tasks.defaults = append(*tasks.defaults, append([]string{}, accountIDs...))
	}
	return tasks.task, tasks.err
}

func (tasks fakeAccountMaintenanceTasks) EnqueueMissingBindingCleanup(_ context.Context, accountIDs []string, _ string) (taskstore.Task, error) {
	if tasks.cleanup != nil {
		*tasks.cleanup = append(*tasks.cleanup, append([]string{}, accountIDs...))
	}
	return tasks.task, tasks.err
}

func (tasks fakeUpstreamSyncTasks) EnqueueAll(_ context.Context, scope upstreamsync.Scope, actor, operation string) (taskstore.Task, error) {
	if tasks.allCalls != nil {
		*tasks.allCalls = append(*tasks.allCalls, upstreamSyncCall{scope: scope, actor: actor, operation: operation})
	}
	return tasks.task, tasks.err
}

func (tasks fakeUpstreamSyncTasks) EnqueueHost(_ context.Context, host string, scope upstreamsync.Scope, actor, operation string) (taskstore.Task, error) {
	if tasks.hostCalls != nil {
		*tasks.hostCalls = append(*tasks.hostCalls, upstreamSyncCall{host: host, scope: scope, actor: actor, operation: operation})
	}
	return tasks.task, tasks.err
}

func (tasks fakeUpstreamSyncTasks) SyncHost(_ context.Context, host string, scope upstreamsync.Scope, actor string) (upstreamsync.HostResult, error) {
	if tasks.hostCalls != nil {
		*tasks.hostCalls = append(*tasks.hostCalls, upstreamSyncCall{host: host, scope: scope, actor: actor, operation: "sync-host"})
	}
	if tasks.hostResult != nil {
		return *tasks.hostResult, tasks.err
	}
	return upstreamsync.HostResult{Host: host, Status: "succeeded", AuthStatus: "已鉴权", BalanceStatus: "已读取"}, tasks.err
}

func (f fakeOnboarding) Candidates(_ context.Context, host string) ([]business.OnboardingCandidate, error) {
	if f.hosts != nil {
		*f.hosts = append(*f.hosts, host)
	}
	return f.candidates, f.err
}

func (f fakeOnboarding) ProbeModels(context.Context, string, string) ([]string, error) {
	return f.models, f.err
}

func (f fakeOnboarding) Probe(context.Context, string, string, string) (onboarding.ProbeResult, error) {
	return f.probe, f.err
}

func (f fakeOnboarding) CancelProbe(context.Context, string, string) error {
	return f.err
}

func (f fakeOnboarding) PreviewUnboundKeys(context.Context, string) (onboarding.KeyCleanupPreview, error) {
	return f.cleanupPreview, f.err
}

func (f fakeOnboarding) EnqueueKeyCleanup(_ context.Context, host string, keyIDs []string, actor string) (taskstore.Task, error) {
	if f.cleanupCalls != nil {
		*f.cleanupCalls = append(*f.cleanupCalls, struct {
			host   string
			keyIDs []string
			actor  string
		}{host: host, keyIDs: append([]string{}, keyIDs...), actor: actor})
	}
	return f.task, f.err
}

func (f fakeOnboarding) Enqueue(context.Context, onboarding.Request) (taskstore.Task, error) {
	return f.task, f.err
}

func (f fakeOnboarding) EnqueueBatch(context.Context, []onboarding.Request) (taskstore.Task, error) {
	return f.task, f.err
}

func (f fakeTraceReader) RequestTrace(_ context.Context, requestID string) (business.RequestTrace, error) {
	if f.ids != nil {
		*f.ids = append(*f.ids, requestID)
	}
	return f.trace, f.err
}

func (f fakeSystemLogReader) SearchSystemLogs(_ context.Context, query opstraffic.SystemLogQuery) (business.SystemLogPage, error) {
	if f.queries != nil {
		*f.queries = append(*f.queries, query)
	}
	return f.page, f.err
}

func (f fakeTaskRepository) Get(_ context.Context, id string) (taskstore.Task, error) {
	for _, row := range f.rows {
		if row.ID == id {
			return row, nil
		}
	}
	return taskstore.Task{}, taskstore.ErrNotFound
}
func (f fakeTaskRepository) LatestByOperation(_ context.Context, operation, status string) (taskstore.Task, error) {
	for _, row := range f.rows {
		if row.Operation == operation && row.Status == status {
			return row, nil
		}
	}
	return taskstore.Task{}, taskstore.ErrNotFound
}
func (f *sequenceTaskRepository) Get(_ context.Context, id string) (taskstore.Task, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.rows) == 0 || f.rows[0].ID != id {
		return taskstore.Task{}, taskstore.ErrNotFound
	}
	index := f.reads
	if index >= len(f.rows) {
		index = len(f.rows) - 1
	}
	f.reads++
	return f.rows[index], nil
}
func (f *sequenceTaskRepository) LatestByOperation(_ context.Context, operation, status string) (taskstore.Task, error) {
	for _, row := range f.rows {
		if row.Operation == operation && row.Status == status {
			return row, nil
		}
	}
	return taskstore.Task{}, taskstore.ErrNotFound
}

func (f fakeInspectionController) Status(context.Context) (inspection.Status, error) {
	return f.status, nil
}
func (f fakeInspectionController) UpdateConfig(_ context.Context, config business.AutoInspectionConfig) (inspection.Status, error) {
	if f.configs != nil {
		*f.configs = append(*f.configs, config)
	}
	f.status.AutoInspectionConfig = config
	return f.status, nil
}
func (f fakeInspectionController) Cancel(context.Context) (inspection.Status, bool, error) {
	f.status.Enabled = false
	return f.status, f.status.Running, nil
}
func (f fakeInspectionController) Resume(context.Context) (inspection.Status, error) {
	f.status.Enabled = true
	return f.status, nil
}
func (f fakeInspectionController) ClearHistory(context.Context) (int64, error) {
	f.status.HeartbeatHistory = []business.InspectionHeartbeat{}
	return 2, nil
}
func (f fakeInspectionController) Subscribe() (<-chan struct{}, func()) {
	updates := make(chan struct{})
	return updates, func() { close(updates) }
}

func TestInitializationLoginSessionAndLogoutContract(t *testing.T) {
	router, store := testRouter(t, config.Config{}, fakeBusiness{mode: "完全模式"})

	status := request(t, router, http.MethodGet, "/api/setup/status", nil, "")
	if status.Code != http.StatusOK || !strings.Contains(status.Body.String(), `"initialized":false`) {
		t.Fatalf("unexpected setup status: %d %s", status.Code, status.Body.String())
	}
	protected := request(t, router, http.MethodGet, "/api/health", nil, "")
	if protected.Code != http.StatusPreconditionRequired || !strings.Contains(protected.Body.String(), "请先完成首次初始化") {
		t.Fatalf("uninitialized protected route was not rejected: %d %s", protected.Code, protected.Body.String())
	}

	initialized := request(t, router, http.MethodPost, "/api/setup/initialize", map[string]any{
		"username":       "admin",
		"password":       "a secure password",
		"admin_base_url": "https://sub2api.example",
		"admin_key":      "admin-key",
	}, "")
	if initialized.Code != http.StatusOK {
		t.Fatalf("initialization failed: %d %s", initialized.Code, initialized.Body.String())
	}
	cookie := responseCookie(t, initialized, sessionCookie)
	if !cookie.HttpOnly || cookie.Path != "/" || cookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("unexpected session cookie: %#v", cookie)
	}

	session := request(t, router, http.MethodGet, "/api/auth/session", nil, cookie.String())
	if session.Code != http.StatusOK || !strings.Contains(session.Body.String(), `"authenticated":true`) {
		t.Fatalf("unexpected session: %d %s", session.Code, session.Body.String())
	}

	unauthorized := request(t, router, http.MethodGet, "/api/health", nil, "")
	if unauthorized.Code != http.StatusUnauthorized || !strings.Contains(unauthorized.Body.String(), "请先登录控制台") {
		t.Fatalf("unexpected unauthorized response: %d %s", unauthorized.Code, unauthorized.Body.String())
	}
	health := request(t, router, http.MethodGet, "/api/health", nil, cookie.String())
	if health.Code != http.StatusOK || health.Body.String() != `{"mode":"完全模式","status":"ok"}` {
		t.Fatalf("unexpected health response: %d %s", health.Code, health.Body.String())
	}

	logout := request(t, router, http.MethodPost, "/api/auth/logout", nil, cookie.String())
	if logout.Code != http.StatusOK || !strings.Contains(logout.Body.String(), `"authenticated":false`) {
		t.Fatalf("unexpected logout: %d %s", logout.Code, logout.Body.String())
	}
	username, err := store.SessionUser(context.Background(), cookie.Value, testNow())
	if err != nil {
		t.Fatal(err)
	}
	if username != nil {
		t.Fatal("logout must revoke persisted session")
	}
}

func TestCORSPreflightUsesExplicitCredentialCompatibleHeaders(t *testing.T) {
	router, _ := testRouter(t, config.Config{Origins: []string{"https://console.example"}}, fakeBusiness{mode: "完全模式"})
	req := httptest.NewRequest(http.MethodOptions, "/api/health", nil)
	req.Header.Set("Origin", "https://console.example")
	req.Header.Set("Access-Control-Request-Method", http.MethodGet)
	req.Header.Set("Access-Control-Request-Headers", "content-type,authorization")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	if response.Code != http.StatusNoContent {
		t.Fatalf("preflight status = %d", response.Code)
	}
	if got := response.Header().Get("Access-Control-Allow-Headers"); got != "Content-Type, Authorization, X-Setup-Token" {
		t.Fatalf("allow headers = %q", got)
	}
	if got := response.Header().Get("Access-Control-Allow-Origin"); got != "https://console.example" {
		t.Fatalf("allow origin = %q", got)
	}
}

func TestCORSRejectsPreflightFromUnknownOrigin(t *testing.T) {
	router, _ := testRouter(t, config.Config{Origins: []string{"https://console.example"}}, fakeBusiness{mode: "完全模式"})
	req := httptest.NewRequest(http.MethodOptions, "/api/health", nil)
	req.Header.Set("Origin", "https://attacker.example")
	req.Header.Set("Access-Control-Request-Method", http.MethodPost)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	if response.Code != http.StatusForbidden {
		t.Fatalf("unknown preflight origin status = %d body=%s", response.Code, response.Body.String())
	}
}

func TestRemoteInitializationRequiresConfiguredSetupToken(t *testing.T) {
	const setupToken = "0123456789abcdef0123456789abcdef"
	for _, test := range []struct {
		name       string
		configured string
		supplied   string
		want       int
	}{
		{name: "missing configuration", want: http.StatusForbidden},
		{name: "missing token", configured: setupToken, want: http.StatusForbidden},
		{name: "wrong token", configured: setupToken, supplied: strings.Repeat("f", 32), want: http.StatusForbidden},
		{name: "valid token", configured: setupToken, supplied: setupToken, want: http.StatusOK},
	} {
		t.Run(test.name, func(t *testing.T) {
			router, _ := testRouter(t, config.Config{SetupToken: test.configured}, fakeBusiness{mode: "完全模式"})
			body := strings.NewReader(`{"username":"admin","password":"a secure password","admin_base_url":"https://sub2api.example","admin_key":"admin-key"}`)
			req := httptest.NewRequest(http.MethodPost, "/api/setup/initialize", body)
			req.RemoteAddr = "198.51.100.20:1234"
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Origin", "http://example.com")
			if test.supplied != "" {
				req.Header.Set("X-Setup-Token", test.supplied)
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			if response.Code != test.want {
				t.Fatalf("status = %d, want %d body=%s", response.Code, test.want, response.Body.String())
			}
		})
	}
}

func TestLoopbackInitializationWithExternalHostRequiresSetupToken(t *testing.T) {
	const setupToken = "0123456789abcdef0123456789abcdef"
	router, _ := testRouter(t, config.Config{SetupToken: setupToken}, fakeBusiness{mode: "完全模式"})
	newRequest := func(method string, browserOrigin bool) *http.Request {
		body := strings.NewReader("")
		if method == http.MethodPost {
			body = strings.NewReader(`{"username":"admin","password":"a secure password","admin_base_url":"https://sub2api.example","admin_key":"admin-key"}`)
		}
		req := httptest.NewRequest(method, "/api/setup/status", body)
		if method == http.MethodPost {
			req.URL.Path = "/api/setup/initialize"
			req.Header.Set("Content-Type", "application/json")
		}
		req.Host = "rebind.example"
		req.RemoteAddr = "127.0.0.1:1234"
		if browserOrigin {
			req.Header.Set("Origin", "http://rebind.example")
		}
		return req
	}
	for _, browserOrigin := range []bool{false, true} {
		status := httptest.NewRecorder()
		router.ServeHTTP(status, newRequest(http.MethodGet, browserOrigin))
		if status.Code != http.StatusOK || !strings.Contains(status.Body.String(), `"setup_token_required":true`) {
			t.Fatalf("rebound setup status with browser_origin=%t: %d %s", browserOrigin, status.Code, status.Body.String())
		}
	}
	for _, test := range []struct {
		name  string
		token string
		want  int
	}{
		{name: "missing token", want: http.StatusForbidden},
		{name: "valid token", token: setupToken, want: http.StatusOK},
	} {
		t.Run(test.name, func(t *testing.T) {
			req := newRequest(http.MethodPost, false)
			if test.token != "" {
				req.Header.Set("X-Setup-Token", test.token)
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			if response.Code != test.want {
				t.Fatalf("status = %d, want %d body=%s", response.Code, test.want, response.Body.String())
			}
		})
	}
}

func TestSetupStatusReportsWhetherRemoteTokenIsRequired(t *testing.T) {
	router, _ := testRouter(t, config.Config{}, fakeBusiness{mode: "完全模式"})
	local := request(t, router, http.MethodGet, "/api/setup/status", nil, "")
	if !strings.Contains(local.Body.String(), `"setup_token_required":false`) {
		t.Fatalf("local setup status = %s", local.Body.String())
	}
	req := httptest.NewRequest(http.MethodGet, "/api/setup/status", nil)
	req.RemoteAddr = "198.51.100.20:1234"
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	if !strings.Contains(response.Body.String(), `"setup_token_required":true`) {
		t.Fatalf("remote setup status = %s", response.Body.String())
	}
	conflicting := httptest.NewRequest(http.MethodGet, "/api/setup/status", nil)
	conflicting.RemoteAddr = "127.0.0.1:1234"
	conflicting.Host = "localhost"
	conflicting.Header.Set("Origin", "http://localhost")
	conflicting.Header.Set("Referer", "http://rebind.example/setup")
	conflictingResponse := httptest.NewRecorder()
	router.ServeHTTP(conflictingResponse, conflicting)
	if !strings.Contains(conflictingResponse.Body.String(), `"setup_token_required":true`) {
		t.Fatalf("conflicting browser origins setup status = %s", conflictingResponse.Body.String())
	}
	securedRouter, _ := testRouter(t, config.Config{SetupToken: strings.Repeat("s", 32)}, fakeBusiness{mode: "完全模式"})
	securedLocal := request(t, securedRouter, http.MethodGet, "/api/setup/status", nil, "")
	if !strings.Contains(securedLocal.Body.String(), `"setup_token_required":true`) {
		t.Fatalf("configured setup token was optional locally: %s", securedLocal.Body.String())
	}
}

func TestTrustedProxyCannotForgeLoopbackInitialization(t *testing.T) {
	router, _ := testRouter(t, config.Config{
		TrustedProxyCIDRs: []netip.Prefix{netip.MustParsePrefix("172.16.0.0/12")},
	}, fakeBusiness{mode: "完全模式"})
	newRequest := func(method, path string, body io.Reader) *http.Request {
		req := httptest.NewRequest(method, path, body)
		req.RemoteAddr = "172.18.0.1:1234"
		req.Host = "localhost:8080"
		req.Header.Set("X-Forwarded-For", "127.0.0.1")
		return req
	}

	req := newRequest(http.MethodGet, "/api/setup/status", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"setup_token_required":true`) {
		t.Fatalf("forged forwarded loopback setup status = %d body=%s", response.Code, response.Body.String())
	}

	body := strings.NewReader(`{"username":"admin","password":"a secure password","admin_base_url":"https://sub2api.example","admin_key":"admin-key"}`)
	initialize := newRequest(http.MethodPost, "/api/setup/initialize", body)
	initialize.Header.Set("Content-Type", "application/json")
	initializeResponse := httptest.NewRecorder()
	router.ServeHTTP(initializeResponse, initialize)
	if initializeResponse.Code != http.StatusForbidden {
		t.Fatalf("forged forwarded loopback initialize status = %d body=%s", initializeResponse.Code, initializeResponse.Body.String())
	}
}

func TestCookieWritesRequireTrustedBrowserOrigin(t *testing.T) {
	probeEnabled := false
	router, store := testRouter(t, config.Config{Origins: []string{"https://console.example"}}, fakeBusiness{
		mode: "完全模式", probeEnabled: &probeEnabled,
	})
	if err := store.Initialize(context.Background(), "operator", "correct password", "https://sub2api.example", "key"); err != nil {
		t.Fatal(err)
	}
	login := request(t, router, http.MethodPost, "/api/auth/login", map[string]any{
		"username": "operator", "password": "correct password",
	}, "")
	cookie := responseCookie(t, login, sessionCookie)

	write := func(origin, referer string) *httptest.ResponseRecorder {
		body := strings.NewReader(`{"enabled":true}`)
		req := httptest.NewRequest(http.MethodPost, "/api/config/probes", body)
		req.RemoteAddr = "198.51.100.20:1234"
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Cookie", cookie.String())
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		if referer != "" {
			req.Header.Set("Referer", referer)
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		return response
	}

	for _, response := range []*httptest.ResponseRecorder{
		write("https://attacker.example", ""),
		write("", ""),
		write("", "https://attacker.example/form"),
	} {
		if response.Code != http.StatusForbidden {
			t.Fatalf("untrusted cookie write status = %d body=%s", response.Code, response.Body.String())
		}
	}
	if probeEnabled {
		t.Fatal("rejected request changed probe settings")
	}
	for _, response := range []*httptest.ResponseRecorder{
		write("https://console.example", ""),
		write("", "http://example.com/settings"),
	} {
		if response.Code != http.StatusOK {
			t.Fatalf("trusted cookie write status = %d body=%s", response.Code, response.Body.String())
		}
	}
}

func TestCookieWriteRecognizesForwardedHTTPSOriginWithPort(t *testing.T) {
	probeEnabled := false
	router, store := testRouter(t, config.Config{
		TrustedProxyCIDRs: []netip.Prefix{netip.MustParsePrefix("172.16.0.0/12")},
	}, fakeBusiness{mode: "完全模式", probeEnabled: &probeEnabled})
	if err := store.Initialize(context.Background(), "operator", "correct password", "https://sub2api.example", "key"); err != nil {
		t.Fatal(err)
	}
	login := request(t, router, http.MethodPost, "/api/auth/login", map[string]any{
		"username": "operator", "password": "correct password",
	}, "")
	cookie := responseCookie(t, login, sessionCookie)

	body := strings.NewReader(`{"enabled":true}`)
	req := httptest.NewRequest(http.MethodPost, "/api/config/probes", body)
	req.RemoteAddr = "172.18.0.3:1234"
	req.Host = "console.example:3004"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", cookie.String())
	req.Header.Set("Origin", "https://console.example:3004")
	req.Header.Set("X-Forwarded-Proto", "https")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	if response.Code != http.StatusOK || !probeEnabled {
		t.Fatalf("forwarded same-origin write status = %d body=%s", response.Code, response.Body.String())
	}
}

func TestJSONBindingRejectsNonJSONContentType(t *testing.T) {
	router, _ := testRouter(t, config.Config{}, fakeBusiness{mode: "完全模式"})
	body := strings.NewReader(`{"username":"admin","password":"a secure password","admin_base_url":"https://sub2api.example","admin_key":"admin-key"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/setup/initialize", body)
	req.RemoteAddr = "127.0.0.1:1234"
	req.Host = "127.0.0.1"
	req.Header.Set("Content-Type", "text/plain")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("non-JSON request status = %d body=%s", response.Code, response.Body.String())
	}
}

func TestInitializationRejectsUnknownFieldsAndOversizedBodies(t *testing.T) {
	router, _ := testRouter(t, config.Config{}, fakeBusiness{mode: "完全模式"})
	unknown := request(t, router, http.MethodPost, "/api/setup/initialize", map[string]any{
		"username": "admin", "password": "a secure password", "admin_base_url": "https://sub2api.example",
		"admin_key": "admin-key", "obsolete_mode": "legacy",
	}, "")
	if unknown.Code != http.StatusUnprocessableEntity {
		t.Fatalf("unknown field status = %d body=%s", unknown.Code, unknown.Body.String())
	}

	trailingBody := strings.NewReader(`{"username":"admin","password":"a secure password","admin_base_url":"https://sub2api.example","admin_key":"admin-key"}{}`)
	trailingRequest := httptest.NewRequest(http.MethodPost, "/api/setup/initialize", trailingBody)
	trailingRequest.RemoteAddr = "127.0.0.1:1234"
	trailingRequest.Host = "127.0.0.1"
	trailingRequest.Header.Set("Content-Type", "application/json")
	trailingResponse := httptest.NewRecorder()
	router.ServeHTTP(trailingResponse, trailingRequest)
	if trailingResponse.Code != http.StatusUnprocessableEntity {
		t.Fatalf("trailing JSON status = %d body=%s", trailingResponse.Code, trailingResponse.Body.String())
	}

	body := strings.NewReader(`{"username":"admin","password":"` + strings.Repeat("x", maximumRequestBytes) + `"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/setup/initialize", body)
	req.RemoteAddr = "127.0.0.1:1234"
	req.Host = "127.0.0.1"
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized body status = %d body=%s", response.Code, response.Body.String())
	}
}

func TestLoginRejectsWrongPasswordAndAcceptsPersistedCredentials(t *testing.T) {
	router, store := testRouter(t, config.Config{}, fakeBusiness{mode: "监控模式"})
	if err := store.Initialize(context.Background(), "operator", "correct password", "https://sub2api.example", "key"); err != nil {
		t.Fatal(err)
	}
	wrong := request(t, router, http.MethodPost, "/api/auth/login", map[string]any{
		"username": "operator", "password": "wrong",
	}, "")
	if wrong.Code != http.StatusUnauthorized {
		t.Fatalf("wrong password status = %d", wrong.Code)
	}
	correct := request(t, router, http.MethodPost, "/api/auth/login", map[string]any{
		"username": "operator", "password": "correct password",
	}, "")
	if correct.Code != http.StatusOK {
		t.Fatalf("login failed: %d %s", correct.Code, correct.Body.String())
	}
}

func TestLoginRateLimitsRepeatedFailures(t *testing.T) {
	router, store := testRouter(t, config.Config{}, fakeBusiness{mode: "监控模式"})
	if err := store.Initialize(context.Background(), "operator", "correct password", "https://sub2api.example", "key"); err != nil {
		t.Fatal(err)
	}
	for attempt := 0; attempt < loginFailureLimit; attempt++ {
		response := request(t, router, http.MethodPost, "/api/auth/login", map[string]any{
			"username": "operator", "password": "wrong",
		}, "")
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d status = %d", attempt+1, response.Code)
		}
	}
	limited := request(t, router, http.MethodPost, "/api/auth/login", map[string]any{
		"username": "operator", "password": "correct password",
	}, "")
	if limited.Code != http.StatusTooManyRequests || limited.Header().Get("Retry-After") == "" {
		t.Fatalf("limited response = %d headers=%v body=%s", limited.Code, limited.Header(), limited.Body.String())
	}
}

func TestPublicSetupStatusDoesNotExposeConfiguredUsername(t *testing.T) {
	router, store := testRouter(t, config.Config{}, fakeBusiness{mode: "监控模式"})
	if err := store.Initialize(context.Background(), "operator", "correct password", "https://sub2api.example", "key"); err != nil {
		t.Fatal(err)
	}
	response := request(t, router, http.MethodGet, "/api/setup/status", nil, "")
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), "username") || strings.Contains(response.Body.String(), "operator") {
		t.Fatalf("public setup status exposed username: %d %s", response.Code, response.Body.String())
	}
}

func TestBearerAdminTokenBypassesSession(t *testing.T) {
	router, _ := testRouter(t, config.Config{AdminToken: "internal-token"}, fakeBusiness{mode: "监控模式"})
	requestWithoutToken := request(t, router, http.MethodGet, "/api/health", nil, "")
	if requestWithoutToken.Code != http.StatusUnauthorized {
		t.Fatalf("missing token status = %d", requestWithoutToken.Code)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	req.Header.Set("Authorization", "Bearer internal-token")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("bearer authentication failed: %d %s", response.Code, response.Body.String())
	}
}

func TestConfiguredAdminTokenDoesNotDisableBrowserSession(t *testing.T) {
	router, store := testRouter(t, config.Config{AdminToken: "internal-token"}, fakeBusiness{mode: "监控模式"})
	if err := store.Initialize(context.Background(), "operator", "correct password", "https://sub2api.example", "key"); err != nil {
		t.Fatal(err)
	}
	login := request(t, router, http.MethodPost, "/api/auth/login", map[string]any{
		"username": "operator", "password": "correct password",
	}, "")
	if login.Code != http.StatusOK {
		t.Fatalf("login failed: %d %s", login.Code, login.Body.String())
	}
	cookie := responseCookie(t, login, sessionCookie)
	response := request(t, router, http.MethodGet, "/api/health", nil, cookie.String())
	if response.Code != http.StatusOK {
		t.Fatalf("session authentication was disabled by admin token: %d %s", response.Code, response.Body.String())
	}
}

func TestOverviewConfigAndModeContract(t *testing.T) {
	cfg := config.Config{DataDB: "/data/sub2api-console.sqlite3"}
	router, store := testRouter(t, cfg, fakeBusiness{mode: "监控模式"})
	if err := store.Initialize(context.Background(), "operator", "correct password", "https://sub2api.example", "key"); err != nil {
		t.Fatal(err)
	}
	login := request(t, router, http.MethodPost, "/api/auth/login", map[string]any{
		"username": "operator", "password": "correct password",
	}, "")
	cookie := responseCookie(t, login, sessionCookie)

	overview := request(t, router, http.MethodGet, "/api/overview", nil, cookie.String())
	if overview.Code != http.StatusOK || !strings.Contains(overview.Body.String(), `"account_count":12`) || strings.Contains(overview.Body.String(), "skill_count") {
		t.Fatalf("unexpected overview: %d %s", overview.Code, overview.Body.String())
	}
	configuration := request(t, router, http.MethodGet, "/api/config", nil, cookie.String())
	if configuration.Code != http.StatusOK || !strings.Contains(configuration.Body.String(), `"mode":"监控模式"`) ||
		!strings.Contains(configuration.Body.String(), `"console_username":"op***"`) ||
		!strings.Contains(configuration.Body.String(), `"account_default_concurrency":10`) ||
		!strings.Contains(configuration.Body.String(), `"account_default_priority":1`) {
		t.Fatalf("unexpected config: %d %s", configuration.Code, configuration.Body.String())
	}
	accountDefaults := request(t, router, http.MethodPost, "/api/config/account-defaults", map[string]any{
		"concurrency": 24, "priority": 7,
	}, cookie.String())
	if accountDefaults.Code != http.StatusOK ||
		!strings.Contains(accountDefaults.Body.String(), `"account_default_concurrency":24`) ||
		!strings.Contains(accountDefaults.Body.String(), `"account_default_priority":7`) {
		t.Fatalf("unexpected account defaults update: %d %s", accountDefaults.Code, accountDefaults.Body.String())
	}
	invalidDefaults := request(t, router, http.MethodPost, "/api/config/account-defaults", map[string]any{
		"concurrency": 0, "priority": 1,
	}, cookie.String())
	if invalidDefaults.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid account defaults response: %d %s", invalidDefaults.Code, invalidDefaults.Body.String())
	}
	updated := request(t, router, http.MethodPost, "/api/config/mode", map[string]any{"mode": "完全模式"}, cookie.String())
	if updated.Code != http.StatusOK || !strings.Contains(updated.Body.String(), `"mode":"完全模式"`) {
		t.Fatalf("unexpected mode update: %d %s", updated.Code, updated.Body.String())
	}
}

func TestParseOnboardingRequestAcceptsPerAccountConcurrencyAndPriority(t *testing.T) {
	request, err := parseOnboardingRequest(map[string]any{
		"host": "upstream.example", "upstream_type": "sub2api",
		"local_group_id": json.Number("3"), "upstream_group_id": "6",
		"concurrency": json.Number("24"), "priority": json.Number("7"),
		"base_url": "https://account-api.example/v1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if request.Concurrency == nil || *request.Concurrency != 24 || request.Priority == nil || *request.Priority != 7 {
		t.Fatalf("request=%#v", request)
	}
	if request.BaseURL == nil || *request.BaseURL != "https://account-api.example/v1" {
		t.Fatalf("account base URL=%#v", request.BaseURL)
	}
	if _, err := parseOnboardingRequest(map[string]any{
		"host": "upstream.example", "upstream_type": "sub2api", "multiplier": "9.9",
		"local_group_id": json.Number("3"), "upstream_group_id": "6",
	}); err == nil || !strings.Contains(err.Error(), "未知字段") {
		t.Fatalf("client multiplier must be rejected: %v", err)
	}
	if _, err := parseOnboardingRequest(map[string]any{
		"host": "upstream.example", "upstream_type": "sub2api",
		"local_group_id": json.Number("3"), "upstream_group_id": "6", "concurrency": json.Number("0"),
	}); err == nil {
		t.Fatal("zero per-account concurrency must be rejected")
	}
}

func TestParseOnboardingRequestAcceptsMultipleGroupsAndExistingAccounts(t *testing.T) {
	request, err := parseOnboardingRequest(map[string]any{
		"host": "upstream.example", "upstream_type": "sub2api",
		"local_group_ids": []any{json.Number("3"), json.Number("4")},
		"account_ids":     []any{"77"}, "upstream_group_id": "6",
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(request.LocalGroupIDs, ",") != "3,4" || strings.Join(request.AccountIDs, ",") != "77" {
		t.Fatalf("request=%#v", request)
	}
	if _, err := parseOnboardingRequest(map[string]any{
		"host": "upstream.example", "upstream_type": "sub2api",
		"local_group_ids": []any{json.Number("0")}, "upstream_group_id": "6",
	}); err == nil {
		t.Fatal("invalid local group IDs must be rejected")
	}
}

func TestTargetAndNotificationConfigurationContracts(t *testing.T) {
	channels := make([]string, 0)
	router, store := testRouter(t, config.Config{DataDB: "/data/sub2api-console.sqlite3"}, fakeBusiness{
		mode: "完全模式", notificationChannels: &channels,
	})
	if err := store.Initialize(context.Background(), "operator", "correct password", "https://old.example", "existing-key"); err != nil {
		t.Fatal(err)
	}
	login := request(t, router, http.MethodPost, "/api/auth/login", map[string]any{
		"username": "operator", "password": "correct password",
	}, "")
	cookie := responseCookie(t, login, sessionCookie)

	target := request(t, router, http.MethodPost, "/api/config/target", map[string]any{
		"admin_base_url": "https://new.example/", "admin_key": "", "request_timeout_seconds": 45,
	}, cookie.String())
	if target.Code != http.StatusOK || !strings.Contains(target.Body.String(), `"admin_base_url":"https://new.example"`) || !strings.Contains(target.Body.String(), `"request_timeout_seconds":45`) {
		t.Fatalf("unexpected target response: %d %s", target.Code, target.Body.String())
	}

	initialStatus := request(t, router, http.MethodGet, "/api/notifications/status", nil, cookie.String())
	if initialStatus.Code != http.StatusOK || !strings.Contains(initialStatus.Body.String(), `"configured":false`) ||
		!strings.Contains(initialStatus.Body.String(), `"queues":{"producer_firing":0`) {
		t.Fatalf("unexpected notification status: %d %s", initialStatus.Code, initialStatus.Body.String())
	}
	configured := request(t, router, http.MethodPost, "/api/notifications/config", map[string]any{
		"app_id": "app", "client_secret": "secret", "home_channel": "target", "home_channel_type": "c2c",
	}, cookie.String())
	if configured.Code != http.StatusOK || !strings.Contains(configured.Body.String(), `"configured":true`) ||
		!strings.Contains(configured.Body.String(), `"app_id":"app"`) ||
		!strings.Contains(configured.Body.String(), `"client_secret_configured":true`) ||
		strings.Contains(configured.Body.String(), `"client_secret":"secret"`) {
		t.Fatalf("unexpected notification configuration: %d %s", configured.Code, configured.Body.String())
	}
	updated := request(t, router, http.MethodPost, "/api/notifications/config", map[string]any{
		"app_id": "updated-app", "client_secret": "", "home_channel": "updated-target", "home_channel_type": "group",
	}, cookie.String())
	if updated.Code != http.StatusOK || !strings.Contains(updated.Body.String(), `"app_id":"updated-app"`) ||
		!strings.Contains(updated.Body.String(), `"home_channel":"updated-target"`) {
		t.Fatalf("unexpected notification update: %d %s", updated.Code, updated.Body.String())
	}
	privateNotification, err := store.NotificationSettings(context.Background())
	if err != nil || privateNotification.ClientSecret != "secret" {
		t.Fatalf("blank notification secret was not preserved: %#v err=%v", privateNotification, err)
	}
	if len(channels) != 2 || channels[0] != "qqbot" || channels[1] != "qqbot" {
		t.Fatalf("public notification rule not enabled: %#v", channels)
	}
}

func TestNotificationTargetDiscoveryReusesSavedSecretAndCanBeCancelled(t *testing.T) {
	requests := []notificationtarget.Request{}
	cancelled := []string{}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	task := taskstore.Task{
		ID: "qqbot-target-1", Skill: "qqbot", Operation: "discover-notification-target",
		Status: "queued", Progress: 0, Message: "已创建", Result: map[string]any{"target_type": "c2c"},
		CreatedAt: now, UpdatedAt: now,
	}
	router, store := testRouterWithDependencies(t, config.Config{}, fakeBusiness{mode: "完全模式"}, Dependencies{
		NotificationTarget: fakeNotificationTargetDiscovery{
			task: task, requests: &requests, cancelled: &cancelled, cancelOkay: true,
		},
	})
	if err := store.Initialize(context.Background(), "operator", "correct password", "https://sub2api.example", "key"); err != nil {
		t.Fatal(err)
	}
	if err := store.ConfigureNotifications(context.Background(), "saved-app", "saved-secret", "old-target", "c2c"); err != nil {
		t.Fatal(err)
	}
	login := request(t, router, http.MethodPost, "/api/auth/login", map[string]any{
		"username": "operator", "password": "correct password",
	}, "")
	cookie := responseCookie(t, login, sessionCookie)
	started := request(t, router, http.MethodPost, "/api/notifications/target-discovery", map[string]any{
		"app_id": "saved-app", "client_secret": "", "target_type": "c2c",
	}, cookie.String())
	if started.Code != http.StatusAccepted || !strings.Contains(started.Body.String(), `"id":"qqbot-target-1"`) || strings.Contains(started.Body.String(), "saved-secret") {
		t.Fatalf("unexpected discovery response: %d %s", started.Code, started.Body.String())
	}
	if len(requests) != 1 || requests[0].AppID != "saved-app" || requests[0].ClientSecret != "saved-secret" || requests[0].TargetType != "c2c" {
		t.Fatalf("discovery request = %#v", requests)
	}
	stopped := request(t, router, http.MethodDelete, "/api/notifications/target-discovery/qqbot-target-1", nil, cookie.String())
	if stopped.Code != http.StatusAccepted || len(cancelled) != 1 || cancelled[0] != "qqbot-target-1" {
		t.Fatalf("unexpected cancel response: %d %s calls=%#v", stopped.Code, stopped.Body.String(), cancelled)
	}
}

func TestProbeConfigurationUpdatesTheSingleBusinessPolicySwitch(t *testing.T) {
	probeEnabled := false
	router, store := testRouter(t, config.Config{DataDB: "/data/sub2api-console.sqlite3"}, fakeBusiness{
		mode: "完全模式", probeEnabled: &probeEnabled,
	})
	if err := store.Initialize(context.Background(), "operator", "correct password", "https://sub2api.example", "key"); err != nil {
		t.Fatal(err)
	}
	login := request(t, router, http.MethodPost, "/api/auth/login", map[string]any{
		"username": "operator", "password": "correct password",
	}, "")
	cookie := responseCookie(t, login, sessionCookie)

	response := request(t, router, http.MethodPost, "/api/config/probes", map[string]any{"enabled": true}, cookie.String())
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"probes_enabled":true`) {
		t.Fatalf("unexpected probe response: %d %s", response.Code, response.Body.String())
	}
	if !probeEnabled {
		t.Fatal("business probe policy was not updated")
	}
}

func TestAccountReadContractsAndRetiredBindingRoute(t *testing.T) {
	accountID := "41"
	name := "upstream-0.1"
	account := business.AccountStatus{
		ID: accountID, Name: name, Groups: []string{"codex"}, Health: "healthy",
		RecentResults: []business.AccountRecentResult{},
	}
	detail := &business.AccountDetail{
		AccountStatus: account,
		Metadata:      map[string]any{"status": "active"},
		GroupRates:    map[string]*string{"codex": nil},
		GroupIDs:      map[string]*string{"codex": nil},
		Bindings:      []business.AccountBinding{},
	}
	router, _ := testRouter(t, config.Config{AdminToken: "test-token"}, fakeBusiness{
		mode:          "完全模式",
		accountRows:   []business.AccountStatus{account},
		accountDetail: detail,
	})

	accounts := authenticatedRequest(t, router, http.MethodGet, "/api/accounts", nil)
	if accounts.Code != http.StatusOK || !strings.Contains(accounts.Body.String(), `"id":"41"`) || !strings.Contains(accounts.Body.String(), `"recent_results":[]`) {
		t.Fatalf("unexpected accounts response: %d %s", accounts.Code, accounts.Body.String())
	}
	accountResponse := authenticatedRequest(t, router, http.MethodGet, "/api/accounts/41", nil)
	if accountResponse.Code != http.StatusOK || !strings.Contains(accountResponse.Body.String(), `"metadata":{"status":"active"}`) {
		t.Fatalf("unexpected account response: %d %s", accountResponse.Code, accountResponse.Body.String())
	}
	invalid := authenticatedRequest(t, router, http.MethodGet, "/api/accounts/by-name", nil)
	if invalid.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid stable ID status = %d", invalid.Code)
	}
	bindings := authenticatedRequest(t, router, http.MethodGet, "/api/bindings?host=api.example&account_id=41&limit=10", nil)
	if bindings.Code != http.StatusNotFound {
		t.Fatalf("retired binding route status = %d: %s", bindings.Code, bindings.Body.String())
	}
}

func TestManagementSyncQueuesGoDomainTask(t *testing.T) {
	actors := []string{}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	task := taskstore.Task{
		ID: "management-1", Skill: "sub2api-operations", Operation: "management-snapshot-sync",
		Status: "queued", Progress: 0, Message: "管理快照同步已排队", Result: map[string]any{},
		CreatedAt: now, UpdatedAt: now,
	}
	router, _ := testRouterWithDependencies(t, config.Config{AdminToken: "test-token"}, fakeBusiness{mode: "完全模式"}, Dependencies{
		ManagementTasks: fakeManagementTasks{actors: &actors, task: task},
	})
	response := authenticatedRequest(t, router, http.MethodPost, "/api/management/sync", nil)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"id":"management-1"`) {
		t.Fatalf("unexpected response: %d %s", response.Code, response.Body.String())
	}
	if len(actors) != 1 || actors[0] != "console" {
		t.Fatalf("actors=%#v", actors)
	}
}

func TestAccountMaintenanceRoutesPassCurrentVisibleStableIDs(t *testing.T) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	task := taskstore.Task{ID: "maintenance-1", Skill: "sub2api-operations", Operation: "account-binding-revalidation",
		Status: "queued", Progress: 0, Message: "已排队", Result: map[string]any{}, CreatedAt: now, UpdatedAt: now}
	revalidations, baseURLs, configurationChecks, baseURLRepairs, hosts, repairs, cleanups, rates, defaults := [][]string{}, [][]string{}, [][]string{}, [][]string{}, [][]string{}, [][]string{}, [][]string{}, [][]string{}, [][]string{}
	router, _ := testRouterWithDependencies(t, config.Config{AdminToken: "test-token"}, fakeBusiness{mode: "完全模式"}, Dependencies{
		AccountMaintenance: fakeAccountMaintenanceTasks{task: task, revalidate: &revalidations, baseURLs: &baseURLs, configurationChecks: &configurationChecks, baseURLRepairs: &baseURLRepairs, hosts: &hosts, repair: &repairs, cleanup: &cleanups, rates: &rates, defaults: &defaults},
	})
	for _, path := range []string{"/api/management/accounts/rates/sync", "/api/management/accounts/revalidate", "/api/management/accounts/base-url/validate", "/api/management/accounts/configuration/check", "/api/management/accounts/base-url/repair", "/api/management/accounts/upstream-hosts/repair", "/api/management/accounts/names/repair", "/api/management/accounts/defaults/repair", "/api/management/accounts/missing-bindings/cleanup"} {
		response := authenticatedRequest(t, router, http.MethodPost, path, map[string]any{"account_ids": []string{"11", "12"}})
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"id":"maintenance-1"`) {
			t.Fatalf("%s response=%d %s", path, response.Code, response.Body.String())
		}
	}
	if !reflect.DeepEqual(rates, [][]string{{"11", "12"}}) {
		t.Fatalf("rates=%#v", rates)
	}
	if !reflect.DeepEqual(baseURLs, [][]string{{"11", "12"}}) {
		t.Fatalf("baseURLs=%#v", baseURLs)
	}
	if !reflect.DeepEqual(configurationChecks, [][]string{{"11", "12"}}) {
		t.Fatalf("configurationChecks=%#v", configurationChecks)
	}
	if !reflect.DeepEqual(baseURLRepairs, [][]string{{"11", "12"}}) {
		t.Fatalf("baseURLRepairs=%#v", baseURLRepairs)
	}
	if !reflect.DeepEqual(hosts, [][]string{{"11", "12"}}) {
		t.Fatalf("hosts=%#v", hosts)
	}
	if !reflect.DeepEqual(defaults, [][]string{{"11", "12"}}) {
		t.Fatalf("defaults=%#v", defaults)
	}
	if !reflect.DeepEqual(revalidations, [][]string{{"11", "12"}}) || !reflect.DeepEqual(repairs, [][]string{{"11", "12"}}) || !reflect.DeepEqual(cleanups, [][]string{{"11", "12"}}) {
		t.Fatalf("revalidations=%#v repairs=%#v cleanups=%#v", revalidations, repairs, cleanups)
	}
	invalid := authenticatedRequest(t, router, http.MethodPost, "/api/management/accounts/revalidate", map[string]any{"account_ids": []string{"011"}})
	if invalid.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid stable ID response=%d %s", invalid.Code, invalid.Body.String())
	}
}

func TestUpstreamSyncRoutesPreserveScopeHostAndOperation(t *testing.T) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	task := taskstore.Task{
		ID: "upstream-task-1", Skill: "sub2api-upstream-info", Operation: "upstream-sync",
		Status: "queued", Progress: 0, Message: "上游同步已排队", Result: map[string]any{}, CreatedAt: now, UpdatedAt: now,
	}
	allCalls, hostCalls := []upstreamSyncCall{}, []upstreamSyncCall{}
	router, _ := testRouterWithDependencies(t, config.Config{AdminToken: "test-token"}, fakeBusiness{mode: "完全模式"}, Dependencies{
		UpstreamSync: fakeUpstreamSyncTasks{task: task, allCalls: &allCalls, hostCalls: &hostCalls},
	})
	for _, path := range []string{"/api/upstreams/balances/sync", "/api/upstreams/groups/sync", "/api/upstreams/sync", "/api/upstreams/names/repair"} {
		response := authenticatedRequest(t, router, http.MethodPost, path, nil)
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"id":"upstream-task-1"`) {
			t.Fatalf("%s response: %d %s", path, response.Code, response.Body.String())
		}
	}
	if len(allCalls) != 4 || allCalls[0].scope != (upstreamsync.Scope{Balance: true}) ||
		allCalls[1].scope != (upstreamsync.Scope{Catalog: true}) ||
		allCalls[2].scope != (upstreamsync.Scope{Catalog: true, Balance: true}) ||
		allCalls[3].scope != (upstreamsync.Scope{Name: true}) || allCalls[3].operation != "upstream-name-repair" {
		t.Fatalf("all calls=%#v", allCalls)
	}
	balance := authenticatedRequest(t, router, http.MethodPost, "/api/upstreams/API.EXAMPLE/balance-sync", nil)
	rate := authenticatedRequest(t, router, http.MethodPost, "/api/upstreams/api.example/rate-sync", map[string]any{"host": "https://API.EXAMPLE/", "key_id": "17"})
	if balance.Code != http.StatusOK || rate.Code != http.StatusOK || len(hostCalls) != 2 {
		t.Fatalf("balance=%d rate=%d calls=%#v", balance.Code, rate.Code, hostCalls)
	}
	if hostCalls[0].host != "API.EXAMPLE" || hostCalls[0].scope != (upstreamsync.Scope{Balance: true}) ||
		!hostCalls[1].scope.Catalog || !hostCalls[1].scope.Balance || hostCalls[1].scope.KeyID == nil || *hostCalls[1].scope.KeyID != "17" || hostCalls[1].operation != "rate-sync" {
		t.Fatalf("host calls=%#v", hostCalls)
	}
	mismatch := authenticatedRequest(t, router, http.MethodPost, "/api/upstreams/api.example/rate-sync", map[string]any{"host": "other.example"})
	if mismatch.Code != http.StatusUnprocessableEntity || len(hostCalls) != 2 {
		t.Fatalf("mismatch=%d calls=%#v", mismatch.Code, hostCalls)
	}
}

func TestUpstreamSyncRoutesRejectInvalidRuntimeMode(t *testing.T) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	task := taskstore.Task{
		ID: "upstream-task-1", Skill: "sub2api-upstream-info", Operation: "upstream-sync",
		Status: "queued", Progress: 0, Message: "上游同步已排队", Result: map[string]any{}, CreatedAt: now, UpdatedAt: now,
	}
	allCalls, hostCalls := []upstreamSyncCall{}, []upstreamSyncCall{}
	router, _ := testRouterWithDependencies(t, config.Config{AdminToken: "test-token"}, fakeBusiness{mode: "配置错误"}, Dependencies{
		UpstreamSync: fakeUpstreamSyncTasks{task: task, allCalls: &allCalls, hostCalls: &hostCalls},
	})
	response := authenticatedRequest(t, router, http.MethodPost, "/api/upstreams/sync", nil)
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "运行模式无效：配置错误") || len(allCalls) != 0 {
		t.Fatalf("invalid mode sync response=%d %s calls=%#v", response.Code, response.Body.String(), allCalls)
	}
	host := authenticatedRequest(t, router, http.MethodPost, "/api/upstreams/api.example/balance-sync", nil)
	if host.Code != http.StatusConflict || len(hostCalls) != 0 {
		t.Fatalf("invalid mode host sync response=%d %s calls=%#v", host.Code, host.Body.String(), hostCalls)
	}
}

func TestHealthRejectsInvalidRuntimeMode(t *testing.T) {
	router, _ := testRouter(t, config.Config{AdminToken: "test-token"}, fakeBusiness{mode: "配置错误"})
	response := authenticatedRequest(t, router, http.MethodGet, "/api/health", nil)
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "运行模式无效：配置错误") {
		t.Fatalf("invalid mode health response=%d %s", response.Code, response.Body.String())
	}
}

func TestAuthRecoveryRoutesUseTypedManualCredentialsAndSelectedVaultEntry(t *testing.T) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	task := taskstore.Task{
		ID: "auth-task-1", Skill: "sub2api-upstream-auth", Operation: "recover-host", Status: "queued", Progress: 0,
		Message: "鉴权恢复已排队", Result: map[string]any{}, CreatedAt: now, UpdatedAt: now,
	}
	manualCalls := []authrecovery.ManualInput{}
	runCalls := []upstreamSyncCall{}
	agreementCalls := []bool{}
	batchCalls := [][]string{}
	captchaSubmits := []upstreamSyncCall{}
	captchaCancels := []string{}
	balance := authrecovery.BalanceResult{Status: "succeeded", BalanceStatus: "已读取"}
	service := fakeAuthRecovery{
		manual: authrecovery.ManualResult{Host: "api.example", Verified: true, BalanceSync: &balance},
		task:   task, manualCalls: &manualCalls, runCalls: &runCalls, agreementCalls: &agreementCalls, batchCalls: &batchCalls,
		captchaSubmits: &captchaSubmits, captchaCancels: &captchaCancels,
		captchaCompletion: authrecovery.CaptchaCompletion{CaptchaResult: authrecovery.CaptchaResult{
			Success: true, Host: "api.example", ProfileStatus: "verified", Stored: true, InteractionKind: "image_captcha_ocr",
		}},
	}
	router, _ := testRouterWithDependencies(t, config.Config{AdminToken: "test-token"}, fakeBusiness{mode: "完全模式"}, Dependencies{AuthRecovery: service})
	rows := authenticatedRequest(t, router, http.MethodGet, "/api/auth-recovery?limit=10", nil)
	if rows.Code != http.StatusNotFound {
		t.Fatalf("retired auth recovery history route=%d %s", rows.Code, rows.Body.String())
	}
	manual := authenticatedRequest(t, router, http.MethodPost, "/api/auth-recovery/manual", map[string]any{
		"host": "api.example", "auth_mode": "newapi_admin_key", "admin_key": "admin", "user_id": "7",
		"accept_login_agreement": true,
		"headers":                map[string]any{"X-CF-Access": "signed-header"},
	})
	if manual.Code != http.StatusOK || len(manualCalls) != 1 || manualCalls[0].AdminKey == nil || *manualCalls[0].AdminKey != "admin" || !manualCalls[0].AcceptLoginAgreement || !manualCalls[0].Present["admin_key"] || !manualCalls[0].Present["headers"] || manualCalls[0].Headers["X-CF-Access"] != "signed-header" {
		t.Fatalf("manual=%d %s calls=%#v", manual.Code, manual.Body.String(), manualCalls)
	}
	run := authenticatedRequest(t, router, http.MethodPost, "/api/auth-recovery/run", map[string]any{"host": "api.example", "entry": "Selected", "accept_login_agreement": true})
	if run.Code != http.StatusOK || len(runCalls) != 1 || runCalls[0].host != "api.example" || runCalls[0].operation != "Selected" || len(agreementCalls) != 1 || !agreementCalls[0] {
		t.Fatalf("run=%d %s calls=%#v", run.Code, run.Body.String(), runCalls)
	}
	batch := authenticatedRequest(t, router, http.MethodPost, "/api/auth-recovery/run-batch", map[string]any{"hosts": []string{"ONE.EXAMPLE", "two.example"}})
	if batch.Code != http.StatusOK || len(batchCalls) != 1 || !slices.Equal(batchCalls[0], []string{"one.example", "two.example"}) {
		t.Fatalf("batch=%d %s calls=%#v", batch.Code, batch.Body.String(), batchCalls)
	}
	submit := authenticatedRequest(t, router, http.MethodPost, "/api/auth-recovery/captcha/submit", map[string]any{"challenge_id": "challenge-1", "captcha_code": "AB12"})
	if submit.Code != http.StatusOK || len(captchaSubmits) != 1 || captchaSubmits[0].host != "challenge-1" || captchaSubmits[0].operation != "AB12" {
		t.Fatalf("submit=%d %s calls=%#v", submit.Code, submit.Body.String(), captchaSubmits)
	}
	cancel := authenticatedRequest(t, router, http.MethodPost, "/api/auth-recovery/captcha/cancel", map[string]any{"challenge_id": "challenge-1"})
	if cancel.Code != http.StatusOK || len(captchaCancels) != 1 || captchaCancels[0] != "challenge-1" || !strings.Contains(cancel.Body.String(), `"cancelled":true`) {
		t.Fatalf("cancel=%d %s calls=%#v", cancel.Code, cancel.Body.String(), captchaCancels)
	}
	invalid := authenticatedRequest(t, router, http.MethodPost, "/api/auth-recovery/manual", map[string]any{"host": "api.example", "unexpected": true})
	if invalid.Code != http.StatusUnprocessableEntity || len(manualCalls) != 1 {
		t.Fatalf("invalid=%d calls=%#v", invalid.Code, manualCalls)
	}
}

func TestAccountFieldMutationPreservesTypedPayloadsAndRetiresLegacyRoutes(t *testing.T) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	task := taskstore.Task{
		ID: "account-task-1", Skill: "sub2api-account-sync", Operation: "account-fields-sync",
		Status: "queued", Progress: 0, Message: "账号操作已排队", Result: map[string]any{},
		CreatedAt: now, UpdatedAt: now,
	}
	fieldsCalls := []fieldsCall{}
	accountID := "41"
	accountName := "alpha"
	account := &business.AccountDetail{AccountStatus: business.AccountStatus{
		ID: accountID, Name: accountName, Groups: []string{}, Health: "healthy", RecentResults: []business.AccountRecentResult{},
	}}
	router, private := testRouterWithDependencies(t, config.Config{AdminToken: "test-token"}, fakeBusiness{
		mode: "完全模式", accountDetail: account,
	}, Dependencies{AccountTasks: fakeAccountTasks{
		task: task, fieldsCalls: &fieldsCalls,
	}})
	if err := private.Initialize(context.Background(), "operator", "correct password", "https://sub2api.example", "admin-key"); err != nil {
		t.Fatal(err)
	}

	fields := authenticatedRequest(t, router, http.MethodPost, "/api/accounts/41/sync", map[string]any{
		"priority": 120, "load_factor": "2.5", "concurrency": 3000,
		"upstream_host": "https://new-upstream.example.test", "base_url": "https://account-api.example.test/v1", "notes": nil,
	})
	if fields.Code != http.StatusOK || len(fieldsCalls) != 1 {
		t.Fatalf("unexpected fields response: %d %s calls=%#v", fields.Code, fields.Body.String(), fieldsCalls)
	}
	patch := fieldsCalls[0].patch
	if patch.NamePresent || !patch.PriorityPresent || patch.Priority == nil || *patch.Priority != 120 ||
		!patch.LoadFactorPresent || patch.LoadFactor == nil || *patch.LoadFactor != "2.5" ||
		!patch.ConcurrencyPresent || patch.Concurrency == nil || *patch.Concurrency != 3000 ||
		!patch.UpstreamHostPresent || patch.UpstreamHost == nil || *patch.UpstreamHost != "new-upstream.example.test" ||
		!patch.BaseURLPresent || patch.BaseURL == nil || *patch.BaseURL != "https://account-api.example.test/v1" ||
		patch.MultiplierPresent || patch.Multiplier != nil || !patch.NotesPresent || patch.Notes != nil {
		t.Fatalf("field presence/null semantics lost: %#v", patch)
	}

	invalidBaseURL := authenticatedRequest(t, router, http.MethodPost, "/api/accounts/41/sync", map[string]any{
		"base_url": "account-api.example.test/v1",
	})
	if invalidBaseURL.Code != http.StatusUnprocessableEntity || len(fieldsCalls) != 1 {
		t.Fatalf("invalid Base URL accepted: %d %s", invalidBaseURL.Code, invalidBaseURL.Body.String())
	}

	models := authenticatedRequest(t, router, http.MethodGet, "/api/accounts/41/models", nil)
	if models.Code != http.StatusOK || !strings.Contains(models.Body.String(), `"gpt-5.1-codex"`) {
		t.Fatalf("unexpected models response: %d %s", models.Code, models.Body.String())
	}
	testModel := authenticatedRequest(t, router, http.MethodPut, "/api/accounts/41/test-model", map[string]any{"model": "gpt-5.1-codex"})
	if testModel.Code != http.StatusOK || !strings.Contains(testModel.Body.String(), `"saved":true`) {
		t.Fatalf("unexpected test model response: %d %s", testModel.Code, testModel.Body.String())
	}

	unknown := authenticatedRequest(t, router, http.MethodPost, "/api/accounts/41/sync", map[string]any{"website": "invalid"})
	manualMultiplier := authenticatedRequest(t, router, http.MethodPost, "/api/accounts/41/sync", map[string]any{"multiplier": "1.5"})
	nullName := authenticatedRequest(t, router, http.MethodPost, "/api/accounts/41/sync", map[string]any{"name": nil})
	leadingZero := authenticatedRequest(t, router, http.MethodPost, "/api/accounts/041/control", map[string]any{"action": "pause"})
	for label, response := range map[string]*httptest.ResponseRecorder{
		"unknown": unknown, "manual multiplier": manualMultiplier, "null name": nullName, "leading zero": leadingZero,
	} {
		if response.Code != http.StatusUnprocessableEntity {
			t.Fatalf("%s status=%d body=%s", label, response.Code, response.Body.String())
		}
	}
	if len(fieldsCalls) != 1 {
		t.Fatalf("invalid requests reached task service: fields=%d", len(fieldsCalls))
	}
	for _, path := range []string{"/api/accounts/41/scheduling", "/api/accounts/41/groups"} {
		response := authenticatedRequest(t, router, http.MethodPost, path, map[string]any{})
		if response.Code != http.StatusNotFound {
			t.Fatalf("retired account route %s status=%d", path, response.Code)
		}
	}
}

func TestUpstreamInputKeepsHostAddressAndAccountBaseURLIndependent(t *testing.T) {
	input, err := parseUpstreamInput(map[string]any{
		"base_url":         "https://upstream.example/admin",
		"account_base_url": "https://account-api.example/v1",
		"upstream_type":    "sub2api",
		"auth_mode":        "sub2api_user_login",
		"recharge_rate":    "1",
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	if input.BaseURL != "https://upstream.example/admin" || input.AccountBaseURL != "https://account-api.example/v1" {
		t.Fatalf("upstream connection fields were coupled: %#v", input)
	}

	legacy, err := parseUpstreamInput(map[string]any{
		"base_url":      "https://legacy-upstream.example",
		"upstream_type": "sub2api",
		"auth_mode":     "sub2api_user_login",
		"recharge_rate": "1",
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	if legacy.AccountBaseURL != legacy.BaseURL {
		t.Fatalf("legacy account Base URL did not default to Host address: %#v", legacy)
	}
}

func TestAccountDeleteUsesPreviewStableBindingAndKeyIDs(t *testing.T) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	preview := accountdelete.Preview{
		AccountID: "37", AccountName: "special-key", Groups: []string{"special"},
		ManagementBaseURL: "https://management.example.test",
		Binding: &accountdelete.Binding{
			ID: 91, UpstreamID: "upstream-1", UpstreamHost: "https://upstream.example.test",
			AuthHost: "upstream.example.test", UpstreamKeyID: "key-8", UpstreamKeyName: "special-key",
		},
	}
	task := taskstore.Task{
		ID: "account-delete-task", Skill: "sub2api-account-management", Operation: "account-delete",
		Status: "queued", Progress: 0, Message: "账号双端删除已排队", Result: map[string]any{}, CreatedAt: now, UpdatedAt: now,
	}
	calls := []accountDeleteCall{}
	router, private := testRouterWithDependencies(t, config.Config{AdminToken: "test-token"}, fakeBusiness{mode: "完全模式"}, Dependencies{
		AccountDelete: fakeAccountDelete{preview: preview, task: task, calls: &calls},
	})
	if err := private.Initialize(context.Background(), "operator", "correct password", "https://sub2api.example", "admin-key"); err != nil {
		t.Fatal(err)
	}
	read := authenticatedRequest(t, router, http.MethodGet, "/api/accounts/37/delete-preview", nil)
	if read.Code != http.StatusOK || !strings.Contains(read.Body.String(), `"upstream_key_id":"key-8"`) {
		t.Fatalf("unexpected preview response: %d %s", read.Code, read.Body.String())
	}
	deleted := authenticatedRequest(t, router, http.MethodPost, "/api/accounts/37/delete", map[string]any{
		"confirmation_account_id": "37", "expected_binding_id": 91,
		"expected_upstream_id":   "upstream-1",
		"expected_upstream_host": "https://upstream.example.test", "expected_auth_host": "upstream.example.test",
		"expected_upstream_key_id": "key-8", "expected_management_base_url": "https://management.example.test",
	})
	if deleted.Code != http.StatusOK || len(calls) != 1 {
		t.Fatalf("unexpected delete response: %d %s calls=%#v", deleted.Code, deleted.Body.String(), calls)
	}
	if calls[0].accountID != "37" || calls[0].binding.ID != 91 || calls[0].binding.UpstreamID != "upstream-1" ||
		calls[0].binding.UpstreamHost != "https://upstream.example.test" ||
		calls[0].binding.AuthHost != "upstream.example.test" || calls[0].binding.UpstreamKeyID != "key-8" ||
		calls[0].managementBaseURL != "https://management.example.test" || calls[0].actor != "console" {
		t.Fatalf("delete scope changed: %#v", calls[0])
	}
	mismatch := authenticatedRequest(t, router, http.MethodPost, "/api/accounts/37/delete", map[string]any{
		"confirmation_account_id": "38", "expected_binding_id": 91,
		"expected_upstream_id":   "upstream-1",
		"expected_upstream_host": "https://upstream.example.test", "expected_auth_host": "upstream.example.test",
		"expected_upstream_key_id": "key-8", "expected_management_base_url": "https://management.example.test",
	})
	if mismatch.Code != http.StatusUnprocessableEntity || len(calls) != 1 {
		t.Fatalf("mismatched confirmation was accepted: %d calls=%#v", mismatch.Code, calls)
	}
	missingIdentity := authenticatedRequest(t, router, http.MethodPost, "/api/accounts/37/delete", map[string]any{
		"confirmation_account_id": "37", "expected_binding_id": 91,
		"expected_upstream_host": "https://upstream.example.test", "expected_auth_host": "upstream.example.test",
		"expected_upstream_key_id": "key-8", "expected_management_base_url": "https://management.example.test",
	})
	if missingIdentity.Code != http.StatusUnprocessableEntity || len(calls) != 1 {
		t.Fatalf("missing stable upstream identity was accepted: %d calls=%#v", missingIdentity.Code, calls)
	}
}

func TestAccountDeleteAcceptsManagementOnlyScopeWhenPreviewHasNoBinding(t *testing.T) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	preview := accountdelete.Preview{
		AccountID: "174", AccountName: "星筱AI-0.125", Groups: []string{},
		ManagementBaseURL: "https://management.example.test", Binding: nil,
	}
	task := taskstore.Task{
		ID: "management-account-delete-task", Skill: "sub2api-account-management", Operation: "account-delete",
		Status: "queued", Progress: 0, Message: "管理平台账号删除已排队", Result: map[string]any{}, CreatedAt: now, UpdatedAt: now,
	}
	calls := []accountDeleteCall{}
	router, private := testRouterWithDependencies(t, config.Config{AdminToken: "test-token"}, fakeBusiness{mode: "完全模式"}, Dependencies{
		AccountDelete: fakeAccountDelete{preview: preview, task: task, calls: &calls},
	})
	if err := private.Initialize(context.Background(), "operator", "correct password", "https://sub2api.example", "admin-key"); err != nil {
		t.Fatal(err)
	}
	read := authenticatedRequest(t, router, http.MethodGet, "/api/accounts/174/delete-preview", nil)
	if read.Code != http.StatusOK || !strings.Contains(read.Body.String(), `"binding":null`) {
		t.Fatalf("unexpected preview response: %d %s", read.Code, read.Body.String())
	}
	deleted := authenticatedRequest(t, router, http.MethodPost, "/api/accounts/174/delete", map[string]any{
		"confirmation_account_id": "174", "expected_management_base_url": "https://management.example.test",
	})
	if deleted.Code != http.StatusOK || len(calls) != 1 || calls[0].binding != nil || calls[0].accountID != "174" {
		t.Fatalf("unexpected management-only delete: %d %s calls=%#v", deleted.Code, deleted.Body.String(), calls)
	}
	partialBinding := authenticatedRequest(t, router, http.MethodPost, "/api/accounts/174/delete", map[string]any{
		"confirmation_account_id": "174", "expected_management_base_url": "https://management.example.test",
		"expected_upstream_key_id": "key-unknown",
	})
	if partialBinding.Code != http.StatusUnprocessableEntity || len(calls) != 1 {
		t.Fatalf("partial binding scope was accepted: %d calls=%#v", partialBinding.Code, calls)
	}
}

func TestAccountBatchDeleteRequiresPreviewScopesAndPassesStableConfirmations(t *testing.T) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	previews := []accountdelete.Preview{
		{
			AccountID: "37", AccountName: "bound", Groups: []string{"codex"},
			ManagementBaseURL: "https://management.example.test",
			Binding: &accountdelete.Binding{
				ID: 91, UpstreamID: "upstream-1", UpstreamHost: "upstream.example.test",
				AuthHost: "auth.example.test", UpstreamKeyID: "key-8", UpstreamKeyName: "key",
			},
		},
		{AccountID: "38", AccountName: "unbound", ManagementBaseURL: "https://management.example.test"},
	}
	task := taskstore.Task{
		ID: "account-delete-batch", Skill: "sub2api-account-management", Operation: "account-delete-batch",
		Status: "queued", Message: "2 个账号批量删除已排队", Result: map[string]any{}, CreatedAt: now, UpdatedAt: now,
	}
	calls := [][]accountdelete.Confirmation{}
	router, _ := testRouterWithDependencies(t, config.Config{AdminToken: "test-token"}, fakeBusiness{mode: "完全模式"}, Dependencies{
		AccountDelete: fakeAccountDelete{
			batchPreview: accountdelete.BatchPreview{Accounts: previews, AccountCount: 2, UpstreamKeyCount: 1},
			task:         task, batchConfirmations: &calls,
		},
	})
	read := authenticatedRequest(t, router, http.MethodPost, "/api/accounts/delete-preview", map[string]any{
		"account_ids": []string{"37", "38"},
	})
	if read.Code != http.StatusOK || !strings.Contains(read.Body.String(), `"upstream_key_count":1`) {
		t.Fatalf("unexpected batch preview response: %d %s", read.Code, read.Body.String())
	}
	deleted := authenticatedRequest(t, router, http.MethodPost, "/api/accounts/delete", map[string]any{
		"confirmations": []map[string]any{
			{
				"account_id": "37", "management_base_url": "https://management.example.test",
				"binding": map[string]any{
					"id": 91, "upstream_id": "upstream-1", "upstream_host": "upstream.example.test",
					"auth_host": "auth.example.test", "upstream_key_id": "key-8", "upstream_key_name": "key",
				},
			},
			{"account_id": "38", "management_base_url": "https://management.example.test", "binding": nil},
		},
	})
	if deleted.Code != http.StatusOK || len(calls) != 1 || len(calls[0]) != 2 ||
		calls[0][0].Binding == nil || calls[0][0].Binding.UpstreamKeyID != "key-8" || calls[0][1].Binding != nil {
		t.Fatalf("unexpected batch delete response: %d %s calls=%#v", deleted.Code, deleted.Body.String(), calls)
	}
	invalid := authenticatedRequest(t, router, http.MethodPost, "/api/accounts/delete-preview", map[string]any{
		"account_ids": []string{"37", "37"},
	})
	if invalid.Code != http.StatusUnprocessableEntity {
		t.Fatalf("duplicate batch preview IDs accepted: %d %s", invalid.Code, invalid.Body.String())
	}
}

func TestAccountMutationReturnsNotFoundBeforeQueuing(t *testing.T) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	calls := []fieldsCall{}
	router, private := testRouterWithDependencies(t, config.Config{AdminToken: "test-token"}, fakeBusiness{mode: "完全模式"}, Dependencies{
		AccountTasks: fakeAccountTasks{
			task:        taskstore.Task{ID: "unused", Skill: "account", Operation: "fields", Status: "queued", Progress: 0, Message: "queued", Result: map[string]any{}, CreatedAt: now, UpdatedAt: now},
			fieldsCalls: &calls,
		},
	})
	if err := private.Initialize(context.Background(), "operator", "correct password", "https://sub2api.example", "admin-key"); err != nil {
		t.Fatal(err)
	}
	response := authenticatedRequest(t, router, http.MethodPost, "/api/accounts/99/sync", map[string]any{"priority": 10})
	if response.Code != http.StatusNotFound || len(calls) != 0 {
		t.Fatalf("response=%d %s calls=%#v", response.Code, response.Body.String(), calls)
	}
}

func TestManualPriorityRoutesAssignAndClearWithoutInspection(t *testing.T) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	account := &business.AccountDetail{AccountStatus: business.AccountStatus{
		ID: "41", Name: "alpha", Groups: []string{"codex"}, Health: "healthy", RecentResults: []business.AccountRecentResult{},
	}}
	manualCalls, clearCalls := []string{}, []string{}
	manualTask := taskstore.Task{ID: "manual-1", Skill: "manual", Operation: "assign", Status: "queued", Progress: 0, Message: "queued", Result: map[string]any{}, CreatedAt: now, UpdatedAt: now}
	router, private := testRouterWithDependencies(t, config.Config{AdminToken: "test-token"}, fakeBusiness{
		mode: "完全模式", accountDetail: account,
	}, Dependencies{
		AccountTasks: fakeAccountTasks{task: manualTask, manualCalls: &manualCalls, clearCalls: &clearCalls},
	})
	if err := private.Initialize(context.Background(), "operator", "correct password", "https://sub2api.example", "admin-key"); err != nil {
		t.Fatal(err)
	}
	assigned := authenticatedRequest(t, router, http.MethodPut, "/api/accounts/41/manual-priority", map[string]any{"priority": 3, "load_factor": "100", "concurrency": 100, "sync_balance_multiplier": true})
	if assigned.Code != http.StatusOK || len(manualCalls) != 1 || manualCalls[0] != "41:3:100:100:true:console" {
		t.Fatalf("assign response=%d %s calls=%#v", assigned.Code, assigned.Body.String(), manualCalls)
	}
	cleared := authenticatedRequest(t, router, http.MethodDelete, "/api/accounts/41/manual-priority", nil)
	if cleared.Code != http.StatusOK || !strings.Contains(cleared.Body.String(), `"id":"manual-1"`) || len(clearCalls) != 1 || clearCalls[0] != "41:console" {
		t.Fatalf("clear response=%d %s clear=%#v", cleared.Code, cleared.Body.String(), clearCalls)
	}
	invalid := authenticatedRequest(t, router, http.MethodPut, "/api/accounts/41/manual-priority", map[string]any{"priority": 0, "load_factor": "100", "concurrency": 100, "sync_balance_multiplier": false})
	if invalid.Code != http.StatusUnprocessableEntity || len(manualCalls) != 1 {
		t.Fatalf("invalid response=%d %s calls=%#v", invalid.Code, invalid.Body.String(), manualCalls)
	}
}

func TestAccountControlQueuesDedicatedTaskWithoutInspection(t *testing.T) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	accountID := "41"
	account := &business.AccountDetail{AccountStatus: business.AccountStatus{
		ID: accountID, Name: "alpha", Groups: []string{"codex"}, Health: "healthy", RecentResults: []business.AccountRecentResult{},
	}}
	controlCalls := []string{}
	router, private := testRouterWithDependencies(t, config.Config{AdminToken: "test-token"}, fakeBusiness{
		mode: "完全模式", accountDetail: account,
	}, Dependencies{AccountTasks: fakeAccountTasks{
		task:         taskstore.Task{ID: "control-1", Skill: "account-control", Operation: "account-control", Status: "queued", Result: map[string]any{}, CreatedAt: now, UpdatedAt: now},
		controlCalls: &controlCalls,
	}})
	if err := private.Initialize(context.Background(), "operator", "correct password", "https://sub2api.example", "admin-key"); err != nil {
		t.Fatal(err)
	}
	response := authenticatedRequest(t, router, http.MethodPost, "/api/accounts/41/control", map[string]any{"action": "pause"})
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"id":"control-1"`) {
		t.Fatalf("control response=%d %s", response.Code, response.Body.String())
	}
	if len(controlCalls) != 1 || controlCalls[0] != "41:pause:console" {
		t.Fatalf("control=%#v", controlCalls)
	}
	invalid := authenticatedRequest(t, router, http.MethodPost, "/api/accounts/41/control", map[string]any{"action": "delete"})
	if invalid.Code != http.StatusUnprocessableEntity || len(controlCalls) != 1 {
		t.Fatalf("invalid control=%d calls=%#v", invalid.Code, controlCalls)
	}
}

func TestGroupAndUpstreamReadContracts(t *testing.T) {
	groupID := "1"
	rawRate := "1"
	router, _ := testRouter(t, config.Config{AdminToken: "test-token"}, fakeBusiness{
		mode: "完全模式",
		groupRows: []business.GroupStatus{{
			Name: "codex", ID: &groupID, Strategy: "balanced", StrategySource: "global_default",
			ParticipationStatus: "participating", Status: "healthy",
		}},
		groupAllocation: business.GroupAllocation{
			GroupID: "1", GroupName: "codex", AccountCount: 1, AssignedConcurrency: 32,
			Channels: []business.GroupAllocationChannel{{AccountID: "41", AccountName: "alpha"}},
		},
		groupProbeModels: business.GroupProbeModels{
			GroupID: "1", GroupName: "codex", Models: []string{"gpt-5.2"},
			AccountCount: 1, AccountsWithModels: 1, Complete: true,
		},
		upstreamSummary: business.UpstreamSummary{
			Hosts: []business.UpstreamHost{{
				Host: "api.example", BaseURL: "https://api.example", Name: "Example",
				UpstreamType: "newapi", AuthStatus: "已鉴权", RechargeRate: "1", BalanceStatus: "已读取",
			}},
			TotalHosts: 1, AuthenticatedHosts: 1, Source: "Console 业务库",
		},
		upstreamGroupRows: []business.UpstreamGroup{{
			Host: "api.example", GroupID: &groupID, Name: "codex", RawRate: &rawRate,
			EffectiveRate: &rawRate, RechargeRate: &rawRate, KeyPresent: true, Bindable: true,
		}},
		upstreamGroupHistory: []business.UpstreamGroupChange{{
			ID: 1, UpstreamID: "up_example", GroupID: "7", GroupName: "新分组", ChangeType: "added", ChangedAt: "2026-08-31T00:00:00Z",
		}},
	})

	groups := authenticatedRequest(t, router, http.MethodGet, "/api/groups", nil)
	if groups.Code != http.StatusOK || !strings.Contains(groups.Body.String(), `"strategy_source":"global_default"`) {
		t.Fatalf("unexpected groups response: %d %s", groups.Code, groups.Body.String())
	}
	allocation := authenticatedRequest(t, router, http.MethodGet, "/api/groups/1/allocation", nil)
	if allocation.Code != http.StatusOK || !strings.Contains(allocation.Body.String(), `"assigned_concurrency":32`) {
		t.Fatalf("unexpected group allocation response: %d %s", allocation.Code, allocation.Body.String())
	}
	models := authenticatedRequest(t, router, http.MethodGet, "/api/groups/1/models", nil)
	if models.Code != http.StatusOK || !strings.Contains(models.Body.String(), `"models":["gpt-5.2"]`) ||
		!strings.Contains(models.Body.String(), `"complete":true`) {
		t.Fatalf("unexpected group model response: %d %s", models.Code, models.Body.String())
	}
	upstreams := authenticatedRequest(t, router, http.MethodGet, "/api/upstreams", nil)
	if upstreams.Code != http.StatusOK || !strings.Contains(upstreams.Body.String(), `"authenticated_hosts":1`) {
		t.Fatalf("unexpected upstreams response: %d %s", upstreams.Code, upstreams.Body.String())
	}
	upstreamGroups := authenticatedRequest(t, router, http.MethodGet, "/api/upstreams/api.example/groups?include_bound=false", nil)
	if upstreamGroups.Code != http.StatusOK || !strings.Contains(upstreamGroups.Body.String(), `"bindable":true`) {
		t.Fatalf("unexpected upstream groups response: %d %s", upstreamGroups.Code, upstreamGroups.Body.String())
	}
	history := authenticatedRequest(t, router, http.MethodGet, "/api/upstreams/api.example/group-history?limit=50", nil)
	if history.Code != http.StatusOK || !strings.Contains(history.Body.String(), `"change_type":"added"`) || !strings.Contains(history.Body.String(), `"group_name":"新分组"`) {
		t.Fatalf("unexpected upstream group history response: %d %s", history.Code, history.Body.String())
	}
	invalid := authenticatedRequest(t, router, http.MethodGet, "/api/upstreams/api.example/groups?include_bound=perhaps", nil)
	if invalid.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid include_bound status = %d", invalid.Code)
	}
}

func TestUpstreamDetectionRouteUsesTypedPayload(t *testing.T) {
	urls := []string{}
	upstreamType := "newapi"
	authMode := "newapi_admin_key"
	detector := fakeUpstreamDetector{urls: &urls, result: upstreamdetect.Result{
		BaseURL: "https://api.example", Host: "api.example", UpstreamType: &upstreamType,
		AuthMode: &authMode, TypeDetected: true,
	}}
	router, _ := testRouterWithDependencies(t, config.Config{AdminToken: "test-token"}, fakeBusiness{mode: "完全模式"}, Dependencies{
		UpstreamDetect: detector,
	})
	response := authenticatedRequest(t, router, http.MethodPost, "/api/upstreams/detect", map[string]any{"base_url": "https://api.example/"})
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"upstream_type":"newapi"`) || len(urls) != 1 || urls[0] != "https://api.example/" {
		t.Fatalf("response=%d %s urls=%#v", response.Code, response.Body.String(), urls)
	}
	invalid := authenticatedRequest(t, router, http.MethodPost, "/api/upstreams/detect", map[string]any{"base_url": "x", "extra": true})
	if invalid.Code != http.StatusUnprocessableEntity || len(urls) != 1 {
		t.Fatalf("invalid response=%d %s urls=%#v", invalid.Code, invalid.Body.String(), urls)
	}
}

func TestUpstreamConfigurationRoutesPreserveExplicitNullPresence(t *testing.T) {
	created := []upstreamconfig.Input{}
	updated := []upstreamconfig.Input{}
	configuration := upstreamconfig.Configuration{
		Host: "api.example", Name: "Example", BaseURL: "https://api.example", UpstreamType: "sub2api",
		AuthMode: "sub2api_user_token", RechargeRate: "1", Headers: map[string]string{},
		HeaderNames: []string{}, CookieNames: []string{}, Groups: []business.UpstreamGroup{},
	}
	router, _ := testRouterWithDependencies(t, config.Config{AdminToken: "test-token"}, fakeBusiness{mode: "完全模式"}, Dependencies{
		UpstreamConfigs: fakeUpstreamConfigurations{configuration: configuration, created: &created, updated: &updated},
	})
	payload := map[string]any{
		"host": "origin.example:8080", "name": "Example", "base_url": "https://accelerated.example:8443/api", "upstream_type": "sub2api",
		"auth_mode": "sub2api_user_token", "recharge_rate": "1", "access_token": nil, "refresh_token": "refresh",
		"headers": map[string]any{"X-Site": "one"},
	}
	response := authenticatedRequest(t, router, http.MethodPost, "/api/upstreams", payload)
	if response.Code != http.StatusOK || len(created) != 1 || created[0].Host != "origin.example:8080" || created[0].BaseURL != "https://accelerated.example:8443/api" || !created[0].Present["access_token"] || created[0].AccessToken != nil || created[0].Headers["X-Site"] != "one" {
		t.Fatalf("response=%d %s created=%#v", response.Code, response.Body.String(), created)
	}
	delete(payload, "host")
	delete(payload, "access_token")
	response = authenticatedRequest(t, router, http.MethodPut, "/api/upstreams/api.example/configuration", payload)
	if response.Code != http.StatusOK || len(updated) != 1 || updated[0].Present["access_token"] {
		t.Fatalf("response=%d %s updated=%#v", response.Code, response.Body.String(), updated)
	}
	read := authenticatedRequest(t, router, http.MethodGet, "/api/upstreams/api.example/configuration", nil)
	if read.Code != http.StatusOK || !strings.Contains(read.Body.String(), `"groups":[]`) {
		t.Fatalf("unexpected read: %d %s", read.Code, read.Body.String())
	}
	invalid := authenticatedRequest(t, router, http.MethodPost, "/api/upstreams", map[string]any{
		"host": "origin.example:8080", "name": "Example", "base_url": "https://accelerated.example:8443/api", "upstream_type": "sub2api",
		"auth_mode": "sub2api_user_token", "recharge_rate": "1", "unexpected": true,
	})
	if invalid.Code != http.StatusUnprocessableEntity || len(created) != 1 {
		t.Fatalf("unknown field reached service: %d %s", invalid.Code, invalid.Body.String())
	}
}

func TestOnboardingPreparationSynchronizesCatalogAndBalanceBeforeReturningCandidates(t *testing.T) {
	hostCalls := []upstreamSyncCall{}
	candidateHosts := []string{}
	groupID := "6"
	configuration := upstreamconfig.Configuration{
		UpstreamID: "up_example", Host: "api.example", Name: "Example", BaseURL: "https://api.example", UpstreamType: "sub2api",
		AuthMode: "sub2api_user_token", RechargeRate: "1", Headers: map[string]string{},
		HeaderNames: []string{}, CookieNames: []string{}, Groups: []business.UpstreamGroup{},
	}
	multiplier := "0.1"
	candidates := []business.OnboardingCandidate{{
		Number: 1, UpstreamID: "up_example", Host: "api.example", GroupID: &groupID, GroupName: "codex", Multiplier: &multiplier, Bindable: true,
	}}
	router, _ := testRouterWithDependencies(t, config.Config{AdminToken: "test-token"}, fakeBusiness{mode: "完全模式"}, Dependencies{
		UpstreamSync:    fakeUpstreamSyncTasks{hostCalls: &hostCalls},
		UpstreamConfigs: fakeUpstreamConfigurations{configuration: configuration},
		Onboarding:      fakeOnboarding{candidates: candidates, hosts: &candidateHosts},
	})
	response := authenticatedRequest(t, router, http.MethodPost, "/api/onboarding/prepare", map[string]any{"host": "api.example"})
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"group_name":"codex"`) ||
		!strings.Contains(response.Body.String(), `"upstream_id":"up_example"`) || !strings.Contains(response.Body.String(), `"host":"api.example"`) {
		t.Fatalf("prepare response=%d %s", response.Code, response.Body.String())
	}
	if len(hostCalls) != 1 || !hostCalls[0].scope.Catalog || !hostCalls[0].scope.Balance || hostCalls[0].actor != "console" || len(candidateHosts) != 1 || candidateHosts[0] != "api.example" {
		t.Fatalf("sync calls=%#v candidate hosts=%#v", hostCalls, candidateHosts)
	}
	reason := "token 已失效"
	failed := upstreamsync.HostResult{Host: "api.example", Status: "auth_failed", AuthStatus: "鉴权失效", BalanceStatus: "未读取", Reason: &reason}
	failedRouter, _ := testRouterWithDependencies(t, config.Config{AdminToken: "test-token"}, fakeBusiness{mode: "完全模式"}, Dependencies{
		UpstreamSync:    fakeUpstreamSyncTasks{hostResult: &failed},
		UpstreamConfigs: fakeUpstreamConfigurations{configuration: configuration},
		Onboarding:      fakeOnboarding{candidates: candidates},
	})
	failedResponse := authenticatedRequest(t, failedRouter, http.MethodPost, "/api/onboarding/prepare", map[string]any{"host": "api.example"})
	if failedResponse.Code != http.StatusConflict || !strings.Contains(failedResponse.Body.String(), reason) {
		t.Fatalf("failed prepare=%d %s", failedResponse.Code, failedResponse.Body.String())
	}
}

func TestOnboardingProbeEndpointsWorkWithoutALocalAccount(t *testing.T) {
	probeResult := onboarding.ProbeResult{
		Status: "passed", Message: "上游已返回成功响应", RequestModel: "gpt-5.2",
		ActualModel: "gpt-5.2", LatencyMS: 82, HTTPStatus: http.StatusOK, TemporaryKey: true,
	}
	router, _ := testRouterWithDependencies(t, config.Config{AdminToken: "test-token"}, fakeBusiness{mode: "完全模式"}, Dependencies{
		Onboarding: fakeOnboarding{models: []string{"gpt-5.2"}, probe: probeResult},
	})
	models := authenticatedRequest(t, router, http.MethodPost, "/api/onboarding/probe/models", map[string]any{
		"host": "api.example", "group_id": "6",
	})
	if models.Code != http.StatusOK || !strings.Contains(models.Body.String(), `"models":["gpt-5.2"]`) {
		t.Fatalf("models=%d %s", models.Code, models.Body.String())
	}
	probe := authenticatedRequest(t, router, http.MethodPost, "/api/onboarding/probe", map[string]any{
		"host": "api.example", "group_id": "6", "model": "gpt-5.2",
	})
	if probe.Code != http.StatusOK || !strings.Contains(probe.Body.String(), `"status":"passed"`) || !strings.Contains(probe.Body.String(), `"temporary_key":true`) {
		t.Fatalf("probe=%d %s", probe.Code, probe.Body.String())
	}
	cancel := authenticatedRequest(t, router, http.MethodPost, "/api/onboarding/probe/cancel", map[string]any{
		"host": "api.example", "group_id": "6",
	})
	if cancel.Code != http.StatusOK || cancel.Body.String() != `{"cancelled":true}` {
		t.Fatalf("cancel=%d %s", cancel.Code, cancel.Body.String())
	}
	invalid := authenticatedRequest(t, router, http.MethodPost, "/api/onboarding/probe", map[string]any{
		"host": "api.example", "group_id": "6", "model": "gpt-5.2", "account_id": "41",
	})
	if invalid.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid=%d %s", invalid.Code, invalid.Body.String())
	}
}

func TestOnboardingKeyCleanupRequiresPreviewAndStableKeyIDs(t *testing.T) {
	calls := []struct {
		host   string
		keyIDs []string
		actor  string
	}{}
	service := fakeOnboarding{
		task: taskstore.Task{ID: "cleanup-task", Operation: "upstream-key-cleanup", Status: "queued"},
		cleanupPreview: onboarding.KeyCleanupPreview{Host: "api.example", Keys: []onboarding.UnboundUpstreamKey{{
			KeyID: "key-17", Name: "unused-key",
		}}},
		cleanupCalls: &calls,
	}
	router, _ := testRouterWithDependencies(t, config.Config{AdminToken: "test-token"}, fakeBusiness{mode: "完全模式"}, Dependencies{
		Onboarding: service,
	})
	preview := authenticatedRequest(t, router, http.MethodPost, "/api/onboarding/keys/cleanup-preview", map[string]any{
		"host": "api.example",
	})
	if preview.Code != http.StatusOK || !strings.Contains(preview.Body.String(), `"key_id":"key-17"`) {
		t.Fatalf("preview=%d %s", preview.Code, preview.Body.String())
	}
	cleanup := authenticatedRequest(t, router, http.MethodPost, "/api/onboarding/keys/cleanup", map[string]any{
		"host": "api.example", "key_ids": []any{"key-17"},
	})
	if cleanup.Code != http.StatusOK || !strings.Contains(cleanup.Body.String(), `"id":"cleanup-task"`) || len(calls) != 1 ||
		calls[0].host != "api.example" || strings.Join(calls[0].keyIDs, ",") != "key-17" || calls[0].actor != "console" {
		t.Fatalf("cleanup=%d %s calls=%#v", cleanup.Code, cleanup.Body.String(), calls)
	}
	invalid := authenticatedRequest(t, router, http.MethodPost, "/api/onboarding/keys/cleanup", map[string]any{
		"host": "api.example", "key_ids": []any{""},
	})
	if invalid.Code != http.StatusUnprocessableEntity || len(calls) != 1 {
		t.Fatalf("invalid=%d %s calls=%#v", invalid.Code, invalid.Body.String(), calls)
	}
}

func TestPricingEndpointsExposeConfigSaveAndQueuedApply(t *testing.T) {
	service := &fakePricingService{snapshot: pricing.Snapshot{Config: pricing.Config{
		Enabled: false, ProfitMargin: 0.2, ExchangeGroupSets: [][]string{{"6", "7"}}, IntervalSeconds: 120, WriteConcurrency: 4,
	}, Accounts: 2, Changes: 1}, changes: []business.PricingChangeRecord{{
		ID: -1, Actor: "operator", CreatedAt: "2026-09-03T08:00:00Z", Changes: []business.PricingAccountChange{{
			AccountID: "41", AccountName: "account-41",
			Before: []business.PricingChangeGroup{{ID: "6", Name: "标准"}},
			After:  []business.PricingChangeGroup{{ID: "7", Name: "低价"}},
		}},
	}}}
	revenueTask := taskstore.Task{
		ID: "latest-revenue", Skill: "sub2api-billing-reconciliation", Operation: "revenue-calculation", Status: "succeeded",
		Progress: 100, Message: "完成", Result: map[string]any{"report_date": "2026-08-29"},
		CreatedAt: "2026-08-30T00:00:00Z", UpdatedAt: "2026-08-30T00:00:01Z",
	}
	router, _ := testRouterWithDependencies(t, config.Config{AdminToken: "test-token"}, fakeBusiness{mode: "完全模式"}, Dependencies{
		Pricing: service, Tasks: fakeTaskRepository{rows: []taskstore.Task{revenueTask}},
	})

	read := authenticatedRequest(t, router, http.MethodGet, "/api/pricing", nil)
	if read.Code != http.StatusOK || !strings.Contains(read.Body.String(), `"profit_margin":0.2`) || !strings.Contains(read.Body.String(), `"enabled":false`) {
		t.Fatalf("read=%d %s", read.Code, read.Body.String())
	}
	changes := authenticatedRequest(t, router, http.MethodGet, "/api/pricing/changes", nil)
	if changes.Code != http.StatusOK || !strings.Contains(changes.Body.String(), `"account_name":"account-41"`) ||
		!strings.Contains(changes.Body.String(), `"actor":"operator"`) {
		t.Fatalf("changes=%d %s", changes.Code, changes.Body.String())
	}
	saved := authenticatedRequest(t, router, http.MethodPut, "/api/pricing/config", map[string]any{
		"enabled": true, "profit_margin": 0.25, "exchange_group_sets": []any{[]any{"6", "7"}},
		"exchange_group_set_names": []any{"Codex 规则"}, "interval_seconds": 300, "write_concurrency": 2,
	})
	if saved.Code != http.StatusOK || service.updated.ProfitMargin != 0.25 || !service.updated.Enabled ||
		!reflect.DeepEqual(service.updated.ExchangeGroupSetNames, []string{"Codex 规则"}) {
		t.Fatalf("saved=%d %s updated=%#v", saved.Code, saved.Body.String(), service.updated)
	}
	queued := authenticatedRequest(t, router, http.MethodPost, "/api/pricing/apply", nil)
	if queued.Code != http.StatusAccepted || !strings.Contains(queued.Body.String(), `"operation":"price-group-allocation"`) || service.enqueued != 1 {
		t.Fatalf("queued=%d %s calls=%d", queued.Code, queued.Body.String(), service.enqueued)
	}
	revenue := authenticatedRequest(t, router, http.MethodPost, "/api/pricing/revenue", map[string]any{"date": "2026-08-29"})
	if revenue.Code != http.StatusAccepted || !strings.Contains(revenue.Body.String(), `"operation":"revenue-calculation"`) || service.enqueued != 2 || service.revenue.Date != "2026-08-29" {
		t.Fatalf("revenue=%d %s calls=%d request=%#v", revenue.Code, revenue.Body.String(), service.enqueued, service.revenue)
	}
	latestRevenue := authenticatedRequest(t, router, http.MethodGet, "/api/pricing/revenue/latest", nil)
	if latestRevenue.Code != http.StatusOK || !strings.Contains(latestRevenue.Body.String(), `"id":"latest-revenue"`) {
		t.Fatalf("latest revenue=%d %s", latestRevenue.Code, latestRevenue.Body.String())
	}
	backup := authenticatedRequest(t, router, http.MethodPost, "/api/pricing/backups", map[string]any{"name": "调价前"})
	if backup.Code != http.StatusCreated || !strings.Contains(backup.Body.String(), `"name":"调价前"`) {
		t.Fatalf("backup=%d %s", backup.Code, backup.Body.String())
	}
	backups := authenticatedRequest(t, router, http.MethodGet, "/api/pricing/backups", nil)
	if backups.Code != http.StatusOK || !strings.Contains(backups.Body.String(), `"id":"backup-1"`) {
		t.Fatalf("backups=%d %s", backups.Code, backups.Body.String())
	}
	restore := authenticatedRequest(t, router, http.MethodPost, "/api/pricing/backups/backup-1/restore", nil)
	if restore.Code != http.StatusAccepted || !strings.Contains(restore.Body.String(), `"operation":"price-group-restore"`) || service.enqueued != 3 {
		t.Fatalf("restore=%d %s calls=%d", restore.Code, restore.Body.String(), service.enqueued)
	}
	deleted := authenticatedRequest(t, router, http.MethodDelete, "/api/pricing/backups/backup-1", nil)
	if deleted.Code != http.StatusOK || !strings.Contains(deleted.Body.String(), `"deleted":true`) || service.deletedBackupID != "backup-1" {
		t.Fatalf("deleted=%d %s backup_id=%q", deleted.Code, deleted.Body.String(), service.deletedBackupID)
	}
	service.deleteBackupErr = business.ErrPricingBackupNotFound
	missingBackup := authenticatedRequest(t, router, http.MethodDelete, "/api/pricing/backups/missing", nil)
	if missingBackup.Code != http.StatusNotFound || !strings.Contains(missingBackup.Body.String(), "价格分组备份不存在") {
		t.Fatalf("missing backup deletion=%d %s", missingBackup.Code, missingBackup.Body.String())
	}
}

func TestVaultConfigurationReturnsOnlyRedactedIndex(t *testing.T) {
	router, _ := testRouter(t, config.Config{AdminToken: "test-token"}, fakeBusiness{mode: "完全模式"})
	configured := authenticatedRequest(t, router, http.MethodPost, "/api/auth-recovery/vault-entry", map[string]any{
		"entry": "Primary", "username": "operator", "password": "secret", "hosts": []any{"api.example"},
		"headers": map[string]any{"X-Site": "private-value"},
	})
	if configured.Code != http.StatusOK {
		t.Fatalf("vault configuration failed: %d %s", configured.Code, configured.Body.String())
	}
	index := authenticatedRequest(t, router, http.MethodGet, "/api/auth-recovery/config", nil)
	if index.Code != http.StatusOK || !strings.Contains(index.Body.String(), `"has_password":true`) || strings.Contains(index.Body.String(), "operator") || strings.Contains(index.Body.String(), "private-value") {
		t.Fatalf("private values leaked: %d %s", index.Code, index.Body.String())
	}
	deleted := authenticatedRequest(t, router, http.MethodDelete, "/api/auth-recovery/vault-entry?entry=Primary", nil)
	if deleted.Code != http.StatusOK || !strings.Contains(deleted.Body.String(), `"deleted":true`) {
		t.Fatalf("vault delete failed: %d %s", deleted.Code, deleted.Body.String())
	}
}

func TestVaultMutationsSerializePartialUpdatesAndDelete(t *testing.T) {
	businessStore := &vaultLeaseBusiness{
		fakeBusiness: fakeBusiness{mode: "完全模式"},
		lease:        make(chan struct{}, 1),
		attempts:     make(chan []string, 3),
	}
	router, private := testRouter(t, config.Config{AdminToken: "test-token"}, businessStore)
	oldUsername, oldPassword := "old-user", "old-password"
	if err := private.SaveVaultEntry(context.Background(), configstore.VaultEntry{
		Entry: "Primary", Username: &oldUsername, Password: &oldPassword,
		Hosts: []string{"api.example"}, Headers: map[string]string{"X-Site": "old"},
	}, nil); err != nil {
		t.Fatal(err)
	}
	businessStore.lease <- struct{}{}

	startRequest := func(method, path string, payload any) <-chan *httptest.ResponseRecorder {
		var body *bytes.Reader
		if payload == nil {
			body = bytes.NewReader(nil)
		} else {
			encoded, err := json.Marshal(payload)
			if err != nil {
				t.Fatal(err)
			}
			body = bytes.NewReader(encoded)
		}
		request := httptest.NewRequest(method, path, body)
		request.Header.Set("Authorization", "Bearer test-token")
		if payload != nil {
			request.Header.Set("Content-Type", "application/json")
		}
		done := make(chan *httptest.ResponseRecorder, 1)
		go func() {
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			done <- response
		}()
		return done
	}
	waitAttempt := func() {
		select {
		case resources := <-businessStore.attempts:
			if len(resources) != 1 || resources[0] != mutationguard.Vault("Primary") {
				t.Fatalf("vault lease resources = %#v", resources)
			}
		case <-time.After(time.Second):
			t.Fatal("vault mutation did not attempt to acquire its resource lease")
		}
	}

	usernameDone := startRequest(http.MethodPost, "/api/auth-recovery/vault-entry", map[string]any{"entry": "Primary", "username": "new-user"})
	passwordDone := startRequest(http.MethodPost, "/api/auth-recovery/vault-entry", map[string]any{"entry": "Primary", "password": "new-password"})
	waitAttempt()
	waitAttempt()
	stored, err := private.VaultEntry(context.Background(), "Primary")
	if err != nil || stored == nil || stored.Username == nil || *stored.Username != oldUsername || stored.Password == nil || *stored.Password != oldPassword {
		t.Fatalf("blocked partial updates changed the vault: entry=%#v err=%v", stored, err)
	}
	<-businessStore.lease
	for _, done := range []<-chan *httptest.ResponseRecorder{usernameDone, passwordDone} {
		response := <-done
		if response.Code != http.StatusOK {
			t.Fatalf("partial vault update failed: %d %s", response.Code, response.Body.String())
		}
	}
	stored, err = private.VaultEntry(context.Background(), "Primary")
	if err != nil || stored == nil || stored.Username == nil || *stored.Username != "new-user" || stored.Password == nil || *stored.Password != "new-password" {
		t.Fatalf("serialized partial updates lost a field: entry=%#v err=%v", stored, err)
	}

	businessStore.lease <- struct{}{}
	deleteDone := startRequest(http.MethodDelete, "/api/auth-recovery/vault-entry?entry=Primary", nil)
	waitAttempt()
	if stored, err := private.VaultEntry(context.Background(), "Primary"); err != nil || stored == nil {
		t.Fatalf("blocked delete changed the vault: entry=%#v err=%v", stored, err)
	}
	<-businessStore.lease
	deleted := <-deleteDone
	if deleted.Code != http.StatusOK || !strings.Contains(deleted.Body.String(), `"deleted":true`) {
		t.Fatalf("vault delete failed: %d %s", deleted.Code, deleted.Body.String())
	}
}

func TestAuthRecoveryConfigurationReportsHeadersWithoutExposingValues(t *testing.T) {
	router, private := testRouter(t, config.Config{AdminToken: "test-token"}, fakeBusiness{mode: "完全模式"})
	if err := private.SaveAuthRecord(context.Background(), configstore.AuthRecord{
		Host: "api.example", BaseURL: "https://api.example", UpstreamType: "sub2api", AuthMode: "sub2api_user_token",
		Headers: map[string]string{"Authorization": "Bearer header-secret"}, Cookies: map[string]string{},
	}, nil); err != nil {
		t.Fatal(err)
	}
	index := authenticatedRequest(t, router, http.MethodGet, "/api/auth-recovery/config", nil)
	if index.Code != http.StatusOK || !strings.Contains(index.Body.String(), `"has_headers":true`) || strings.Contains(index.Body.String(), "header-secret") {
		t.Fatalf("unexpected private auth index: %d %s", index.Code, index.Body.String())
	}
}

func TestGroupMutationContractsUsePathIDAndTypedPayload(t *testing.T) {
	groupID := "6"
	policyUpdates := []map[string]any{}
	excludedUpdates := []bool{}
	router, _ := testRouter(t, config.Config{AdminToken: "test-token"}, fakeBusiness{
		mode: "完全模式", groupPolicyUpdates: &policyUpdates, groupExcludedUpdates: &excludedUpdates,
		groupRows: []business.GroupStatus{{Name: "codex", ID: &groupID, Strategy: "reliability", StrategySource: "group_override"}},
	})
	payload := map[string]any{
		"enabled": true, "strategy": "reliability", "min_pool_size": 2, "weight_budget": 500,
		"balanced_price_ratio": 0.4, "breaker_enabled": false, "recovery_enabled": true,
		"weights_enabled": true, "scaling_enabled": false, "probe_enabled": true,
		"probe_interval_seconds": 600, "probe_model": nil,
	}
	updated := authenticatedRequest(t, router, http.MethodPut, "/api/groups/6/policy", payload)
	if updated.Code != http.StatusOK || !strings.Contains(updated.Body.String(), `"strategy":"reliability"`) {
		t.Fatalf("group policy update failed: %d %s", updated.Code, updated.Body.String())
	}
	if len(policyUpdates) != 1 {
		t.Fatalf("group policy update not dispatched: %#v", policyUpdates)
	}
	cleared := authenticatedRequest(t, router, http.MethodDelete, "/api/groups/6/policy", nil)
	if cleared.Code != http.StatusOK || !strings.Contains(cleared.Body.String(), `"strategy_source":"group_override"`) {
		t.Fatalf("group policy clear failed: %d %s", cleared.Code, cleared.Body.String())
	}
	excluded := authenticatedRequest(t, router, http.MethodPut, "/api/groups/6/excluded", map[string]any{"excluded": true})
	if excluded.Code != http.StatusOK || len(excludedUpdates) != 1 || !excludedUpdates[0] {
		t.Fatalf("group exclusion failed: %d %s updates=%#v", excluded.Code, excluded.Body.String(), excludedUpdates)
	}
	invalid := authenticatedRequest(t, router, http.MethodPut, "/api/groups/6/excluded", map[string]any{"excluded": true, "extra": true})
	if invalid.Code != http.StatusUnprocessableEntity {
		t.Fatalf("unexpected unknown-field status: %d %s", invalid.Code, invalid.Body.String())
	}
}

func TestPolicyReadContract(t *testing.T) {
	strategy := "balanced"
	groupID := "6"
	router, store := testRouter(t, config.Config{}, fakeBusiness{
		mode: "完全模式",
		policySnapshot: business.PolicySnapshot{
			Available:      true,
			Source:         "当前控制面策略",
			Mode:           "完全模式",
			GlobalStrategy: &strategy,
			GroupStrategies: []business.PolicyGroupStrategy{{
				ID: &groupID, Name: "codex", Strategy: "speed_first", StrategySource: "group_override",
				ParticipationStatus: "participating", AccountCount: 2,
			}},
			AutoApply:        map[string]any{"schedulable": true},
			ExcludedGroupIDs: []string{},
			AdvancedPolicy:   map[string]any{}, ConfigurationErrors: []string{},
		},
	})
	if err := store.Initialize(context.Background(), "operator", "correct password", "https://sub2api.example", "key"); err != nil {
		t.Fatal(err)
	}
	login := request(t, router, http.MethodPost, "/api/auth/login", map[string]any{
		"username": "operator", "password": "correct password",
	}, "")
	cookie := responseCookie(t, login, sessionCookie)

	response := request(t, router, http.MethodGet, "/api/policy", nil, cookie.String())
	if response.Code != http.StatusOK {
		t.Fatalf("policy read failed: %d %s", response.Code, response.Body.String())
	}
	var snapshot business.PolicySnapshot
	if err := json.Unmarshal(response.Body.Bytes(), &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.Mode != "完全模式" || snapshot.GlobalStrategy == nil || *snapshot.GlobalStrategy != "balanced" {
		t.Fatalf("unexpected global strategy: %#v", snapshot.GlobalStrategy)
	}
	if len(snapshot.GroupStrategies) != 1 || snapshot.GroupStrategies[0].Strategy != "speed_first" || snapshot.GroupStrategies[0].StrategySource != "group_override" {
		t.Fatalf("group override was not preserved: %#v", snapshot.GroupStrategies)
	}
}

func TestPolicyUpdatePreservesExplicitEmptyCollections(t *testing.T) {
	updates := []map[string]any{}
	actors := []string{}
	strategy := "balanced"
	router, store := testRouter(t, config.Config{}, fakeBusiness{
		mode: "完全模式", policyUpdates: &updates, policyActors: &actors,
		policySnapshot: business.PolicySnapshot{
			Available: true, Source: "当前控制面策略", GlobalStrategy: &strategy,
			GroupStrategies: []business.PolicyGroupStrategy{}, AutoApply: map[string]any{},
			ExcludedGroupIDs: []string{},
			AdvancedPolicy:   map[string]any{}, ConfigurationErrors: []string{},
		},
	})
	if err := store.Initialize(context.Background(), "operator", "correct password", "https://sub2api.example", "key"); err != nil {
		t.Fatal(err)
	}
	login := request(t, router, http.MethodPost, "/api/auth/login", map[string]any{
		"username": "operator", "password": "correct password",
	}, "")
	cookie := responseCookie(t, login, sessionCookie)
	response := request(t, router, http.MethodPut, "/api/policy", map[string]any{
		"mode": "监控模式", "excluded_group_ids": []any{}, "group_strategies": map[string]any{"6": nil},
	}, cookie.String())
	if response.Code != http.StatusOK {
		t.Fatalf("policy update failed: %d %s", response.Code, response.Body.String())
	}
	if len(updates) != 1 || len(updates[0]["excluded_group_ids"].([]any)) != 0 {
		t.Fatalf("explicit empty array was lost: %#v", updates)
	}
	if updates[0]["mode"] != "监控模式" {
		t.Fatalf("runtime mode was not submitted with the policy: %#v", updates[0])
	}
	groupStrategies, ok := updates[0]["group_strategies"].(map[string]any)
	if !ok || groupStrategies["6"] != nil {
		t.Fatalf("explicit null override reset was lost: %#v", updates[0]["group_strategies"])
	}
	if len(actors) != 1 || actors[0] != "operator" {
		t.Fatalf("unexpected policy actor: %#v", actors)
	}
	invalid := request(t, router, http.MethodPut, "/api/policy", nil, cookie.String())
	if invalid.Code != http.StatusUnprocessableEntity {
		t.Fatalf("null body status = %d body=%s", invalid.Code, invalid.Body.String())
	}
}

func TestNotificationAndInspectionServiceContracts(t *testing.T) {
	messages := []string{}
	configs := []business.AutoInspectionConfig{}
	events := []int64{}
	messageID := "confirmed-message"
	businessService := fakeBusiness{mode: "完全模式", runtimeEventIDs: &events}
	notifier := fakeNotifier{messages: &messages, result: notification.TestResult{
		Sent: true, Detail: "发送成功", MessageID: &messageID, RuntimeEventID: -2, Persisted: true,
	}}
	controller := fakeInspectionController{configs: &configs, status: inspection.Status{
		AutoInspectionConfig: business.AutoInspectionConfig{Enabled: false, IntervalSeconds: 15},
		Queue:                []inspection.QueueItem{}, HeartbeatHistory: []business.InspectionHeartbeat{},
	}}
	router, _ := testRouterWithDependencies(t, config.Config{AdminToken: "test-token"}, businessService, Dependencies{
		Notification: notifier, Inspection: controller,
	})

	notificationResponse := authenticatedRequest(t, router, http.MethodPost, "/api/notifications/test", map[string]any{
		"message": "Sub2API Console 通知测试", "dry_run": false,
	})
	if notificationResponse.Code != http.StatusOK || !strings.Contains(notificationResponse.Body.String(), `"message_id":"confirmed-message"`) || len(messages) != 1 {
		t.Fatalf("unexpected notification test: %d %s messages=%#v", notificationResponse.Code, notificationResponse.Body.String(), messages)
	}
	inspectionResponse := authenticatedRequest(t, router, http.MethodGet, "/api/inspection/automation", nil)
	if inspectionResponse.Code != http.StatusOK || !strings.Contains(inspectionResponse.Body.String(), `"interval_seconds":15`) {
		t.Fatalf("unexpected inspection status: %d %s", inspectionResponse.Code, inspectionResponse.Body.String())
	}
	updated := authenticatedRequest(t, router, http.MethodPut, "/api/inspection/automation", map[string]any{
		"enabled": true, "interval_seconds": 30,
	})
	if updated.Code != http.StatusOK || len(configs) != 1 || !configs[0].Enabled || configs[0].IntervalSeconds != 30 || len(events) != 1 {
		t.Fatalf("unexpected inspection update: %d %s configs=%#v events=%#v", updated.Code, updated.Body.String(), configs, events)
	}
	canceled := authenticatedRequest(t, router, http.MethodPost, "/api/inspection/automation/cancel", nil)
	if canceled.Code != http.StatusOK || !strings.Contains(canceled.Body.String(), `"canceled":`) {
		t.Fatalf("unexpected inspection cancellation: %d %s", canceled.Code, canceled.Body.String())
	}
	resumed := authenticatedRequest(t, router, http.MethodPost, "/api/inspection/automation/resume", nil)
	if resumed.Code != http.StatusOK || !strings.Contains(resumed.Body.String(), `"enabled":true`) || len(events) != 3 {
		t.Fatalf("unexpected inspection resume: %d %s events=%#v", resumed.Code, resumed.Body.String(), events)
	}
}

func TestInspectionUpdateReturnsActualStateWhenSecondaryEventPersistenceFails(t *testing.T) {
	configs := []business.AutoInspectionConfig{}
	controller := fakeInspectionController{configs: &configs, status: inspection.Status{
		AutoInspectionConfig: business.AutoInspectionConfig{Enabled: false, IntervalSeconds: 15},
		Queue:                []inspection.QueueItem{},
		HeartbeatHistory:     []business.InspectionHeartbeat{},
	}}
	router, _ := testRouterWithDependencies(t, config.Config{AdminToken: "test-token"}, fakeBusiness{
		mode: "完全模式", runtimeEventError: errors.New("event database unavailable"),
	}, Dependencies{Inspection: controller})
	response := authenticatedRequest(t, router, http.MethodPut, "/api/inspection/automation", map[string]any{
		"enabled": true, "interval_seconds": 30,
	})
	if response.Code != http.StatusOK || len(configs) != 1 || !configs[0].Enabled {
		t.Fatalf("updated state was reported as failed: %d %s configs=%#v", response.Code, response.Body.String(), configs)
	}
}

func TestInspectionHistoryAndLogMaintenanceContracts(t *testing.T) {
	updates := [][2]int{}
	controller := fakeInspectionController{status: inspection.Status{
		AutoInspectionConfig: business.AutoInspectionConfig{Enabled: false, IntervalSeconds: 15},
		Queue:                []inspection.QueueItem{}, HeartbeatHistory: []business.InspectionHeartbeat{},
	}}
	maintenance := fakeLogMaintenance{
		status:  consolelogs.CleanupStatus{LogCleanupSettings: configstore.LogCleanupSettings{RetentionDays: 30}},
		updates: &updates,
		result:  consolelogs.CleanupResult{Tasks: 1, Runs: 2, Events: 3, Changes: 4, Total: 10},
	}
	router, _ := testRouterWithDependencies(t, config.Config{AdminToken: "test-token"}, fakeBusiness{mode: "完全模式"}, Dependencies{
		Inspection: controller, LogMaintenance: maintenance,
	})

	read := authenticatedRequest(t, router, http.MethodGet, "/api/config/log-cleanup", nil)
	if read.Code != http.StatusOK || !strings.Contains(read.Body.String(), `"retention_days":30`) {
		t.Fatalf("unexpected cleanup status: %d %s", read.Code, read.Body.String())
	}
	updated := authenticatedRequest(t, router, http.MethodPut, "/api/config/log-cleanup", map[string]any{"enabled": true, "retention_days": 45})
	if updated.Code != http.StatusOK || len(updates) != 1 || updates[0] != [2]int{1, 45} {
		t.Fatalf("unexpected cleanup update: %d %s updates=%#v", updated.Code, updated.Body.String(), updates)
	}
	missingRetention := authenticatedRequest(t, router, http.MethodDelete, "/api/logs", nil)
	if missingRetention.Code != http.StatusUnprocessableEntity {
		t.Fatalf("cleanup without retention must be rejected: %d %s", missingRetention.Code, missingRetention.Body.String())
	}
	clearedLogs := authenticatedRequest(t, router, http.MethodDelete, "/api/logs?retention_days=45", nil)
	if clearedLogs.Code != http.StatusOK || !strings.Contains(clearedLogs.Body.String(), `"total":10`) || !strings.Contains(clearedLogs.Body.String(), `"retention_days":45`) {
		t.Fatalf("unexpected log cleanup: %d %s", clearedLogs.Code, clearedLogs.Body.String())
	}
	clearedHeartbeats := authenticatedRequest(t, router, http.MethodDelete, "/api/inspection/automation/history", nil)
	if clearedHeartbeats.Code != http.StatusOK || !strings.Contains(clearedHeartbeats.Body.String(), `"deleted":2`) {
		t.Fatalf("unexpected heartbeat cleanup: %d %s", clearedHeartbeats.Code, clearedHeartbeats.Body.String())
	}
}

func TestActiveProbeRoutePreservesOptionalFiltersAndRejectsInvalidValues(t *testing.T) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	calls := []probeCall{}
	task := taskstore.Task{
		ID: "probe-1", Skill: "sub2api-connectivity-test", Operation: "active-probe", Status: "queued",
		Progress: 0, Message: "主动探测已排队", Result: map[string]any{}, CreatedAt: now, UpdatedAt: now,
	}
	router, _ := testRouterWithDependencies(t, config.Config{AdminToken: "test-token"}, fakeBusiness{mode: "完全模式"}, Dependencies{
		ProbeTasks: fakeProbeTasks{task: task, calls: &calls},
	})
	response := authenticatedRequest(t, router, http.MethodPost, "/api/inspection/probe", map[string]any{
		"account_id": "41", "group_name": " codex ",
	})
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"id":"probe-1"`) || len(calls) != 1 {
		t.Fatalf("response=%d %s calls=%#v", response.Code, response.Body.String(), calls)
	}
	if calls[0].request.AccountID == nil || *calls[0].request.AccountID != "41" || calls[0].request.GroupName == nil || *calls[0].request.GroupName != "codex" || calls[0].actor != "console" {
		t.Fatalf("optional filters were not preserved: %#v", calls[0])
	}

	invalidBodies := []map[string]any{
		{"account_id": nil}, {"account_id": "041"}, {"group_name": nil}, {"group_name": " "}, {"unexpected": true},
	}
	for _, body := range invalidBodies {
		invalid := authenticatedRequest(t, router, http.MethodPost, "/api/inspection/probe", body)
		if invalid.Code != http.StatusUnprocessableEntity {
			t.Fatalf("body=%#v status=%d response=%s", body, invalid.Code, invalid.Body.String())
		}
	}
	if len(calls) != 1 {
		t.Fatalf("invalid requests reached probe service: %#v", calls)
	}
}

func TestModelCheckRoutesExposeCapabilitiesAndForwardAccountMatrix(t *testing.T) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	requests := []modelcheck.Request{}
	task := taskstore.Task{
		ID: "model-check-1", Skill: "sub2api-model-check", Operation: "model-behavior-check",
		Status: "queued", Progress: 0, Message: "模型检测已排队", Result: map[string]any{"credentials_persisted": false},
		CreatedAt: now, UpdatedAt: now,
	}
	router, _ := testRouterWithDependencies(t, config.Config{AdminToken: "test-token"}, fakeBusiness{mode: "完全模式"}, Dependencies{
		ModelChecks: fakeModelChecks{
			task: task, requests: &requests,
			statuses: []modelcheck.AccountCheckStatus{{
				AccountID: "41", Status: "consistent", CheckedAt: now, TaskID: "model-check-previous",
			}},
		},
	})
	capabilities := authenticatedRequest(t, router, http.MethodGet, "/api/model-checks/capabilities", nil)
	if capabilities.Code != http.StatusOK || !strings.Contains(capabilities.Body.String(), `"claude_standards":["claude-opus-5"]`) {
		t.Fatalf("unexpected capabilities: %d %s", capabilities.Code, capabilities.Body.String())
	}
	statuses := authenticatedRequest(t, router, http.MethodGet, "/api/model-checks/account-statuses", nil)
	if statuses.Code != http.StatusOK || !strings.Contains(statuses.Body.String(), `"account_id":"41"`) || !strings.Contains(statuses.Body.String(), `"status":"consistent"`) {
		t.Fatalf("unexpected account statuses: %d %s", statuses.Code, statuses.Body.String())
	}
	response := authenticatedRequest(t, router, http.MethodPost, "/api/model-checks", map[string]any{
		"account_ids": []string{"41", "42"}, "models": []string{"claude-opus-5", "gpt-5.6-sol"},
		"rounds": 3, "timeout_seconds": 40,
	})
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"id":"model-check-1"`) || len(requests) != 1 {
		t.Fatalf("response=%d %s requests=%#v", response.Code, response.Body.String(), requests)
	}
	if len(requests[0].AccountIDs) != 2 || len(requests[0].Models) != 2 || requests[0].Rounds != 3 {
		t.Fatalf("request fields not preserved: %#v", requests[0])
	}

	invalid := authenticatedRequest(t, router, http.MethodPost, "/api/model-checks", map[string]any{
		"account_ids": []string{}, "models": []string{"gpt-5.6-sol"},
	})
	if invalid.Code != http.StatusUnprocessableEntity || len(requests) != 1 {
		t.Fatalf("invalid request reached service: status=%d requests=%#v", invalid.Code, requests)
	}
}

func TestTaskReadContracts(t *testing.T) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	repository := fakeTaskRepository{rows: []taskstore.Task{{
		ID: "task-1", Skill: "console", Operation: "sync", Status: "running", Progress: 40,
		Message: "同步中", Result: map[string]any{}, CreatedAt: now, UpdatedAt: now,
	}}}
	router, _ := testRouterWithDependencies(t, config.Config{AdminToken: "test-token"}, fakeBusiness{mode: "完全模式"}, Dependencies{Tasks: repository})
	list := authenticatedRequest(t, router, http.MethodGet, "/api/tasks?limit=1", nil)
	if list.Code != http.StatusNotFound {
		t.Fatalf("retired task list status: %d %s", list.Code, list.Body.String())
	}
	detail := authenticatedRequest(t, router, http.MethodGet, "/api/tasks/task-1", nil)
	if detail.Code != http.StatusOK || !strings.Contains(detail.Body.String(), `"progress":40`) {
		t.Fatalf("unexpected task detail: %d %s", detail.Code, detail.Body.String())
	}
	missing := authenticatedRequest(t, router, http.MethodGet, "/api/tasks/missing", nil)
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing task status = %d", missing.Code)
	}
	missingEvents := authenticatedRequest(t, router, http.MethodGet, "/api/tasks/missing/events", nil)
	if missingEvents.Code != http.StatusNotFound || !strings.Contains(missingEvents.Body.String(), taskstore.ErrNotFound.Error()) {
		t.Fatalf("missing task event stream = %d %s", missingEvents.Code, missingEvents.Body.String())
	}
}

func TestTaskEventStreamContinuesUntilTerminalState(t *testing.T) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	running := taskstore.Task{
		ID: "long-task", Skill: "console", Operation: "inspect", Status: "running", Progress: 50,
		Message: "运行中", Result: map[string]any{}, CreatedAt: now, UpdatedAt: now,
	}
	succeeded := running
	succeeded.Status, succeeded.Progress, succeeded.Message = "succeeded", 100, "完成"
	repository := &sequenceTaskRepository{rows: []taskstore.Task{running, succeeded}}
	router, _ := testRouterWithDependencies(t, config.Config{AdminToken: "test-token"}, fakeBusiness{mode: "完全模式"}, Dependencies{Tasks: repository})

	response := authenticatedRequest(t, router, http.MethodGet, "/api/tasks/long-task/events", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("task event stream status = %d: %s", response.Code, response.Body.String())
	}
	if contentType := response.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "text/event-stream") {
		t.Fatalf("unexpected content type: %q", contentType)
	}
	if response.Header().Get("Cache-Control") != "no-cache" || response.Header().Get("X-Accel-Buffering") != "no" {
		t.Fatalf("missing SSE proxy headers: %#v", response.Header())
	}
	body := response.Body.String()
	if !strings.Contains(body, `"status":"running"`) || !strings.Contains(body, `"status":"succeeded"`) || repository.reads != 2 {
		t.Fatalf("stream did not reach terminal state: reads=%d body=%s", repository.reads, body)
	}
}

func TestPartialTaskStatusIsTerminal(t *testing.T) {
	if !terminalTaskStatus("partial") {
		t.Fatal("partial task status must stop task event streaming")
	}
}

func TestRequestTraceAndAlertRoutesUseCurrentServicesAndBoundedLimits(t *testing.T) {
	requestIDs := []string{}
	trace := business.RequestTrace{RequestID: "req/42", Matched: true, Records: []business.UsageRecord{}, RecentErrors: []business.UsageRecord{}}
	alert := business.AlertListItem{AlertIncident: business.AlertIncident{
		IncidentKey: "probe:41", EventType: "probe_failed", ObjectKind: "account", ObjectID: "41",
		CauseCode: "timeout", Status: "open", FirstSeenAt: "2026-08-27T00:00:00Z", LastSeenAt: "2026-08-27T00:00:00Z",
	}}
	router, _ := testRouterWithDependencies(t, config.Config{AdminToken: "test-token"}, fakeBusiness{
		mode: "完全模式", alertRows: []business.AlertListItem{alert}, clearedAlerts: 1,
	}, Dependencies{RequestTrace: fakeTraceReader{trace: trace, ids: &requestIDs}})
	traceResponse := authenticatedRequest(t, router, http.MethodGet, "/api/usage/trace/req%2F42", nil)
	if traceResponse.Code != http.StatusOK || !strings.Contains(traceResponse.Body.String(), `"request_id":"req/42"`) || len(requestIDs) != 1 || requestIDs[0] != "req/42" {
		t.Fatalf("trace=%d %s ids=%#v", traceResponse.Code, traceResponse.Body.String(), requestIDs)
	}
	alerts := authenticatedRequest(t, router, http.MethodGet, "/api/alerts?limit=200", nil)
	if alerts.Code != http.StatusOK || !strings.Contains(alerts.Body.String(), `"incident_key":"probe:41"`) {
		t.Fatalf("alerts=%d %s", alerts.Code, alerts.Body.String())
	}
	invalidLimit := authenticatedRequest(t, router, http.MethodGet, "/api/alerts?limit=100001", nil)
	if invalidLimit.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid alert limit=%d %s", invalidLimit.Code, invalidLimit.Body.String())
	}
	cleared := authenticatedRequest(t, router, http.MethodDelete, "/api/alerts", nil)
	if cleared.Code != http.StatusOK || !strings.Contains(cleared.Body.String(), `"deleted":1`) {
		t.Fatalf("clear alerts=%d %s", cleared.Code, cleared.Body.String())
	}
}

func TestTrafficRankingRouteValidatesDimensionsAndUsesBoundedTimeWindow(t *testing.T) {
	queries := []business.TrafficRankingQuery{}
	score := 91.25
	ranking := business.TrafficRanking{
		StartAt: "2026-08-30T12:00:00Z", EndAt: "2026-08-31T12:00:00Z", SortBy: "stability",
		TotalRequests: 20, AccountsWithTraffic: 1,
		Accounts: []business.TrafficRankingRow{{Rank: 1, AccountID: "41", AccountName: "alpha", Requests: 20, StabilityScore: &score}},
	}
	router, _ := testRouterWithDependencies(t, config.Config{AdminToken: "test-token"}, fakeBusiness{
		mode: "完全模式", trafficRanking: ranking, trafficQueries: &queries,
	}, Dependencies{})

	response := authenticatedRequest(t, router, http.MethodGet, "/api/traffic/ranking?time_range=24h&group=codex&sort_by=stability", nil)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"account_id":"41"`) {
		t.Fatalf("traffic ranking=%d %s", response.Code, response.Body.String())
	}
	if len(queries) != 1 || queries[0].GroupName != "codex" || queries[0].SortBy != "stability" || queries[0].EndAt.Sub(queries[0].StartAt) != 24*time.Hour {
		t.Fatalf("queries=%#v", queries)
	}
	for _, path := range []string{
		"/api/traffic/ranking?time_range=90d", "/api/traffic/ranking?sort_by=unknown",
	} {
		invalid := authenticatedRequest(t, router, http.MethodGet, path, nil)
		if invalid.Code != http.StatusUnprocessableEntity {
			t.Fatalf("invalid ranking query %s=%d %s", path, invalid.Code, invalid.Body.String())
		}
	}
}

func TestRequestTraceTimeoutReturnsGatewayTimeout(t *testing.T) {
	router, _ := testRouterWithDependencies(t, config.Config{AdminToken: "test-token"}, fakeBusiness{
		mode: "完全模式",
	}, Dependencies{RequestTrace: fakeTraceReader{err: context.DeadlineExceeded}})

	response := authenticatedRequest(t, router, http.MethodGet, "/api/usage/trace/req-timeout", nil)
	if response.Code != http.StatusGatewayTimeout || !strings.Contains(response.Body.String(), "请求追踪超时") {
		t.Fatalf("trace timeout=%d %s", response.Code, response.Body.String())
	}
}

func TestSystemLogRouteAcceptsManagementPlatformSearchConditions(t *testing.T) {
	queries := []opstraffic.SystemLogQuery{}
	page := business.SystemLogPage{Items: []business.UsageRecord{{ID: 9, RequestID: "req-42", Source: "system-log"}}, Total: 1, Page: 2, PageSize: 20}
	router, _ := testRouterWithDependencies(t, config.Config{AdminToken: "test-token"}, fakeBusiness{mode: "完全模式"}, Dependencies{
		SystemLogs: fakeSystemLogReader{page: page, queries: &queries},
	})
	path := "/api/ops/system-logs?time_range=6h&host=node-1&level=info&component=http.access&request_id=req-42&client_request_id=client-42&user_id=7&api_key_id=8&account_id=42&platform=openai&model=gpt-5&q=completed&page=2&page_size=20"
	response := authenticatedRequest(t, router, http.MethodGet, path, nil)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"request_id":"req-42"`) {
		t.Fatalf("system logs=%d %s", response.Code, response.Body.String())
	}
	if len(queries) != 1 {
		t.Fatalf("queries=%#v", queries)
	}
	query := queries[0]
	if query.TimeRange != "6h" || query.Host != "node-1" || query.RequestID != "req-42" || query.ClientRequestID != "client-42" || query.APIKeyID != "8" || query.Keyword != "completed" || query.Page != 2 || query.PageSize != 20 {
		t.Fatalf("query=%#v", query)
	}
}

func TestSystemLogRouteRejectsInvalidIdentityFilter(t *testing.T) {
	router, _ := testRouterWithDependencies(t, config.Config{AdminToken: "test-token"}, fakeBusiness{mode: "完全模式"}, Dependencies{
		SystemLogs: fakeSystemLogReader{},
	})
	response := authenticatedRequest(t, router, http.MethodGet, "/api/ops/system-logs?account_id=not-a-number", nil)
	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "account_id 必须是正整数") {
		t.Fatalf("invalid filter=%d %s", response.Code, response.Body.String())
	}
}

func TestNotificationQueueRouteReturnsCurrentQueueContents(t *testing.T) {
	keys := []string{}
	details := business.NotificationQueueDetails{
		ProducerFiring: []business.NotificationQueueItem{{AlertIncident: business.AlertIncident{
			IncidentKey: "balance:host", EventType: "upstream.balance", ObjectKind: "host", ObjectID: "api.example",
			CauseCode: "BALANCE:5", Status: "firing", FirstSeenAt: "2026-08-28T12:00:00Z", LastSeenAt: "2026-08-28T12:01:00Z",
		}}},
		ProducerRecovered: []business.NotificationQueueItem{},
		ConsumerPending:   []business.NotificationQueueItem{},
		ConsumerFailed:    []business.NotificationQueueItem{},
	}
	router, _ := testRouter(t, config.Config{AdminToken: "test-token"}, fakeQueueBusiness{
		fakeBusiness: fakeBusiness{mode: "完全模式"}, details: details, keys: &keys,
	})
	response := authenticatedRequest(t, router, http.MethodGet, "/api/notifications/queue", nil)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"incident_key":"balance:host"`) ||
		!strings.Contains(response.Body.String(), `"consumer_pending":[]`) {
		t.Fatalf("unexpected queue response: %d %s", response.Code, response.Body.String())
	}
	if len(keys) != 1 || keys[0] == "" {
		t.Fatalf("notification channel key was not resolved: %#v", keys)
	}
}

func TestAlertPolicyContracts(t *testing.T) {
	policy := business.DefaultAlertPolicy()
	router, _ := testRouter(t, config.Config{AdminToken: "test-token"}, fakeBusiness{mode: "完全模式", alertPolicy: policy})
	read := authenticatedRequest(t, router, http.MethodGet, "/api/alerts/policy", nil)
	if read.Code != http.StatusOK || !strings.Contains(read.Body.String(), `"balance_thresholds":["20","10","5"]`) ||
		!strings.Contains(read.Body.String(), `"multiplier_increase_enabled":true`) ||
		!strings.Contains(read.Body.String(), `"multiplier_decrease_enabled":true`) ||
		!strings.Contains(read.Body.String(), `"routing_degraded_types":["health_score","gateway_error_rate","latency","other"]`) ||
		!strings.Contains(read.Body.String(), `"recovery_notification_types":["auth","balance","group_unavailable"]`) {
		t.Fatalf("unexpected alert policy: %d %s", read.Code, read.Body.String())
	}
	payload := map[string]any{
		"enabled": true, "configuration_enabled": true, "auth_enabled": true, "rate_sync_enabled": true,
		"multiplier_increase_enabled": true, "multiplier_decrease_enabled": true,
		"balance_enabled": true, "probe_enabled": true, "balance_thresholds": []any{"20", "10", "5"},
		"routing_breaker_enabled": true, "routing_degraded_enabled": true,
		"routing_degraded_types": []any{"health_score", "gateway_error_rate", "latency", "other"}, "routing_survivor_enabled": true,
		"group_unavailable_enabled": true, "group_survivor_enabled": true, "apply_failure_enabled": true,
		"probe_failure_streak": 3, "probe_recovery_streak": 3, "probe_groups": []any{}, "delivery_enabled": true, "notify_recovery": true,
		"recovery_notification_types": []any{"auth", "balance", "group_unavailable"},
		"repeat_interval_minutes":     0, "state_change_cooldown_minutes": 30, "merge_threshold": 10,
	}
	updated := authenticatedRequest(t, router, http.MethodPut, "/api/alerts/policy", payload)
	if updated.Code != http.StatusOK {
		t.Fatalf("alert policy update failed: %d %s", updated.Code, updated.Body.String())
	}
}

func TestParseOnboardingBatchRequestsKeepsEachBindingExplicit(t *testing.T) {
	requests, err := parseOnboardingBatchRequests(map[string]any{
		"items": []any{
			map[string]any{
				"host": "https://upstream.test", "upstream_type": "sub2api",
				"local_group_id": json.Number("3"), "upstream_group_id": "6", "platform": "openai",
			},
			map[string]any{
				"host": "https://upstream.test", "upstream_type": "sub2api",
				"local_group_id": json.Number("4"), "upstream_group_id": "7",
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(requests) != 2 || requests[0].LocalGroupID != "3" || requests[0].Platform == nil ||
		*requests[0].Platform != "openai" || !requests[0].PlatformPresent || requests[1].UpstreamGroupID != "7" {
		t.Fatalf("requests=%#v", requests)
	}
}

func TestParseOnboardingBatchRequestsRejectsInvalidBatchShape(t *testing.T) {
	for name, payload := range map[string]map[string]any{
		"empty":         {"items": []any{}},
		"not an array":  {"items": "invalid"},
		"unknown field": {"items": []any{}, "host": "upstream.test"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parseOnboardingBatchRequests(payload); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func testRouter(t *testing.T, cfg config.Config, business Business) (http.Handler, *configstore.Store) {
	return testRouterWithDependencies(t, cfg, business, Dependencies{})
}

func testRouterWithDependencies(t *testing.T, cfg config.Config, business Business, dependencies Dependencies) (http.Handler, *configstore.Store) {
	t.Helper()
	store, err := configstore.Open(filepath.Join(t.TempDir(), "console-config.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	router := New(cfg, store, business, dependencies)
	return router, store
}

func request(t *testing.T, handler http.Handler, method string, path string, payload any, cookie string) *httptest.ResponseRecorder {
	t.Helper()
	var body *bytes.Reader
	if payload == nil {
		body = bytes.NewReader(nil)
	} else {
		encoded, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		body = bytes.NewReader(encoded)
	}
	req := httptest.NewRequest(method, path, body)
	req.RemoteAddr = "127.0.0.1:1234"
	req.Host = "127.0.0.1"
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if cookie != "" {
		req.Header.Set("Cookie", cookie)
		if method != http.MethodGet && method != http.MethodHead && method != http.MethodOptions {
			req.Header.Set("Origin", "http://127.0.0.1")
		}
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	return response
}

func authenticatedRequest(t *testing.T, handler http.Handler, method string, path string, payload any) *httptest.ResponseRecorder {
	t.Helper()
	var body *bytes.Reader
	if payload == nil {
		body = bytes.NewReader(nil)
	} else {
		encoded, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		body = bytes.NewReader(encoded)
	}
	req := httptest.NewRequest(method, path, body)
	req.Header.Set("Authorization", "Bearer test-token")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	return response
}

func responseCookie(t *testing.T, response *httptest.ResponseRecorder, name string) *http.Cookie {
	t.Helper()
	for _, cookie := range response.Result().Cookies() {
		if cookie.Name == name {
			return cookie
		}
	}
	t.Fatalf("cookie %q not found", name)
	return nil
}

func testNow() (result time.Time) {
	return time.Now()
}
