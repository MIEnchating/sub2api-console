import { SetupPage } from "@/features/setup/components/setup-page";
export { SetupPage } from "@/features/setup/components/setup-page";
import { LoginPage } from "@/features/auth/components/login-page";
import { StartupLoading } from "@/components/startup-loading";
import { ContentLoading } from "@/components/content-loading";
import { AlertBellButton } from "@/features/alerts/components/alert-bell-button";
import { useCallback, useEffect, useState } from "react";
import {
  Activity,
  Files,
  PanelsTopLeft,
  BadgeDollarSign,
  ChartSpline,
  ChartNoAxesColumnIncreasing,
  ChartNoAxesCombined,
  CircleDollarSign,
  FileSearch,
  Fingerprint,
  GitCompareArrows,
  HeartPulse,
  KeyRound,
  Layers3,
  ListChecks,
  Link2,
  ListTodo,
  LogOut,
  Moon,
  Network,
  Pause,
  Play,
  RadioTower,
  RefreshCw,
  Route,
  ScanSearch,
  ScrollText,
  Settings,
  ShieldAlert,
  Siren,
  SlidersHorizontal,
  Sun,
  UserRound,
  UserPlus,
  UsersRound,
} from "lucide-react";
import type { LucideIcon } from "lucide-react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, Outlet, useLocation, useNavigate, useRouterState } from "@tanstack/react-router";
import { toast } from "sonner";
import { api, type AutoInspectionStatus, type SessionStatus } from "./api";
import { Button } from "./components/ui/button";
import { Card } from "./components/ui/card";
import { TaskCancelButton } from "./components/task-startup-state";
import { Tooltip, TooltipContent, TooltipTrigger } from "./components/ui/tooltip";
import {
  Sidebar,
  SidebarContent,
  SidebarGroup,
  SidebarGroupLabel,
  SidebarInset,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarProvider,
  SidebarTrigger,
  useSidebar,
} from "./components/ui/sidebar";
import { taskIsPending, taskPollInterval } from "./lib/task-state";
import { terminalRefreshKeys } from "./lib/task-refresh";
import { notifyOperationError } from "./lib/operation-feedback";
import {
  advanceSessionGeneration,
  sessionExpiredEvent,
  sessionExpiredMessage,
} from "./lib/session-auth";
import { cn } from "./lib/utils";
import { QueryErrorToast } from "./components/query-error-toast";
import { type NavigationSettingsSection } from "./features/config/components/navigation-settings-card";
import { visibleNavigationSections } from "./lib/navigation-preferences";
import { useNavigationPreferences } from "./hooks/use-navigation-preferences";
import { useThemePreference } from "./hooks/use-theme-preference";
import { AppShellContext } from "./app-shell-context";
import { formatDate } from "./lib/date-format";
import { autoInspectionStatusSchema } from "./features/auto-inspection/lib/status-schema";

export type View =
  | "overview"
  | "accounts"
  | "account-workbench"
  | "upstreams"
  | "groups"
  | "uptime-kuma-templates"
  | "uptime-kuma-status-pages"
  | "uptime-kuma-config"
  | "uptime-kuma"
  | "newapi"
  | "newapi-groups"
  | "newapi-channels"
  | "newapi-account-automation"
  | "newapi-prices"
  | "newapi-differences"
  | "pricing"
  | "revenue-analysis"
  | "pricing-config"
  | "auto-inspection"
  | "model-check"
  | "animation-check"
  | "traffic"
  | "logs"
  | "alerts"
  | "alert-policy"
  | "onboarding"
  | "trace"
  | "vault"
  | "system-info"
  | "profile"
  | "config"
  | "policy";

export const navItems: Array<{
  id: View;
  label: string;
  icon: LucideIcon;
  to:
    | "/"
    | "/accounts"
    | "/account-workbench"
    | "/upstreams"
    | "/groups"
    | "/uptime-kuma/templates"
    | "/uptime-kuma/status-pages"
    | "/uptime-kuma/config"
    | "/uptime-kuma"
    | "/newapi"
    | "/newapi/groups"
    | "/newapi/channels"
    | "/newapi/account-model-automation"
    | "/newapi/prices"
    | "/newapi/differences"
    | "/pricing"
    | "/revenue-analysis"
    | "/pricing-config"
    | "/auto-inspection"
    | "/model-check"
    | "/animation-check"
    | "/traffic"
    | "/logs"
    | "/alerts"
    | "/alert-policy"
    | "/onboarding"
    | "/trace"
    | "/vault"
    | "/system-info"
    | "/profile"
    | "/config"
    | "/policy";
}> = [
  { id: "overview", label: "运营总览", icon: ChartSpline, to: "/" },
  { id: "upstreams", label: "上游管理", icon: Network, to: "/upstreams" },
  { id: "groups", label: "分组管理", icon: Layers3, to: "/groups" },
  { id: "pricing", label: "价格管理", icon: CircleDollarSign, to: "/pricing" },
  {
    id: "revenue-analysis",
    label: "收益分析",
    icon: ChartNoAxesCombined,
    to: "/revenue-analysis",
  },
  { id: "accounts", label: "账号管理", icon: UsersRound, to: "/accounts" },
  { id: "account-workbench", label: "账号工作台", icon: UserPlus, to: "/account-workbench" },
  {
    id: "auto-inspection",
    label: "自动巡检",
    icon: HeartPulse,
    to: "/auto-inspection",
  },
  {
    id: "model-check",
    label: "模型检测",
    icon: Fingerprint,
    to: "/model-check",
  },
  { id: "animation-check", label: "智商检测", icon: ScanSearch, to: "/animation-check" },
  {
    id: "traffic",
    label: "流量排行",
    icon: ChartNoAxesColumnIncreasing,
    to: "/traffic",
  },
  { id: "trace", label: "请求查询", icon: FileSearch, to: "/trace" },
  { id: "alerts", label: "告警通知", icon: Siren, to: "/alerts" },
  { id: "newapi-groups", label: "分组绑定", icon: Link2, to: "/newapi/groups" },
  { id: "newapi-channels", label: "渠道管理", icon: RadioTower, to: "/newapi/channels" },
  {
    id: "newapi-account-automation",
    label: "账号模型",
    icon: ListChecks,
    to: "/newapi/account-model-automation",
  },
  { id: "newapi-prices", label: "模型价格", icon: BadgeDollarSign, to: "/newapi/prices" },
  {
    id: "newapi-differences",
    label: "价格比对",
    icon: GitCompareArrows,
    to: "/newapi/differences",
  },
  { id: "uptime-kuma", label: "监控管理", icon: Activity, to: "/uptime-kuma" },
  {
    id: "uptime-kuma-templates",
    label: "功能模板",
    icon: Files,
    to: "/uptime-kuma/templates",
  },
  {
    id: "uptime-kuma-status-pages",
    label: "状态页管理",
    icon: PanelsTopLeft,
    to: "/uptime-kuma/status-pages",
  },
  {
    id: "pricing-config",
    label: "价格配置",
    icon: SlidersHorizontal,
    to: "/pricing-config",
  },
  {
    id: "policy",
    label: "调度策略",
    icon: Route,
    to: "/policy",
  },
  {
    id: "alert-policy",
    label: "告警策略",
    icon: ShieldAlert,
    to: "/alert-policy",
  },
  { id: "system-info", label: "系统信息", icon: ListTodo, to: "/system-info" },
  { id: "vault", label: "密码箱", icon: KeyRound, to: "/vault" },
  { id: "logs", label: "日志中心", icon: ScrollText, to: "/logs" },
  { id: "config", label: "系统设置", icon: Settings, to: "/config" },
];

export const navSections: Array<{ label: string; itemIDs: View[] }> = [
  {
    label: "运营管理",
    itemIDs: [
      "overview",
      "upstreams",
      "groups",
      "pricing",
      "revenue-analysis",
      "accounts",
      "account-workbench",
      "auto-inspection",
      "model-check",
      "animation-check",
      "traffic",
      "trace",
      "alerts",
    ],
  },
  {
    label: "New API",
    itemIDs: [
      "newapi-groups",
      "newapi-channels",
      "newapi-account-automation",
      "newapi-prices",
      "newapi-differences",
    ],
  },
  {
    label: "Uptime Kuma",
    itemIDs: ["uptime-kuma", "uptime-kuma-templates", "uptime-kuma-status-pages"],
  },
  { label: "策略配置", itemIDs: ["pricing-config", "policy", "alert-policy"] },
  { label: "系统管理", itemIDs: ["system-info", "vault", "logs", "config"] },
];

const navigationItemIDs = navItems.map((item) => item.id);
export const lockedNavigationItemIDs = new Set<View>(["config"]);
export const emptyHiddenNavigationItemIDs = new Set<View>();
export const navigationSettingsSections: Array<NavigationSettingsSection<View>> = navSections.map(
  (section) => ({
    label: section.label,
    items: section.itemIDs.flatMap((itemID) => {
      const item = navItems.find((candidate) => candidate.id === itemID);
      return item ? [{ id: item.id, label: item.label, path: item.to }] : [];
    }),
  }),
);

const viewByPath: Record<string, View> = {
  "/": "overview",
  "/accounts": "accounts",
  "/account-workbench": "account-workbench",
  "/upstreams": "upstreams",
  "/groups": "groups",
  "/uptime-kuma/config": "config",
  "/uptime-kuma/templates": "uptime-kuma-templates",
  "/uptime-kuma/status-pages": "uptime-kuma-status-pages",
  "/uptime-kuma": "uptime-kuma",
  "/newapi": "config",
  "/newapi/groups": "newapi-groups",
  "/newapi/channels": "newapi-channels",
  "/newapi/account-model-automation": "newapi-account-automation",
  "/newapi/prices": "newapi-prices",
  "/newapi/differences": "newapi-differences",
  "/pricing": "pricing",
  "/revenue-analysis": "revenue-analysis",
  "/pricing-config": "pricing-config",
  "/auto-inspection": "auto-inspection",
  "/model-check": "model-check",
  "/animation-check": "animation-check",
  "/traffic": "traffic",
  "/logs": "logs",
  "/alerts": "alerts",
  "/alert-policy": "alert-policy",
  "/onboarding": "onboarding",
  "/trace": "trace",
  "/vault": "vault",
  "/system-info": "system-info",
  "/profile": "profile",
  "/config": "config",
  "/policy": "policy",
};

export function viewForPath(pathname: string): View {
  return viewByPath[pathname.replace(/\/+$/, "") || "/"] ?? "overview";
}

export default function App() {
  const queryClient = useQueryClient();
  const [loginReason, setLoginReason] = useState<string | null>(null);
  const { theme, toggleTheme } = useThemePreference();
  const location = useLocation();
  const navigate = useNavigate();
  const navigationLoading = useRouterState({
    select: (state) =>
      state.isLoading && state.location.pathname !== state.resolvedLocation?.pathname,
  });
  const resetSession = useCallback(
    (value: SessionStatus = { authenticated: false, username: null }): void => {
      advanceSessionGeneration();
      void queryClient.cancelQueries();
      queryClient.removeQueries({
        predicate: (query) => !["setup-status", "session"].includes(String(query.queryKey[0])),
      });
      queryClient.getMutationCache().clear();
      queryClient.setQueryData(["session"], value);
    },
    [queryClient],
  );
  useEffect(() => {
    const handleSessionExpired = () => {
      resetSession();
      setLoginReason(sessionExpiredMessage);
    };
    window.addEventListener(sessionExpiredEvent, handleSessionExpired);
    return () => window.removeEventListener(sessionExpiredEvent, handleSessionExpired);
  }, [resetSession]);
  const logout = useMutation({
    mutationFn: api.logout,
    networkMode: "always",
    retry: false,
    onSuccess: () => {
      resetSession();
      setLoginReason(null);
    },
    onError: (error) => notifyOperationError(error, "退出登录失败"),
  });
  const setup = useQuery({
    queryKey: ["setup-status"],
    queryFn: ({ signal }) => api.setupStatus(signal),
    retry: false,
  });
  const session = useQuery({
    queryKey: ["session"],
    queryFn: ({ signal }) => api.session(signal),
    enabled: setup.data?.initialized === true,
    retry: false,
  });
  const shellContext = useNavigationPreferences(
    session.data?.authenticated === true,
    navigationItemIDs,
    lockedNavigationItemIDs,
  );
  const hiddenNavigationItemIDs = shellContext.hiddenNavigationItemIDs;
  const autoInspectionConnected = useAutoInspectionEvents(session.data?.authenticated === true);

  if (setup.isLoading) return <StartupLoading label="正在读取初始化状态…" />;
  if (setup.error)
    return (
      <>
        <QueryErrorToast error={setup.error} fallback="控制台 API 不可用" />
        <StartupState text="Sub2API Console" onRetry={() => void setup.refetch()} />
      </>
    );
  if (setup.data?.configuration_errors?.length)
    return (
      <StartupState text={`初始化配置无效：${setup.data.configuration_errors.join("、")}`} error />
    );
  if (!setup.data?.initialized)
    return (
      <SetupPage
        status={setup.data}
        onComplete={(status) => {
          void queryClient.cancelQueries({ queryKey: ["setup-status"], exact: true });
          queryClient.setQueryData(["setup-status"], status);
        }}
      />
    );
  if (session.isLoading) return <StartupLoading label="正在验证登录状态…" />;
  if (session.error)
    return (
      <>
        <QueryErrorToast error={session.error} fallback="登录状态读取失败" />
        <StartupState text="Sub2API Console" onRetry={() => void session.refetch()} />
      </>
    );
  if (!session.data?.authenticated)
    return (
      <LoginPage
        reason={loginReason}
        theme={theme}
        onThemeChange={toggleTheme}
        onLogin={(value) => {
          setLoginReason(null);
          resetSession(value);
        }}
      />
    );

  const activeView = viewForPath(location.pathname);
  const displayedNavigationSections = visibleNavigationSections(
    navSections,
    hiddenNavigationItemIDs,
  );
  return (
    <>
      <SidebarProvider className="h-svh max-h-svh flex-col overflow-clip" defaultOpen>
        <header className="sticky top-0 z-40 h-[var(--app-header-height)] w-full shrink-0 bg-transparent">
          <div className="flex h-full items-center gap-1 px-2 sm:gap-2 sm:px-3">
            <SidebarTrigger variant="ghost" className="size-8" />
            <Link
              to="/"
              aria-label="Sub2API"
              className="text-foreground inline-flex h-7 shrink-0 items-center gap-1.5 rounded-md px-1.5 text-sm font-medium transition-colors hover:bg-accent"
            >
              <img
                src="/console-mark.svg"
                width={20}
                height={20}
                alt=""
                className="size-5 shrink-0"
              />
              <span className="hidden sm:inline">Sub2API</span>
            </Link>
            <div className="ml-auto flex min-w-0 items-center overflow-x-auto sm:gap-2">
              <SchedulerHeaderControls realtimeConnected={autoInspectionConnected} />
              <AlertBellButton onClick={() => void navigate({ to: "/alerts" })} />
              <Tooltip>
                <TooltipTrigger
                  render={
                    <Button
                      variant="ghost"
                      size="icon"
                      aria-label={theme === "dark" ? "切换亮色主题" : "切换暗色主题"}
                      onClick={toggleTheme}
                    />
                  }
                >
                  {theme === "dark" ? <Sun size={16} /> : <Moon size={16} />}
                </TooltipTrigger>
                <TooltipContent>
                  {theme === "dark" ? "切换亮色主题" : "切换暗色主题"}
                </TooltipContent>
              </Tooltip>
              <Button
                variant="ghost"
                size="icon"
                className="gap-1.5 sm:w-auto sm:max-w-44 sm:px-2.5"
                aria-label="个人资料"
                onClick={() => void navigate({ to: "/profile" })}
              >
                <UserRound size={15} />
                <span className="hidden truncate sm:inline">{session.data?.username}</span>
              </Button>
              <Tooltip>
                <TooltipTrigger
                  render={
                    <Button
                      variant="ghost"
                      size="icon"
                      aria-label="退出登录"
                      disabled={logout.isPending}
                      aria-busy={logout.isPending}
                      onClick={() => {
                        if (!logout.isPending) logout.mutate();
                      }}
                    />
                  }
                >
                  <LogOut size={16} />
                </TooltipTrigger>
                <TooltipContent>退出登录</TooltipContent>
              </Tooltip>
            </div>
          </div>
        </header>
        <div className="flex min-h-0 w-full flex-1">
          <ConsoleSidebar activeView={activeView} sections={displayedNavigationSections} />
          <SidebarInset className="@container/content h-[calc(100svh-var(--app-header-height,0px))] min-h-0 overflow-hidden peer-data-[variant=inset]:h-[calc(100svh-var(--app-header-height,0px)-(var(--spacing)*4))]">
            <div className="relative flex min-h-0 w-full min-w-0 flex-1 overflow-hidden">
              <div
                key={activeView}
                className={cn(
                  "console-page flex min-h-0 w-full min-w-0 flex-1 flex-col overflow-hidden",
                  navigationLoading && "invisible",
                )}
              >
                <AppShellContext.Provider value={{ ...shellContext, autoInspectionConnected }}>
                  <Outlet />
                </AppShellContext.Provider>
              </div>
              {navigationLoading && (
                <div className="bg-background/95 absolute inset-0 z-10 flex items-center justify-center">
                  <ContentLoading label="正在切换页面" />
                </div>
              )}
            </div>
          </SidebarInset>
        </div>
      </SidebarProvider>
    </>
  );
}

function ConsoleSidebar(props: {
  activeView: View;
  sections: Array<{ label: string; itemIDs: View[] }>;
}) {
  const sidebar = useSidebar();
  return (
    <Sidebar variant="inset" collapsible="icon">
      <SidebarContent className="py-2">
        {props.sections.map((section) => (
          <SidebarGroup key={section.label} className="px-2 py-1">
            <SidebarGroupLabel className="text-muted-foreground/70 px-2 text-[11px] font-medium tracking-wider uppercase">
              {section.label}
            </SidebarGroupLabel>
            <SidebarMenu>
              {section.itemIDs.map((itemID) => {
                const item = navItems.find((candidate) => candidate.id === itemID);
                if (!item) return null;
                const Icon = item.icon;
                return (
                  <SidebarMenuItem key={item.id}>
                    <SidebarMenuButton
                      isActive={props.activeView === item.id}
                      tooltip={item.label}
                      render={
                        <Link
                          to={item.to}
                          onClick={(event) => {
                            if (
                              sidebar.isMobile &&
                              event.button === 0 &&
                              !event.ctrlKey &&
                              !event.metaKey &&
                              !event.shiftKey &&
                              !event.altKey
                            )
                              sidebar.setOpenMobile(false);
                          }}
                        />
                      }
                    >
                      <Icon />
                      <span>{item.label}</span>
                    </SidebarMenuButton>
                  </SidebarMenuItem>
                );
              })}
            </SidebarMenu>
          </SidebarGroup>
        ))}
      </SidebarContent>
      <SchedulerSidebarStatus />
    </Sidebar>
  );
}

function useAutoInspectionEvents(enabled: boolean): boolean {
  const queryClient = useQueryClient();
  const [connected, setConnected] = useState(false);
  useEffect(() => {
    if (!enabled || typeof EventSource === "undefined") {
      setConnected(false);
      return;
    }
    const source = new EventSource(api.autoInspectionEventsURL(), {
      withCredentials: true,
    });
    // A connected stream is not yet a usable source of status. Keep polling
    // until a valid event arrives, including after reconnecting.
    source.onopen = () => setConnected(false);
    source.onerror = () => setConnected(false);
    const updateStatus = (event: MessageEvent<string>) => {
      try {
        const status = autoInspectionStatusSchema.parse(JSON.parse(event.data));
        // An HTTP poll started earlier must not overwrite this newer push.
        void queryClient.cancelQueries({ queryKey: ["auto-inspection"], exact: true });
        queryClient.setQueryData<AutoInspectionStatus>(["auto-inspection"], status);
        setConnected(true);
      } catch {
        // Preserve the last good snapshot and restore the HTTP polling fallback.
        setConnected(false);
      }
    };
    source.addEventListener("status", updateStatus as EventListener);
    return () => {
      source.close();
      setConnected(false);
    };
  }, [enabled, queryClient]);
  return connected;
}

export function SchedulerHeaderControls(props: { realtimeConnected?: boolean }) {
  const queryClient = useQueryClient();
  const status = useQuery({
    queryKey: ["auto-inspection"],
    queryFn: ({ signal }) => api.autoInspection(signal),
    staleTime: 15_000,
    refetchInterval: props.realtimeConnected ? false : 15_000,
  });
  const [syncTaskId, setSyncTaskId] = useState<string | null>(null);
  const [runTaskId, setRunTaskId] = useState<string | null>(null);
  const sync = useMutation({
    mutationFn: api.syncManagement,
    onSuccess: (task) => {
      setSyncTaskId(task.id);
      toast.success("账号与分组同步已开始");
    },
    onError: (error) => notifyOperationError(error, "同步启动失败"),
  });
  const run = useMutation({
    mutationFn: () => api.runInspection(),
    onSuccess: (task) => {
      setRunTaskId(task.id);
      void status.refetch();
    },
    onError: (error) => notifyOperationError(error, "巡检启动失败"),
  });
  const toggle = useMutation({
    mutationFn: async () => {
      if (status.data?.enabled) {
        return (await api.cancelAutoInspection()).status;
      }
      return api.resumeAutoInspection();
    },
    onSuccess: (value) => {
      queryClient.setQueryData(["auto-inspection"], value);
      toast.success(value.enabled ? "自动巡检已启动" : "自动巡检已停止");
    },
    onError: (error) => notifyOperationError(error, "调度状态更新失败"),
  });
  const syncTask = useQuery({
    queryKey: ["header-management-sync", syncTaskId],
    queryFn: ({ signal }) => api.task(syncTaskId!, signal),
    enabled: Boolean(syncTaskId),
    refetchInterval: taskPollInterval,
  });
  const runTask = useQuery({
    queryKey: ["header-inspection", runTaskId],
    queryFn: ({ signal }) => api.task(runTaskId!, signal),
    enabled: Boolean(runTaskId),
    refetchInterval: taskPollInterval,
  });
  useEffect(() => {
    for (const key of terminalRefreshKeys("management-sync", syncTask.data)) {
      void queryClient.invalidateQueries({ queryKey: key });
    }
  }, [queryClient, syncTask.data]);
  useEffect(() => {
    for (const key of terminalRefreshKeys("inspection", runTask.data)) {
      void queryClient.invalidateQueries({ queryKey: key });
    }
    if (
      runTask.data &&
      ["succeeded", "partial", "failed", "cancelled"].includes(runTask.data.status)
    ) {
      void queryClient.invalidateQueries({ queryKey: ["auto-inspection"] });
    }
  }, [queryClient, runTask.data]);
  const syncing = sync.isPending || taskIsPending(syncTaskId, syncTask);
  const executing =
    run.isPending || taskIsPending(runTaskId, runTask) || status.data?.running === true;
  const schedulingEnabled = status.data?.enabled === true;
  let runtimeLabel = "等待首次调度";
  if (status.data?.running) runtimeLabel = "调度中";
  else if (status.data?.last_run_at)
    runtimeLabel = `上次 ${formatRelativeRun(status.data.last_run_at)}`;
  let toggleLabel = "启动调度";
  if (toggle.isPending) toggleLabel = "处理中";
  else if (schedulingEnabled) toggleLabel = "取消调度";
  let statusDotClassName = "bg-muted-foreground/60";
  if (status.isError) statusDotClassName = "bg-destructive";
  else if (status.isLoading) statusDotClassName = "animate-pulse bg-muted-foreground";
  else if (status.data?.running) statusDotClassName = "animate-pulse bg-amber-500";
  else if (status.data?.enabled) statusDotClassName = "bg-emerald-500";
  let statusLabel = runtimeLabel;
  if (status.isError) statusLabel = "状态读取失败";
  else if (status.isLoading) statusLabel = "读取状态";
  let statusDescription = "尚未执行巡检";
  if (status.isError) statusDescription = "自动巡检状态接口暂时不可用";
  else if (status.data?.last_run_at) {
    statusDescription = `上次执行：${formatDate(status.data.last_run_at, true)}`;
  }
  return (
    <div className="flex items-center gap-1" data-testid="scheduler-header-controls">
      {syncTaskId && taskIsPending(syncTaskId, syncTask) ? (
        <TaskCancelButton taskId={syncTaskId} compact />
      ) : null}
      {runTaskId && taskIsPending(runTaskId, runTask) ? (
        <TaskCancelButton taskId={runTaskId} compact />
      ) : null}
      <Tooltip>
        <TooltipTrigger
          render={
            <span
              className="bg-muted text-muted-foreground hidden h-7 items-center gap-1.5 rounded-md px-2 text-xs lg:inline-flex"
              tabIndex={0}
            />
          }
        >
          <span className={cn("size-1.5 rounded-full", statusDotClassName)} />
          {statusLabel}
        </TooltipTrigger>
        <TooltipContent>{statusDescription}</TooltipContent>
      </Tooltip>
      <Tooltip>
        <TooltipTrigger render={<span className="inline-flex" />}>
          <Button
            variant="ghost"
            size="icon"
            className="gap-1.5 xl:w-auto xl:px-2.5"
            disabled={syncing}
            aria-label={syncing ? "正在同步账号与分组" : "同步账号与分组"}
            onClick={() => sync.mutate()}
          >
            <RefreshCw size={15} className={syncing ? "animate-spin" : undefined} />
            <span className="hidden xl:inline">{syncing ? "同步中" : "同步"}</span>
          </Button>
        </TooltipTrigger>
        <TooltipContent>同步 Sub2API 账号与分组目录</TooltipContent>
      </Tooltip>
      <Tooltip>
        <TooltipTrigger render={<span className="inline-flex" />}>
          <Button
            variant="ghost"
            size="icon"
            className="gap-1.5 xl:w-auto xl:px-2.5"
            disabled={executing}
            aria-label={executing ? "巡检执行中" : "立即检查一轮到期任务"}
            onClick={() => run.mutate()}
          >
            <Play size={15} />
            <span className="hidden xl:inline">{executing ? "执行中" : "跑一轮"}</span>
          </Button>
        </TooltipTrigger>
        <TooltipContent>立即按当前策略检查一次，只执行已到期任务</TooltipContent>
      </Tooltip>
      <Tooltip>
        <TooltipTrigger render={<span className="inline-flex" />}>
          <Button
            variant={schedulingEnabled ? "destructive" : "default"}
            size="icon"
            className="gap-1.5 2xl:w-auto 2xl:px-2.5"
            disabled={!status.data || toggle.isPending}
            aria-label={schedulingEnabled ? "取消自动调度" : "启动自动调度"}
            onClick={() => toggle.mutate()}
          >
            {schedulingEnabled ? <Pause size={15} /> : <Play size={15} />}
            <span className="hidden 2xl:inline">{toggleLabel}</span>
          </Button>
        </TooltipTrigger>
        <TooltipContent>
          {schedulingEnabled ? "中断当前巡检并停止后续自动调度" : "启动自动巡检"}
        </TooltipContent>
      </Tooltip>
    </div>
  );
}

function formatRelativeRun(value: string): string {
  const elapsed = Math.max(0, Math.floor((Date.now() - new Date(value).getTime()) / 1_000));
  if (elapsed < 60) return `${elapsed}秒前`;
  if (elapsed < 3_600) return `${Math.floor(elapsed / 60)}分钟前`;
  if (elapsed < 86_400) return `${Math.floor(elapsed / 3_600)}小时前`;
  return `${Math.floor(elapsed / 86_400)}天前`;
}

function trafficCollectionState(value?: AutoInspectionStatus, failed = false) {
  if (failed) {
    return {
      label: "真实流量状态读取失败",
      shortLabel: "读取失败",
      tone: "danger" as const,
    };
  }
  if (!value) {
    return {
      label: "真实流量状态读取中",
      shortLabel: "读取中",
      tone: "neutral" as const,
    };
  }
  if (!value.monitoring_configured) {
    return {
      label: "真实流量采集已关闭",
      shortLabel: "采集已关闭",
      tone: "neutral" as const,
    };
  }
  if (!value.monitoring_checked_at) {
    return {
      label: "真实流量随巡检同步",
      shortLabel: "随巡检同步",
      tone: "info" as const,
    };
  }
  if (value.monitoring_enabled) {
    return {
      label: "真实流量同步正常",
      shortLabel: "同步正常",
      tone: "success" as const,
    };
  }
  return {
    label: "本轮未获取真实流量",
    shortLabel: "本轮无样本",
    tone: "warning" as const,
  };
}

function trafficStateDot(tone: ReturnType<typeof trafficCollectionState>["tone"]): string {
  if (tone === "danger") return "bg-destructive";
  if (tone === "success") return "bg-emerald-500";
  if (tone === "warning") return "bg-amber-500";
  if (tone === "info") return "bg-sky-500";
  return "bg-muted-foreground/50";
}

export function SchedulerSidebarStatus() {
  const status = useQuery({
    queryKey: ["auto-inspection"],
    queryFn: ({ signal }) => api.autoInspection(signal),
    staleTime: 15_000,
    // SchedulerHeaderControls is mounted for every authenticated shell and is
    // the single polling owner when SSE is unavailable. This observer only
    // renders the shared cache and must not start a second timer after route
    // changes mount the sidebar at a different time.
    refetchInterval: false,
  });
  const trafficState = trafficCollectionState(status.data, status.isError);
  let inspectionDotClassName = "bg-muted-foreground/50";
  let inspectionLabel = "自动巡检已暂停";
  if (status.isError) {
    inspectionDotClassName = "bg-destructive";
    inspectionLabel = "自动巡检状态读取失败";
  } else if (status.data?.enabled) {
    inspectionDotClassName = "bg-emerald-500";
    inspectionLabel = "自动巡检中";
  }
  return (
    <div
      className="border-sidebar-border mt-auto min-w-0 space-y-2 overflow-hidden border-t px-4 py-3 group-data-[collapsible=icon]:hidden"
      data-testid="scheduler-sidebar-status"
    >
      <div className="flex h-4 min-w-0 items-center gap-2 overflow-hidden text-xs whitespace-nowrap">
        <span className={cn("size-2 shrink-0 rounded-full", inspectionDotClassName)} />
        <span className="min-w-0 truncate">{inspectionLabel}</span>
      </div>
      <div className="flex h-4 min-w-0 items-center gap-2 overflow-hidden text-xs whitespace-nowrap">
        <span className={cn("size-2 shrink-0 rounded-full", trafficStateDot(trafficState.tone))} />
        <span className="min-w-0 truncate">{trafficState.label}</span>
      </div>
    </div>
  );
}

function StartupState(props: { text: string; error?: boolean; onRetry?: () => void }) {
  return (
    <div className="bg-background text-foreground grid min-h-svh place-items-center p-6">
      <Card className={cn("w-full max-w-md p-6", props.error && "border-destructive")}>
        <div className="flex items-center gap-3">
          <img src="/console-mark.svg" width={36} height={36} alt="" className="size-9 shrink-0" />
          <div>
            <strong className="block text-sm">{props.text}</strong>
            {props.onRetry ? (
              <Button className="mt-3" variant="outline" onClick={props.onRetry}>
                重新连接
              </Button>
            ) : (
              <span className="text-muted-foreground mt-1 block text-xs">
                {props.error ? "请检查 API 地址和服务状态。" : "Sub2API Console"}
              </span>
            )}
          </div>
        </div>
      </Card>
    </div>
  );
}
