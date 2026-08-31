package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountops"
	"github.com/MIEnchating/sub2api-console/backend/internal/alerting"
	"github.com/MIEnchating/sub2api-console/backend/internal/api"
	"github.com/MIEnchating/sub2api-console/backend/internal/authrecovery"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/config"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/evidence"
	"github.com/MIEnchating/sub2api-console/backend/internal/inspection"
	consolelogs "github.com/MIEnchating/sub2api-console/backend/internal/logs"
	"github.com/MIEnchating/sub2api-console/backend/internal/management"
	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
	"github.com/MIEnchating/sub2api-console/backend/internal/notification"
	"github.com/MIEnchating/sub2api-console/backend/internal/notificationtarget"
	"github.com/MIEnchating/sub2api-console/backend/internal/onboarding"
	"github.com/MIEnchating/sub2api-console/backend/internal/opstraffic"
	"github.com/MIEnchating/sub2api-console/backend/internal/pricing"
	"github.com/MIEnchating/sub2api-console/backend/internal/probe"
	"github.com/MIEnchating/sub2api-console/backend/internal/routing"
	"github.com/MIEnchating/sub2api-console/backend/internal/routingwrite"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/upstreamauth"
	"github.com/MIEnchating/sub2api-console/backend/internal/upstreamconfig"
	"github.com/MIEnchating/sub2api-console/backend/internal/upstreamdelete"
	"github.com/MIEnchating/sub2api-console/backend/internal/upstreamdetect"
	"github.com/MIEnchating/sub2api-console/backend/internal/upstreamsync"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	privateStore, err := configstore.Open(cfg.ConfigDB)
	if err != nil {
		log.Fatal(err)
	}
	defer privateStore.Close()
	businessStore, err := business.Open(cfg.DataDB)
	if err != nil {
		log.Fatal(err)
	}
	defer businessStore.Close()
	taskStore, err := taskstore.Open(cfg.TaskDB)
	if err != nil {
		log.Fatal(err)
	}
	defer taskStore.Close()
	if recovered, err := taskStore.RecoverInterrupted(context.Background()); err != nil {
		log.Fatal(err)
	} else if recovered > 0 {
		log.Printf("已将 %d 个进程重启前未完成任务标记为失败", recovered)
	}
	notificationService := notification.New(
		businessStore,
		privateStore,
		notification.NewQQBotSender(&http.Client{Timeout: 20 * time.Second}),
	)
	notificationTargetDiscovery := notificationtarget.New(
		notificationtarget.NewGatewayListener(&http.Client{Timeout: 20 * time.Second}),
		taskStore,
	)
	alertService := alerting.New(businessStore, notificationService)
	alertTasks := alerting.NewTaskService(alertService, taskStore)
	upstreamReader := upstreamsync.NewReader(&http.Client{Timeout: 20 * time.Second})
	accountTasks := accountops.New(privateStore, businessStore, taskStore)
	managementTasks := management.New(privateStore, businessStore, taskStore, accountTasks)
	pricingTasks := pricing.New(businessStore, privateStore, taskStore)
	managementTasks.UseUpstreamCatalogReader(upstreamReader)
	probeTasks := probe.New(businessStore, privateStore, taskStore)
	modelChecks, err := modelcheck.New(taskStore, privateStore, businessStore, upstreamReader)
	if err != nil {
		log.Fatal(err)
	}
	upstreamDetector := upstreamdetect.New(&http.Client{Timeout: 8 * time.Second})
	authClient := upstreamauth.New(&http.Client{Timeout: 20 * time.Second})
	upstreamConfigurations := upstreamconfig.New(
		businessStore,
		privateStore,
		authClient,
	)
	upstreamSyncTasks := upstreamsync.New(
		businessStore,
		privateStore,
		upstreamReader,
		authClient,
		taskStore,
	)
	upstreamDeleteService := upstreamdelete.New(businessStore, privateStore, taskStore)
	onboardingService := onboarding.New(businessStore, privateStore, upstreamReader, taskStore)
	captchaManager := authrecovery.NewCaptchaManager(
		privateStore,
		authClient,
		upstreamReader,
		&http.Client{Timeout: 20 * time.Second},
	)
	authRecoveryService := authrecovery.New(
		businessStore,
		privateStore,
		authClient,
		upstreamConfigurations,
		upstreamSyncTasks,
		taskStore,
		captchaManager,
	)
	authRecoveryService.UsePlatformDetector(upstreamDetector)
	pricingTasks.UseAuthResolver(authRecoveryService)
	upstreamSyncTasks.SetAuthResolver(authRecoveryService)
	managementTasks.UseUpstreamAuthResolver(authRecoveryService)
	modelChecks.UseUpstreamAuthResolver(authRecoveryService)
	logService := consolelogs.New(businessStore, taskStore)
	logMaintenance := consolelogs.NewMaintenance(privateStore, businessStore, taskStore)
	evidenceService := evidence.New(businessStore, probeTasks)
	opsTrafficService := opstraffic.New(privateStore, businessStore)
	routingService := routing.NewService(businessStore)
	routingWriteService := routingwrite.New(privateStore, businessStore)
	inspectionRunner := inspection.NewRunner(
		businessStore,
		privateStore,
		evidenceService,
		routingService,
		routingWriteService,
		alertService,
		upstreamSyncTasks,
		taskStore,
		managementTasks,
		pricingTasks,
	)
	inspectionScheduler, err := inspection.NewScheduler(businessStore, inspectionRunner)
	if err != nil {
		log.Fatal(err)
	}
	serviceContext, cancelServices := context.WithCancel(context.Background())
	defer cancelServices()
	if err := logMaintenance.Start(serviceContext); err != nil {
		log.Fatal(err)
	}
	defer logMaintenance.Stop()
	if err := inspectionScheduler.Start(serviceContext); err != nil {
		log.Fatal(err)
	}
	defer inspectionScheduler.Stop()
	manualInspections := inspection.NewManualService(inspectionScheduler, inspectionRunner, taskStore)

	handler := api.New(cfg, privateStore, businessStore, api.Dependencies{
		Notification:       notificationService,
		NotificationTarget: notificationTargetDiscovery,
		Inspection:         inspectionScheduler,
		InspectionTasks:    manualInspections,
		RoutingControl:     routingWriteService,
		Tasks:              taskStore,
		Logs:               logService,
		LogMaintenance:     logMaintenance,
		AlertTasks:         alertTasks,
		ManagementTasks:    managementTasks,
		AccountMaintenance: managementTasks,
		AccountTasks:       accountTasks,
		ProbeTasks:         probeTasks,
		ModelChecks:        modelChecks,
		UpstreamDetect:     upstreamDetector,
		UpstreamConfigs:    upstreamConfigurations,
		UpstreamSync:       upstreamSyncTasks,
		UpstreamDelete:     upstreamDeleteService,
		AuthRecovery:       authRecoveryService,
		Onboarding:         onboardingService,
		RequestTrace:       opsTrafficService,
		SystemLogs:         opsTrafficService,
		Pricing:            pricingTasks,
	})
	server := newHTTPServer(cfg.ListenAddress, handler)
	stopped := make(chan os.Signal, 1)
	signal.Notify(stopped, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-stopped
		cancelServices()
		inspectionScheduler.Stop()
		shutdown, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdown); err != nil {
			log.Printf("HTTP 服务停止失败: %v", err)
		}
	}()
	log.Printf("Sub2API Console Go API listening on %s", cfg.ListenAddress)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

func newHTTPServer(address string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              address,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		// SSE responses remain open for the lifetime of a task or scheduler subscription.
		WriteTimeout: 0,
		IdleTimeout:  60 * time.Second,
	}
}
