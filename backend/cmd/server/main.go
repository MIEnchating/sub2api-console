package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountdelete"
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
	"github.com/MIEnchating/sub2api-console/backend/internal/newapimanagement"
	"github.com/MIEnchating/sub2api-console/backend/internal/notification"
	"github.com/MIEnchating/sub2api-console/backend/internal/notificationtarget"
	"github.com/MIEnchating/sub2api-console/backend/internal/onboarding"
	"github.com/MIEnchating/sub2api-console/backend/internal/opstraffic"
	"github.com/MIEnchating/sub2api-console/backend/internal/pricing"
	"github.com/MIEnchating/sub2api-console/backend/internal/probe"
	"github.com/MIEnchating/sub2api-console/backend/internal/routing"
	"github.com/MIEnchating/sub2api-console/backend/internal/routingwrite"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskrunner"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/upstreamauth"
	"github.com/MIEnchating/sub2api-console/backend/internal/upstreamconfig"
	"github.com/MIEnchating/sub2api-console/backend/internal/upstreamdelete"
	"github.com/MIEnchating/sub2api-console/backend/internal/upstreamdetect"
	"github.com/MIEnchating/sub2api-console/backend/internal/upstreamsync"
	"golang.org/x/sys/unix"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	privateStore, err := configstore.Open(cfg.ConfigDB)
	if err != nil {
		return err
	}
	closeStores := true
	defer func() {
		if closeStores {
			_ = privateStore.Close()
		}
	}()
	businessStore, err := business.Open(cfg.DataDB)
	if err != nil {
		return err
	}
	defer func() {
		if closeStores {
			_ = businessStore.Close()
		}
	}()
	taskStore, err := taskstore.Open(cfg.TaskDB)
	if err != nil {
		return err
	}
	defer func() {
		if closeStores {
			_ = taskStore.Close()
		}
	}()
	if recovered, err := taskStore.RecoverInterrupted(context.Background()); err != nil {
		return err
	} else if recovered > 0 {
		log.Printf("已将 %d 个进程重启前未完成任务标记为失败", recovered)
	}
	serviceContext, cancelServices := context.WithCancel(context.Background())
	backgroundTasks := taskrunner.New(serviceContext)
	defer cancelServices()
	defer backgroundTasks.Cancel()
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
		return err
	}
	upstreamDetector := upstreamdetect.New(&http.Client{Timeout: 8 * time.Second})
	authClient := upstreamauth.New(&http.Client{Timeout: 20 * time.Second})
	upstreamConfigurations := upstreamconfig.New(
		businessStore,
		privateStore,
		authClient,
		managementTasks,
	)
	upstreamSyncTasks := upstreamsync.New(
		businessStore,
		privateStore,
		upstreamReader,
		authClient,
		taskStore,
		managementTasks,
	)
	upstreamDeleteService := upstreamdelete.New(businessStore, privateStore, taskStore)
	accountDeleteService := accountdelete.New(businessStore, privateStore, upstreamReader, taskStore)
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
	notificationTargetDiscovery.UseTaskRunner(backgroundTasks)
	alertTasks.UseTaskRunner(backgroundTasks)
	accountTasks.UseTaskRunner(backgroundTasks)
	managementTasks.UseTaskRunner(backgroundTasks)
	pricingTasks.UseTaskRunner(backgroundTasks)
	probeTasks.UseTaskRunner(backgroundTasks)
	modelChecks.UseTaskRunner(backgroundTasks)
	upstreamSyncTasks.UseTaskRunner(backgroundTasks)
	upstreamDeleteService.UseTaskRunner(backgroundTasks)
	accountDeleteService.UseTaskRunner(backgroundTasks)
	onboardingService.UseTaskRunner(backgroundTasks)
	authRecoveryService.UseTaskRunner(backgroundTasks)
	pricingTasks.UseAuthResolver(authRecoveryService)
	upstreamSyncTasks.SetAuthResolver(authRecoveryService)
	managementTasks.UseUpstreamAuthResolver(authRecoveryService)
	accountDeleteService.SetAuthResolver(authRecoveryService)
	modelChecks.UseUpstreamAuthResolver(authRecoveryService)
	logService := consolelogs.New(businessStore, taskStore)
	logService.UseTaskRunner(backgroundTasks)
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
		return err
	}
	inspectionScheduler.UseTaskRunner(backgroundTasks)
	if err := logMaintenance.Start(serviceContext); err != nil {
		return err
	}
	logMaintenanceStarted := true
	defer func() {
		if logMaintenanceStarted {
			logMaintenance.Stop()
		}
	}()
	if err := inspectionScheduler.Start(serviceContext); err != nil {
		return err
	}
	inspectionSchedulerStarted := true
	defer func() {
		if inspectionSchedulerStarted {
			inspectionScheduler.Stop()
		}
	}()
	manualInspections := inspection.NewManualService(inspectionScheduler, inspectionRunner, taskStore)
	manualInspections.UseTaskRunner(backgroundTasks)
	newAPIManagement := newapimanagement.New(
		privateStore,
		businessStore,
		&http.Client{Timeout: 20 * time.Second},
	)

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
		AccountDelete:      accountDeleteService,
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
		NewAPIManagement:   newAPIManagement,
	})
	tcpListener, err := net.Listen("tcp", cfg.ListenAddress)
	if err != nil {
		return err
	}
	defer tcpListener.Close()
	servers := []httpServeTarget{{
		name:     cfg.ListenAddress,
		server:   newHTTPServer(cfg.ListenAddress, handler),
		listener: tcpListener,
	}}
	if cfg.TrustedProxySocket != "" {
		proxyListener, err := listenTrustedProxySocket(cfg.TrustedProxySocket)
		if err != nil {
			return err
		}
		defer proxyListener.Close()
		servers = append(servers, httpServeTarget{
			name:     cfg.TrustedProxySocket,
			server:   newHTTPServer("", api.TrustedProxyHandler(handler)),
			listener: proxyListener,
		})
	}
	stopped, stopSignals := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stopSignals()
	serveErrors := make(chan error, len(servers))
	for _, target := range servers {
		go func() {
			err := target.server.Serve(target.listener)
			if errors.Is(err, http.ErrServerClosed) {
				err = nil
			}
			serveErrors <- err
		}()
		log.Printf("Sub2API Console Go API listening on %s", target.name)
	}
	remainingServers := len(servers)
	var serveErr error
	select {
	case serveErr = <-serveErrors:
		remainingServers--
	case <-stopped.Done():
	}

	shutdown, cancelShutdown := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelShutdown()
	httpShutdowns := make(chan error, len(servers))
	for _, target := range servers {
		go func() { httpShutdowns <- target.server.Shutdown(shutdown) }()
	}
	cancelServices()
	backgroundTasks.Cancel()
	schedulerErr := inspectionScheduler.StopContext(shutdown)
	inspectionSchedulerStarted = false
	if schedulerErr != nil {
		schedulerErr = fmt.Errorf("停止巡检调度器失败: %w", schedulerErr)
	}
	maintenanceErr := logMaintenance.StopContext(shutdown)
	logMaintenanceStarted = false
	if maintenanceErr != nil {
		maintenanceErr = fmt.Errorf("停止日志维护任务失败: %w", maintenanceErr)
	}
	taskErr := backgroundTasks.Shutdown(shutdown)
	var httpErr error
	for range servers {
		httpErr = errors.Join(httpErr, <-httpShutdowns)
	}
	for remainingServers > 0 {
		serveErr = errors.Join(serveErr, <-serveErrors)
		remainingServers--
	}
	if schedulerErr != nil || maintenanceErr != nil || taskErr != nil || httpErr != nil {
		// Do not explicitly close databases while a task or HTTP handler may still be finalizing.
		closeStores = false
	}
	return errors.Join(serveErr, httpErr, schedulerErr, maintenanceErr, taskErr)
}

type httpServeTarget struct {
	name     string
	server   *http.Server
	listener net.Listener
}

func listenTrustedProxySocket(path string) (net.Listener, error) {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o750); err != nil {
		return nil, fmt.Errorf("创建受信代理 socket 目录失败: %w", err)
	}
	lock, err := lockTrustedProxySocket(path)
	if err != nil {
		return nil, err
	}
	keepLock := false
	defer func() {
		if !keepLock {
			_ = lock.Close()
		}
	}()
	if identity, err := openSocketIdentity(path); err == nil {
		defer identity.Close()
		info, statErr := identity.Stat()
		if statErr != nil {
			return nil, fmt.Errorf("检查受信代理 socket 失败: %w", statErr)
		}
		if info.Mode()&os.ModeSocket == 0 {
			return nil, errors.New("受信代理 socket 路径已被非 socket 文件占用")
		}
		connection, dialErr := net.DialTimeout("unix", path, 200*time.Millisecond)
		if dialErr == nil {
			_ = connection.Close()
			return nil, errors.New("受信代理 socket 已由其他进程监听")
		}
		if !errors.Is(dialErr, syscall.ECONNREFUSED) {
			return nil, fmt.Errorf("无法确认旧受信代理 socket 是否失活: %w", dialErr)
		}
		removed, err := removeSocketIfSame(path, identity)
		if err != nil {
			return nil, fmt.Errorf("清理旧受信代理 socket 失败: %w", err)
		}
		if !removed {
			return nil, errors.New("受信代理 socket 在失活检查期间发生变化")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("检查受信代理 socket 失败: %w", err)
	}
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		return nil, fmt.Errorf("监听受信代理 socket 失败: %w", err)
	}
	listener.SetUnlinkOnClose(false)
	identity, err := openSocketIdentity(path)
	if err != nil {
		_ = listener.Close()
		return nil, fmt.Errorf("记录受信代理 socket 身份失败: %w", err)
	}
	info, err := identity.Stat()
	if err != nil {
		_ = identity.Close()
		_ = listener.Close()
		return nil, fmt.Errorf("记录受信代理 socket 身份失败: %w", err)
	}
	if info.Mode()&os.ModeSocket == 0 {
		_ = identity.Close()
		_ = listener.Close()
		return nil, errors.New("受信代理 socket 路径在监听期间发生变化")
	}
	if err := os.Chmod(path, 0o660); err != nil {
		_ = listener.Close()
		_, _ = removeSocketIfSame(path, identity)
		_ = identity.Close()
		return nil, fmt.Errorf("设置受信代理 socket 权限失败: %w", err)
	}
	keepLock = true
	return &trustedProxySocketListener{
		UnixListener: listener,
		path:         path,
		identity:     identity,
		lock:         lock,
	}, nil
}

type trustedProxySocketListener struct {
	*net.UnixListener
	path     string
	identity *os.File
	lock     *os.File

	closeOnce sync.Once
	closeErr  error
}

func (l *trustedProxySocketListener) Close() error {
	l.closeOnce.Do(func() {
		listenerErr := l.UnixListener.Close()
		_, cleanupErr := removeSocketIfSame(l.path, l.identity)
		identityErr := l.identity.Close()
		// Closing the descriptor releases the cross-process flock. The lock file
		// intentionally remains so concurrent processes always lock one inode.
		lockErr := l.lock.Close()
		l.closeErr = errors.Join(listenerErr, cleanupErr, identityErr, lockErr)
	})
	return l.closeErr
}

func lockTrustedProxySocket(path string) (*os.File, error) {
	lockPath := path + ".lock"
	fd, err := syscall.Open(
		lockPath,
		syscall.O_CREAT|syscall.O_RDWR|syscall.O_CLOEXEC|syscall.O_NOFOLLOW,
		0o600,
	)
	if err != nil {
		return nil, fmt.Errorf("打开受信代理 socket 锁失败: %w", err)
	}
	lock := os.NewFile(uintptr(fd), lockPath)
	info, err := lock.Stat()
	if err != nil {
		_ = lock.Close()
		return nil, fmt.Errorf("检查受信代理 socket 锁失败: %w", err)
	}
	if !info.Mode().IsRegular() {
		_ = lock.Close()
		return nil, errors.New("受信代理 socket 锁路径已被非普通文件占用")
	}
	if err := syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = lock.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return nil, errors.New("受信代理 socket 正由其他进程管理")
		}
		return nil, fmt.Errorf("锁定受信代理 socket 失败: %w", err)
	}
	return lock, nil
}

func openSocketIdentity(path string) (*os.File, error) {
	fd, err := unix.Open(path, unix.O_PATH|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), path), nil
}

func removeSocketIfSame(path string, identity *os.File) (bool, error) {
	expected, err := identity.Stat()
	if err != nil {
		return false, err
	}
	current, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !os.SameFile(current, expected) {
		return false, nil
	}
	if err := os.Remove(path); err != nil {
		return false, err
	}
	return true, nil
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
