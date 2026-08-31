import * as React from "react";
import { useEffect, useMemo, useState } from "react";
import { motion, useReducedMotion } from "motion/react";
import {
  Activity,
  BellRing,
  BadgeCheck,
  ChartSpline,
  ChartNoAxesColumnIncreasing,
  ChartNoAxesCombined,
  Check,
  CircleAlert,
  CircleDollarSign,
  CircleHelp,
  CirclePlus,
  Eye,
  ExternalLink,
  FileSearch,
  Fingerprint,
  FolderOpen,
  Gauge,
  HeartPulse,
  KeyRound,
  Layers3,
  LogOut,
  Moon,
  MoreHorizontal,
  Network,
  Pause,
  Pencil,
  Play,
  RefreshCw,
  Route,
  Save,
  ScanSearch,
  Search,
  ScrollText,
  Server,
  Settings,
  ShieldAlert,
  ShieldCheck,
  ShieldOff,
  SpellCheck2,
  Siren,
  SlidersHorizontal,
  Sun,
  Trash2,
  UserRound,
  UserPlus,
  UsersRound,
  WalletCards,
  X,
} from "lucide-react";
import type { LucideIcon } from "lucide-react";
import { Popover } from "@base-ui/react/popover";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useLocation, useNavigate, useSearch } from "@tanstack/react-router";
import { zodResolver } from "@hookform/resolvers/zod";
import { Controller, useForm } from "react-hook-form";
import { toast } from "sonner";
import { z } from "zod";
import {
  api,
  type AccountControlAction,
  type AccountStatus,
  type AutoInspectionConfig,
  type AutoInspectionStatus,
  type GroupPolicyOverrideUpdate,
  type GroupStatus,
  type CaptchaChallenge,
  type ManualAuthVerifyResult,
  type NotificationStatus,
  type OnboardingCandidate,
  type OnboardingRequest,
  type PolicyUpdate,
  type RuntimeConfig,
  type RuntimeMode,
  type SetupStatus,
  type Task,
  type UpstreamConfiguration,
  type UpstreamSummary,
} from "./api";
import { Button } from "./components/ui/button";
import { Badge } from "./components/ui/badge";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "./components/ui/card";
import { Checkbox } from "./components/ui/checkbox";
import { Input } from "./components/ui/input";
import { Progress } from "./components/ui/progress";
import { TaskProgressState, TaskStartupState } from "./components/task-startup-state";
import { Textarea } from "./components/ui/textarea";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "./components/ui/select";
import { Skeleton } from "./components/ui/skeleton";
import { Switch } from "./components/ui/switch";
import { Tooltip, TooltipContent, TooltipTrigger } from "./components/ui/tooltip";
import { UpstreamIdentity } from "./features/upstreams/components/upstream-identity";
import { SystemLogSearchPanel } from "./features/request-trace/components/system-log-search-panel";
import { GroupAllocationDialog } from "./features/groups/components/group-allocation-dialog";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "./components/ui/sheet";
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  operationDialogHeight,
  operationDialogWidth,
} from "./components/ui/dialog";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "./components/ui/dropdown-menu";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "./components/ui/table";
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
} from "./components/ui/sidebar";
import { taskIsPending, taskPollInterval, taskStopsPolling } from "./lib/task-state";
import { terminalRefreshKeys, type TaskRefreshScope } from "./lib/task-refresh";
import { flattenTaskResult } from "./lib/task-result";
import { OverviewPage } from "./features/overview/components/overview-page";
import { AlertPolicyPage } from "./features/alert-policy/components/alert-policy-page";
import { PricingConfigPage, PricingPage } from "./features/pricing/components/pricing-page";
import { RevenueAnalysisPage } from "./features/pricing/components/revenue-analysis-page";
import {
  alertCauseLabel,
  alertDeliveryLabel,
  alertObjectLabel,
  alertStatusLabel,
  alertTypeLabel,
} from "./features/alerts/lib/alert-display";
import { AlertListActions } from "./features/alerts/components/alert-list-actions";
import { NotificationQueueStatus } from "./features/alerts/components/notification-queue-status";
import { LogsCenterPage } from "./features/logs/components/logs-center-page";
import { GroupPolicyEditorFields } from "./features/groups/components/group-policy-editor-fields";
import { captchaChallengeFromTask } from "./lib/captcha-challenge";
import { groupStatusMeta } from "./lib/group-policy-display";
import { schedulingMetric } from "./lib/scheduling-display";
import {
  schedulingStrategyDescription,
  schedulingStrategyOptions,
  schedulingWeightFormula,
} from "./lib/scheduling-strategy";
import { notifyOperationError } from "./lib/operation-feedback";
import { sensitiveFieldPlaceholder } from "./lib/sensitive-field";
import { sessionExpiredEvent, sessionExpiredMessage } from "./lib/session-auth";
import { cn } from "./lib/utils";
import { StatusBadge } from "./components/status-badge";
import { TableFilterToolbar } from "./components/data-table/filter-toolbar";
import { DataTablePagination } from "./components/data-table/pagination";
import { TableActionButton } from "./components/data-table/table-action-button";
import { SearchField } from "./components/data-table/search-field";
import { ConfirmActionDialog } from "./components/confirm-action-dialog";
import { OnboardingSelectionSkeleton } from "./components/onboarding-selection-skeleton";
import { QueryErrorToast } from "./components/query-error-toast";
import { PageActions } from "./components/page-actions";
import { PageHeading } from "./components/page-heading";
import { PageLayout } from "./components/page-layout";
import { ResultSummaryRow } from "./components/result-summary-row";
import { UpstreamEditDialog } from "./features/upstreams/components/upstream-edit-dialog";
import { UpstreamBoundAccountSelect } from "./features/upstreams/components/upstream-bound-account-select";
import { OnboardingHeadingActions } from "./features/upstreams/components/onboarding-heading-actions";
import { OnboardingMaintenanceActions } from "./features/upstreams/components/onboarding-maintenance-actions";
import { OnboardingGroupBindingSelect } from "./features/upstreams/components/onboarding-group-binding-select";
import {
  OnboardingConfirmDialog,
  type OnboardingBindingPreview,
} from "./features/upstreams/components/onboarding-confirm-dialog";
import {
  OnboardingProbeAction,
  type OnboardingProbeTarget,
} from "./features/upstreams/components/onboarding-probe-action";
import { VaultPage } from "./features/vault/components/vault-page";
import { ProfilePage } from "./features/profile/components/profile-page";
import { AccountStatusFilter } from "./features/accounts/components/account-status-tabs";
import { ManualPriorityDialog } from "./features/accounts/components/manual-priority-dialog";
import { AccountOperationButtons } from "./features/accounts/components/account-operation-buttons";
import { AccountDetailDialog } from "./features/accounts/components/account-detail-dialog";
import { AccountSettingsPanel } from "./features/accounts/components/account-settings-panel";
import {
  AccountProbeDialog,
  type ProbeDialogTarget,
} from "./features/accounts/components/account-probe-dialog";
import { BaseURLCheckResults } from "./features/accounts/components/base-url-check-results";
import { ModelCheckPage } from "./features/model-check/components/model-check-page";
import { TrafficRankingPage } from "./features/traffic-ranking/components/traffic-ranking-page";
import {
  AccountHealthCell,
  AccountIdentityCell,
  AccountKeyStatusCell,
  AccountSub2APIStatusCell,
  AccountLatencyCell,
  AccountRecentResultsCell,
  AccountRoutingParametersCell,
  AccountStateCell,
} from "./features/accounts/components/account-pool-cells";
import {
  accountMatchesPoolFilter,
  type AccountPoolFilter,
} from "./features/accounts/lib/account-pool";
import { accountTypeLabel, groupPlatformSummary } from "./features/accounts/lib/account-labels";
import {
  authModesForPlatform,
  parseStringMap,
} from "./features/upstreams/lib/upstream-edit-schema";
import { upstreamRateLabels } from "./features/upstreams/lib/upstream-rate-labels";
import {
  defaultVaultEntryForHost,
  vaultEntriesForHost,
  vaultEntryLabel,
} from "./lib/vault-entry-label";
import {
  adjacentOnboardingUpstreams,
  canSubmitOnboarding,
  candidateBoundAccountIDs,
  candidateBoundLocalGroupIDs,
  candidateBoundLocalGroups,
  candidateCanCreateKey,
  candidateCreationUnavailableReason,
  candidateHasExistingBinding,
  composeOnboardingBaseUrl,
  localGroupMultiplierLabel,
  normalizeOnboardingHost,
  onboardingCandidateStats,
  onboardingEntryDescription,
  onboardingEntryKind,
  onboardingRequestHost,
  onboardingSelectionTitle,
  parseOnboardingBaseUrl,
  rechargeRatioLabel,
  sameOnboardingGroupSelection,
} from "./lib/onboarding-entry";

type View =
  | "overview"
  | "accounts"
  | "upstreams"
  | "groups"
  | "pricing"
  | "revenue-analysis"
  | "pricing-config"
  | "auto-inspection"
  | "model-check"
  | "traffic"
  | "logs"
  | "alerts"
  | "alert-policy"
  | "onboarding"
  | "trace"
  | "vault"
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
    | "/upstreams"
    | "/groups"
    | "/pricing"
    | "/revenue-analysis"
    | "/pricing-config"
    | "/auto-inspection"
    | "/model-check"
    | "/traffic"
    | "/logs"
    | "/alerts"
    | "/alert-policy"
    | "/onboarding"
    | "/trace"
    | "/vault"
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
  {
    id: "traffic",
    label: "流量排行",
    icon: ChartNoAxesColumnIncreasing,
    to: "/traffic",
  },
  { id: "trace", label: "请求查询", icon: FileSearch, to: "/trace" },
  { id: "alerts", label: "告警通知", icon: Siren, to: "/alerts" },
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
      "auto-inspection",
      "model-check",
      "traffic",
      "trace",
      "alerts",
    ],
  },
  { label: "策略配置", itemIDs: ["pricing-config", "policy", "alert-policy"] },
  { label: "系统管理", itemIDs: ["vault", "logs", "config"] },
];

const viewByPath: Record<string, View> = {
  "/": "overview",
  "/accounts": "accounts",
  "/upstreams": "upstreams",
  "/groups": "groups",
  "/pricing": "pricing",
  "/revenue-analysis": "revenue-analysis",
  "/pricing-config": "pricing-config",
  "/auto-inspection": "auto-inspection",
  "/model-check": "model-check",
  "/traffic": "traffic",
  "/logs": "logs",
  "/alerts": "alerts",
  "/alert-policy": "alert-policy",
  "/onboarding": "onboarding",
  "/trace": "trace",
  "/vault": "vault",
  "/profile": "profile",
  "/config": "config",
  "/policy": "policy",
};

export function viewForPath(pathname: string): View {
  return viewByPath[pathname] ?? "overview";
}

function App() {
  const queryClient = useQueryClient();
  const [loginReason, setLoginReason] = useState<string | null>(null);
  const [theme, setTheme] = useState<"dark" | "light">(() => {
    if (typeof window === "undefined") return "dark";
    return window.localStorage.getItem("sub2api-console-theme") === "light" ? "light" : "dark";
  });
  const location = useLocation();
  const navigate = useNavigate();
  const shouldReduceMotion = useReducedMotion();
  useEffect(() => {
    document.documentElement.dataset.theme = theme;
    document.documentElement.classList.toggle("dark", theme === "dark");
    document.documentElement.style.colorScheme = theme;
    window.localStorage.setItem("sub2api-console-theme", theme);
  }, [theme]);
  useEffect(() => {
    const handleSessionExpired = () => {
      queryClient.removeQueries({
        predicate: (query) => !["setup-status", "session"].includes(String(query.queryKey[0])),
      });
      queryClient.setQueryData(["session"], {
        authenticated: false,
        username: null,
      });
      setLoginReason(sessionExpiredMessage);
    };
    window.addEventListener(sessionExpiredEvent, handleSessionExpired);
    return () => window.removeEventListener(sessionExpiredEvent, handleSessionExpired);
  }, [queryClient]);
  const setup = useQuery({
    queryKey: ["setup-status"],
    queryFn: api.setupStatus,
    retry: false,
  });
  const session = useQuery({
    queryKey: ["session"],
    queryFn: api.session,
    enabled: setup.data?.initialized === true,
    retry: false,
  });
  const overview = useQuery({
    queryKey: ["overview"],
    queryFn: api.overview,
    enabled: session.data?.authenticated === true,
    refetchInterval: 30_000,
  });
  useAutoInspectionEvents(session.data?.authenticated === true);

  if (setup.isLoading) return <StartupState text="正在读取初始化状态…" />;
  if (setup.error)
    return (
      <StartupState
        text={setup.error instanceof Error ? setup.error.message : "控制台 API 不可用"}
        error
      />
    );
  if (setup.data?.configuration_errors?.length)
    return (
      <StartupState text={`初始化配置无效：${setup.data.configuration_errors.join("、")}`} error />
    );
  if (!setup.data?.initialized)
    return (
      <SetupPage
        status={setup.data}
        onComplete={() => {
          void setup.refetch();
          void session.refetch();
        }}
      />
    );
  if (session.isLoading) return <StartupState text="正在验证登录状态…" />;
  if (session.error)
    return (
      <StartupState
        text={session.error instanceof Error ? session.error.message : "登录状态读取失败"}
        error
      />
    );
  if (!session.data?.authenticated)
    return (
      <LoginPage
        reason={loginReason}
        onLogin={() => {
          setLoginReason(null);
          void session.refetch();
        }}
      />
    );

  const activeView = viewForPath(location.pathname);
  return (
    <>
      <SidebarProvider className="h-svh max-h-svh flex-col overflow-hidden" defaultOpen>
        <header className="sticky top-0 z-40 h-[var(--app-header-height)] w-full shrink-0 bg-transparent">
          <div className="flex h-full items-center gap-1.5 px-2 sm:gap-2 sm:px-3">
            <SidebarTrigger variant="ghost" className="size-8" />
            <Link
              to="/"
              className="text-foreground inline-flex h-7 items-center gap-1.5 rounded-md px-1.5 text-sm font-medium transition-colors hover:bg-accent"
            >
              <span className="flex size-5 items-center justify-center overflow-hidden rounded-md bg-primary/15 text-primary">
                <Activity size={14} />
              </span>
              <span>Sub2API</span>
            </Link>
            <div className="ml-auto flex items-center gap-1.5 sm:gap-2">
              <SchedulerHeaderControls />
              <Button
                variant="ghost"
                size="sm"
                className="gap-1.5"
                onClick={() => void navigate({ to: "/alerts" })}
              >
                <BellRing size={15} />
                <span className="hidden sm:inline">告警</span>
                <Badge variant="destructive">
                  {overview.isError
                    ? "错误"
                    : overview.isLoading
                      ? "…"
                      : overview.data?.open_alerts}
                </Badge>
              </Button>
              <Tooltip>
                <TooltipTrigger
                  render={
                    <Button
                      variant="ghost"
                      size="icon-sm"
                      aria-label={theme === "dark" ? "切换亮色主题" : "切换暗色主题"}
                      onClick={() => setTheme(theme === "dark" ? "light" : "dark")}
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
                size="sm"
                className="max-w-44 gap-1.5"
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
                      size="icon-sm"
                      aria-label="退出登录"
                      onClick={async () => {
                        try {
                          await api.logout();
                          await session.refetch();
                        } catch (error) {
                          toast.error(error instanceof Error ? error.message : "退出登录失败");
                        }
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
          <ConsoleSidebar activeView={activeView} />
          <SidebarInset className="@container/content h-[calc(100svh-var(--app-header-height,0px))] min-h-0 overflow-hidden peer-data-[variant=inset]:h-[calc(100svh-var(--app-header-height,0px)-(var(--spacing)*4))]">
            <motion.div
              key={activeView}
              initial={shouldReduceMotion ? false : { opacity: 0, y: 8, filter: "blur(4px)" }}
              animate={{ opacity: 1, y: 0, filter: "blur(0px)" }}
              transition={{ duration: 0.15, ease: [0.33, 1, 0.68, 1] }}
              className="console-page flex min-h-0 w-full min-w-0 flex-1 flex-col overflow-hidden"
            >
              {activeView === "overview" && (
                <OverviewPage
                  onOpenAccounts={() =>
                    void navigate({
                      to: "/accounts",
                    })
                  }
                  onOpenEvents={() =>
                    void navigate({
                      to: "/logs",
                      search: { kind: "event" },
                    })
                  }
                  onOpenGroups={() =>
                    void navigate({
                      to: "/groups",
                    })
                  }
                />
              )}
              {activeView === "accounts" && <AccountsPage />}
              {activeView === "upstreams" && <UpstreamsPage />}
              {activeView === "groups" && <GroupsPage />}
              {activeView === "pricing" && <PricingPage />}
              {activeView === "revenue-analysis" && <RevenueAnalysisPage />}
              {activeView === "pricing-config" && <PricingConfigPage />}
              {activeView === "auto-inspection" && <AutoInspectionPage />}
              {activeView === "model-check" && <ModelCheckPage />}
              {activeView === "traffic" && <TrafficRankingPage />}
              {activeView === "logs" && <LogsCenterPage />}
              {activeView === "alerts" && <AlertsPage />}
              {activeView === "alert-policy" && (
                <AlertPolicyPage onOpenSettings={() => void navigate({ to: "/config" })} />
              )}
              {activeView === "onboarding" && <OnboardingPage />}
              {activeView === "trace" && <RequestTracePage />}
              {activeView === "vault" && <VaultPage />}
              {activeView === "profile" && <ProfilePage />}
              {activeView === "config" && <ConfigPage />}
              {activeView === "policy" && <PolicyPage />}
            </motion.div>
          </SidebarInset>
        </div>
      </SidebarProvider>
    </>
  );
}

function ConsoleSidebar(props: { activeView: View }) {
  return (
    <Sidebar variant="inset" collapsible="icon">
      <SidebarContent className="py-2">
        {navSections.map((section) => (
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
                      render={<Link to={item.to} />}
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
    source.onopen = () => setConnected(true);
    source.onerror = () => setConnected(false);
    const updateStatus = (event: MessageEvent<string>) => {
      try {
        queryClient.setQueryData<AutoInspectionStatus>(["auto-inspection"], JSON.parse(event.data));
        setConnected(true);
      } catch {
        // The polling fallback below recovers malformed or interrupted event streams.
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

export function SchedulerHeaderControls() {
  const queryClient = useQueryClient();
  const status = useQuery({
    queryKey: ["auto-inspection"],
    queryFn: api.autoInspection,
    refetchInterval: 15_000,
  });
  const [syncTaskId, setSyncTaskId] = useState<string | null>(null);
  const [runTaskId, setRunTaskId] = useState<string | null>(null);
  const sync = useMutation({
    mutationFn: api.syncManagement,
    onSuccess: (task) => {
      setSyncTaskId(task.id);
      toast.success("账号与分组同步已开始");
    },
    onError: (error) => toast.error(error instanceof Error ? error.message : "同步启动失败"),
  });
  const run = useMutation({
    mutationFn: () => api.runInspection(),
    onSuccess: (task) => {
      setRunTaskId(task.id);
      void status.refetch();
    },
    onError: (error) => toast.error(error instanceof Error ? error.message : "巡检启动失败"),
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
    onError: (error) => toast.error(error instanceof Error ? error.message : "调度状态更新失败"),
  });
  const syncTask = useQuery({
    queryKey: ["header-management-sync", syncTaskId],
    queryFn: () => api.task(syncTaskId!),
    enabled: Boolean(syncTaskId),
    refetchInterval: taskPollInterval,
  });
  const runTask = useQuery({
    queryKey: ["header-inspection", runTaskId],
    queryFn: () => api.task(runTaskId!),
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
    if (runTask.data && ["succeeded", "failed", "cancelled"].includes(runTask.data.status)) {
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
  return (
    <div className="flex items-center gap-1" data-testid="scheduler-header-controls">
      <Tooltip>
        <TooltipTrigger
          render={
            <span
              className="bg-muted text-muted-foreground hidden h-7 items-center gap-1.5 rounded-md px-2 text-xs lg:inline-flex"
              tabIndex={0}
            />
          }
        >
          <span
            className={cn(
              "size-1.5 rounded-full",
              status.isError
                ? "bg-destructive"
                : status.isLoading
                  ? "animate-pulse bg-muted-foreground"
                  : status.data?.running
                    ? "animate-pulse bg-amber-500"
                    : status.data?.enabled
                      ? "bg-emerald-500"
                      : "bg-muted-foreground/60",
            )}
          />
          {status.isError ? "状态读取失败" : status.isLoading ? "读取状态" : runtimeLabel}
        </TooltipTrigger>
        <TooltipContent>
          {status.isError
            ? "自动巡检状态接口暂时不可用"
            : status.data?.last_run_at
              ? `上次执行：${formatDate(status.data.last_run_at, true)}`
              : "尚未执行巡检"}
        </TooltipContent>
      </Tooltip>
      <Tooltip>
        <TooltipTrigger render={<span className="inline-flex" />}>
          <Button
            variant="ghost"
            size="sm"
            className="gap-1.5"
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
            size="sm"
            className="gap-1.5"
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
            size="sm"
            className="gap-1.5"
            disabled={status.isLoading || toggle.isPending}
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
    queryFn: api.autoInspection,
    refetchInterval: 15_000,
  });
  const trafficState = trafficCollectionState(status.data, status.isError);
  return (
    <div
      className="border-sidebar-border mt-auto min-w-0 space-y-2 overflow-hidden border-t px-4 py-3 group-data-[collapsible=icon]:hidden"
      data-testid="scheduler-sidebar-status"
    >
      <div className="flex h-4 min-w-0 items-center gap-2 overflow-hidden text-xs whitespace-nowrap">
        <span
          className={cn(
            "size-2 shrink-0 rounded-full",
            status.isError
              ? "bg-destructive"
              : status.data?.enabled
                ? "bg-emerald-500"
                : "bg-muted-foreground/50",
          )}
        />
        <span className="min-w-0 truncate">
          {status.isError
            ? "自动巡检状态读取失败"
            : status.data?.enabled
              ? "自动巡检中"
              : "自动巡检已暂停"}
        </span>
      </div>
      <div className="flex h-4 min-w-0 items-center gap-2 overflow-hidden text-xs whitespace-nowrap">
        <span className={cn("size-2 shrink-0 rounded-full", trafficStateDot(trafficState.tone))} />
        <span className="min-w-0 truncate">{trafficState.label}</span>
      </div>
    </div>
  );
}

function StartupState(props: { text: string; error?: boolean }) {
  return (
    <div className="bg-background text-foreground grid min-h-svh place-items-center p-6">
      <Card className={cn("w-full max-w-md p-6", props.error && "border-destructive")}>
        <div className="flex items-center gap-3">
          <span className="flex size-9 items-center justify-center rounded-lg bg-primary/15 text-primary">
            <Activity size={20} />
          </span>
          <div>
            <strong className="block text-sm">{props.text}</strong>
            <span className="text-muted-foreground mt-1 block text-xs">
              {props.error ? "请检查 API 地址和服务状态。" : "Sub2API Console"}
            </span>
          </div>
        </div>
      </Card>
    </div>
  );
}

const setupSchema = z
  .object({
    username: z.string().min(2, "账号至少 2 个字符"),
    password: z.string().min(10, "密码至少 10 个字符"),
    confirm_password: z.string().min(1, "请再次输入密码"),
    admin_base_url: z.union([z.literal(""), z.string().url("请输入完整的 http/https 地址")]),
    admin_key: z.string(),
  })
  .refine((value) => value.password === value.confirm_password, {
    path: ["confirm_password"],
    message: "两次输入的密码不一致",
  })
  .refine(
    (value) =>
      (value.admin_base_url === "" && value.admin_key === "") ||
      (value.admin_base_url !== "" && value.admin_key !== ""),
    {
      path: ["admin_base_url"],
      message: "Admin Base URL 和 Admin Key 必须同时填写",
    },
  );
type SetupForm = z.infer<typeof setupSchema>;

function SetupPage(props: { status?: SetupStatus; onComplete: () => void }) {
  const form = useForm<SetupForm>({
    resolver: zodResolver(setupSchema),
    defaultValues: {
      username: "",
      password: "",
      confirm_password: "",
      admin_base_url: "",
      admin_key: "",
    },
  });
  const submit = form.handleSubmit(async (values) => {
    try {
      await api.initialize({
        username: values.username,
        password: values.password,
        admin_base_url: props.status?.target_configured ? "" : values.admin_base_url,
        admin_key: props.status?.target_configured ? "" : values.admin_key,
      });
      props.onComplete();
    } catch (reason) {
      notifyOperationError(reason, "初始化失败");
    }
  });
  return (
    <div className="bg-background text-foreground grid min-h-svh place-items-center p-6">
      <Card className="w-full max-w-md">
        <CardHeader>
          <div className="flex items-start gap-3">
            <span className="flex size-9 items-center justify-center rounded-lg bg-primary/15 text-primary">
              <Activity size={20} />
            </span>
            <div>
              <div className="text-muted-foreground text-[11px] font-medium tracking-wider uppercase">
                FIRST RUN / SETUP
              </div>
              <CardTitle className="mt-1">初始化控制台</CardTitle>
              <p className="text-muted-foreground mt-2 text-sm">
                {props.status?.target_configured
                  ? "管理目标已存在，只需设置控制台登录账号。"
                  : "设置登录账号，并连接 Sub2API 管理端。"}
              </p>
            </div>
          </div>
        </CardHeader>
        <CardContent>
          <form className="grid gap-4" onSubmit={submit}>
            <FormField label="控制台账号" error={form.formState.errors.username?.message}>
              <Input
                autoComplete="username"
                aria-invalid={Boolean(form.formState.errors.username)}
                {...form.register("username")}
                placeholder="例如 operator"
              />
            </FormField>
            <FormField label="控制台密码" error={form.formState.errors.password?.message}>
              <Input
                type="password"
                autoComplete="new-password"
                aria-invalid={Boolean(form.formState.errors.password)}
                {...form.register("password")}
                placeholder="至少 10 个字符"
              />
            </FormField>
            <FormField
              label="确认控制台密码"
              error={form.formState.errors.confirm_password?.message}
            >
              <Input
                type="password"
                autoComplete="new-password"
                aria-invalid={Boolean(form.formState.errors.confirm_password)}
                {...form.register("confirm_password")}
              />
            </FormField>
            {!props.status?.target_configured && (
              <>
                <FormField
                  label="Admin Base URL"
                  error={form.formState.errors.admin_base_url?.message}
                >
                  <Input
                    type="url"
                    aria-invalid={Boolean(form.formState.errors.admin_base_url)}
                    {...form.register("admin_base_url")}
                    placeholder="https://sub2api.example.com"
                  />
                </FormField>
                <FormField label="Admin Key" error={form.formState.errors.admin_key?.message}>
                  <Input
                    type="password"
                    autoComplete="off"
                    aria-invalid={Boolean(form.formState.errors.admin_key)}
                    {...form.register("admin_key")}
                    placeholder="只提交到后端，不会回显"
                  />
                </FormField>
              </>
            )}
            <Button type="submit" disabled={form.formState.isSubmitting}>
              <ShieldCheck size={16} />
              {form.formState.isSubmitting ? "正在初始化…" : "完成初始化"}
            </Button>
          </form>
          <div className="border-border text-muted-foreground mt-5 flex items-start gap-2 border-t pt-4 text-xs leading-relaxed">
            <ShieldCheck size={15} className="text-primary shrink-0" />
            {props.status?.target_configured
              ? "管理目标已保存在 Console 私有配置库，Admin Key 不会重新要求输入。"
              : "Admin Key 仅保存在后端本地配置库，页面不会读取或显示它。"}
          </div>
        </CardContent>
      </Card>
    </div>
  );
}

const loginSchema = z.object({
  username: z.string().min(1, "请输入账号"),
  password: z.string().min(1, "请输入密码"),
});
type LoginForm = z.infer<typeof loginSchema>;

export function LoginPage(props: { onLogin: () => void; reason?: string | null }) {
  const form = useForm<LoginForm>({
    resolver: zodResolver(loginSchema),
    defaultValues: { username: "", password: "" },
  });
  const submit = form.handleSubmit(async (values) => {
    try {
      await api.login(values);
      props.onLogin();
    } catch (reason) {
      notifyOperationError(reason, "登录失败");
    }
  });
  return (
    <div className="bg-background text-foreground relative min-h-svh overflow-hidden">
      <header className="absolute inset-x-0 top-0 z-10 flex h-16 items-center px-5 sm:h-20 sm:px-8">
        <div className="flex items-center gap-3">
          <span className="bg-primary/12 text-primary flex size-9 items-center justify-center rounded-xl ring-1 ring-primary/15">
            <Activity size={19} />
          </span>
          <div className="leading-none">
            <strong className="block text-sm font-semibold tracking-tight">Sub2API</strong>
            <span className="text-muted-foreground mt-1 block text-[10px] font-medium tracking-[0.16em] uppercase">
              Console
            </span>
          </div>
        </div>
      </header>
      <main className="flex min-h-svh items-center justify-center px-5 py-20 sm:px-8">
        <section className="w-full max-w-[440px]">
          <div className="mb-8">
            <h1 className="text-2xl font-semibold tracking-tight sm:text-3xl">登录</h1>
          </div>
          {props.reason ? (
            <div
              className="border-warning/35 bg-warning/10 text-warning mb-5 flex items-start gap-2 rounded-lg border px-3 py-2.5 text-sm"
              role="alert"
            >
              <CircleAlert className="mt-0.5 shrink-0" size={16} aria-hidden="true" />
              <span>{props.reason}</span>
            </div>
          ) : null}
          <form className="grid gap-5" onSubmit={submit}>
            <FormField
              reserveErrorSpace
              label="账号"
              error={form.formState.errors.username?.message}
            >
              <Input
                autoFocus
                autoComplete="username"
                aria-invalid={Boolean(form.formState.errors.username)}
                {...form.register("username")}
                className="h-10 bg-card/35 px-3"
              />
            </FormField>
            <FormField
              reserveErrorSpace
              label="密码"
              error={form.formState.errors.password?.message}
            >
              <Input
                type="password"
                autoComplete="current-password"
                aria-invalid={Boolean(form.formState.errors.password)}
                {...form.register("password")}
                className="h-10 bg-card/35 px-3"
              />
            </FormField>
            <Button
              type="submit"
              className="mt-2 h-10 w-full"
              disabled={form.formState.isSubmitting}
            >
              <ShieldCheck size={16} />
              {form.formState.isSubmitting ? "登录中…" : "登录"}
            </Button>
          </form>
        </section>
      </main>
    </div>
  );
}

export function FormField(props: {
  label: string;
  error?: string;
  children: React.ReactNode;
  reserveErrorSpace?: boolean;
}) {
  return (
    <div className="grid gap-1.5 text-sm font-medium">
      <span>{props.label}</span>
      {props.children}
      {props.reserveErrorSpace ? (
        <span
          className="text-destructive min-h-4 text-xs leading-4 font-normal break-words"
          role={props.error ? "alert" : undefined}
        >
          {props.error ?? ""}
        </span>
      ) : (
        props.error && (
          <span className="text-destructive text-xs leading-4 font-normal break-words" role="alert">
            {props.error}
          </span>
        )
      )}
    </div>
  );
}
function searchable(values: Array<string | number | null | undefined>, query: string) {
  const normalizedQuery = query.trim().toLocaleLowerCase();
  return (
    !normalizedQuery ||
    values.some((value) =>
      String(value ?? "")
        .toLocaleLowerCase()
        .includes(normalizedQuery),
    )
  );
}
function explicitValue(value: string | null | undefined, missing = "未设置", empty = "空值") {
  if (value === null || value === undefined) return missing;
  return value === "" ? empty : value;
}
const strategyLabels: Record<string, string> = {
  balanced: "均衡",
  price: "价格优先",
  price_first: "价格优先",
  speed: "速度优先",
  latency_first: "速度优先",
  speed_first: "速度优先",
  cost_first: "价格优先",
  reliability: "稳定优先",
  reliability_first: "稳定优先",
  stable: "稳定优先",
  stability: "稳定优先",
  stability_first: "稳定优先",
  未参与: "未参与",
  配置错误: "配置错误",
};
export { schedulingStrategyOptions };
const fallbackLabels: Record<string, string> = {
  current_cost_wall: "回退当前成本墙",
  fail_closed: "严格关闭",
  fail_open: "允许继续",
};
const autoApplyLabels: Record<string, string> = {
  schedulable: "调度状态",
  priority: "优先级",
  load_factor: "负载因子",
  concurrency: "并发上限",
};
const statusLabels: Record<string, string> = {
  ok: "正常",
  partial: "部分完成",
  warning: "警告",
  记录: "已记录",
  queued: "排队中",
  running: "运行中",
  waiting_input: "等待输入",
  succeeded: "已完成",
  failed: "失败",
  cancelled: "已取消",
  error: "错误",
  healthy: "健康",
  degraded: "降级",
  fused: "熔断",
  cost_blocked: "成本墙拦截",
  survivor: "保底",
  paused: "已暂停",
  excluded: "已排除",
  unknown: "待观察",
  active: "生效",
  inactive: "未生效",
  enabled: "已启用",
  disabled: "已停用",
  available: "可用",
  unavailable: "不可用",
  authenticated: "已鉴权",
  unauthenticated: "未鉴权",
  synced: "已同步",
  unsynced: "未同步",
  insufficient: "余额不足",
  exhausted: "额度耗尽",
  not_found: "未找到",
  not_read: "未读取",
  pending: "待处理",
  firing: "告警中",
  recovered: "已恢复",
  suppressed: "规则已停用",
  closed: "已关闭",
  sent: "已发送",
  delivered: "已发送",
  failed_delivery: "通知发送失败",
  credential_invalid: "鉴权失效",
  auth_recovery_failed: "鉴权恢复失败",
  refresh_token_invalid: "刷新令牌失效",
  image_captcha_required: "需要人工完成图片验证码",
  image_captcha_ocr: "等待输入图片验证码",
  browser_challenge_required: "需要浏览器验证",
  probe_failed: "探测失败",
  gateway_error: "网关错误",
  rate_limited_or_exhausted: "限流或额度不足",
  unknown_upstream_error: "上游错误",
  apply: "自动执行",
  shadow: "仅计算",
  calculation: "计算",
  write: "写入",
  readback: "读回",
  remote_write: "远程写入",
  remote_readback: "远程读回",
  evaluate: "评估",
  inspect: "检查",
  true: "开启",
  false: "关闭",
};
const eventLabels: Record<string, string> = {
  "latency-watcher": "延迟监控",
  "inspection-run": "巡检任务",
  "routing-decision-change": "路由决策变更",
  "upstream-runtime-snapshot": "上游运行快照",
  "upstream-sync-run": "上游同步任务",
  "inspection-coordinator-run": "巡检协调任务",
  "auth-recovery-run": "鉴权恢复任务",
  "auth-recovery-runtime-snapshot": "鉴权恢复快照",
  "policy.updated": "策略已更新",
  "alerts.evaluate": "告警检测",
  "active-probe": "主动探测",
  "runtime.inspection": "巡检运行",
  "inspection.snapshot-review": "巡检快照复核",
  "inspection.automation.updated": "自动巡检设置",
  "auth-recovery": "鉴权恢复",
  "rate-sync": "倍率同步",
  "upstream.rate_sync": "上游倍率同步",
  "account-scheduling": "账号调度",
  "account-sync": "账号字段同步",
  "account.groups.sync": "账号分组同步",
  "account.rates.synced": "账号倍率同步",
  "account.onboarding": "账号添加",
  "routing.writeback": "自动执行",
  "automatic-inspection": "自动巡检",
};
const upstreamTypeLabels: Record<string, string> = {
  sub2api: "Sub2API",
  newapi: "New API",
  oneapi: "OneAPI",
  custom: "自定义上游",
  apikey: "API Key",
};
const operationLabels: Record<string, string> = {
  "management-balances-sync": "账号余额同步",
  "management-groups-sync": "账号分组同步",
  "management-snapshot-sync": "管理数据同步",
  "account-rate-sync": "账号倍率同步",
  "account.scheduling": "账号调度",
  "account.sync": "账号同步",
  "account.groups.sync": "账号分组同步",
  "account.onboarding": "账号添加",
  "routing.writeback": "自动执行",
  "upstream.delete": "删除上游",
  "upstream.rate_sync": "上游倍率同步",
  "policy.update": "策略更新",
  "policy.updated": "策略更新",
};
const phaseLabels: Record<string, string> = {
  calculation: "计算",
  write: "写入",
  "local-commit": "本地提交",
  read: "读取",
  readback: "读回",
  "remote-write": "远程写入",
  "remote-readback": "远程读回",
  evaluate: "评估",
  inspect: "检查",
  "snapshot-review": "快照复核",
};
const keyLabels: Record<string, string> = {
  account_id: "账号 ID",
  account_name: "账号名称",
  group_name: "分组",
  upstream_host: "上游 Host",
  upstream_type: "上游类型",
  group_id: "分组 ID",
  request_id: "请求 ID",
  run_key: "运行标识",
  task_name: "任务名称",
  operation: "操作",
  operation_type: "操作类型",
  actor: "执行人",
  phase: "阶段",
  status: "状态",
  result: "结果",
  reason: "原因",
  source: "来源",
  strategy: "策略",
  priority: "优先级",
  load_factor: "负载因子",
  concurrency: "并发上限",
  schedulable: "可调度",
  health_score: "健康分",
  sample_count: "样本数",
  rate: "倍率",
  weight: "权重",
  duration_seconds: "耗时（秒）",
  started_at: "开始时间",
  ended_at: "结束时间",
  enabled: "是否启用",
  interval_seconds: "巡检周期（秒）",
  active_probe_enabled: "允许主动探测",
  writeback_enabled: "允许自动执行",
  alert_evaluation_enabled: "巡检后执行告警检测",
  created_at: "创建时间",
  updated_at: "更新时间",
  observed_at: "记录时间",
  mode: "运行模式",
  stage: "执行阶段",
  progress: "进度",
  remote_account_id: "远程账号 ID",
  remote_group_id: "远程分组 ID",
  upstream_key_id: "上游 Key ID",
  upstream_group_id: "上游分组 ID",
  remote_confirmed: "远程已确认",
  readback_confirmed: "读回已确认",
  failure_reason: "失败原因",
  error_reason: "错误原因",
  interaction_kind: "恢复方式",
  refresh_attempt: "刷新令牌结果",
  refresh_kind: "刷新方式",
  code: "结果代码",
  outcome: "结果",
  host: "上游 Host",
  success: "恢复成功",
  attempted: "已尝试",
  transient: "临时错误",
  committed: "已提交",
  projection: "恢复投影",
  hosts: "关联 Host 数",
  credential_entry: "密码箱条目",
  evidence: "请求记录与探针",
  traffic_persisted: "写入真实请求样本",
  probes_persisted: "写入主动探针样本",
  monitored_accounts: "读取请求记录的账号数",
  monitoring_available: "运维请求记录可用",
  probe_duration_second: "主动探针耗时（秒）",
};
const resultLabels: Record<string, string> = {
  vault_login_failed: "密码箱登录失败",
  vault_login_succeeded: "密码箱登录成功",
  refresh_token_missing: "没有刷新令牌",
  refresh_token_invalid: "刷新令牌无效",
  refresh_failed: "刷新令牌失败",
  auth_failed: "上游鉴权失败",
  auth_verified: "鉴权复核通过",
  credential_invalid: "鉴权信息失效",
  upstream_auth_failed: "上游鉴权失败",
  browser_challenge_required: "需要浏览器验证",
  image_captcha_required: "需要人工完成图片验证码",
  image_captcha_ocr: "图片验证码识别",
};
function formatKey(value: string) {
  return value
    .replace(/_id$/i, " ID")
    .replace(/_/g, " ")
    .replace(/\b\w/g, (letter) => letter.toUpperCase());
}
function displayLabel(value: string | null | undefined, labels = statusLabels) {
  if (value === null || value === undefined) return "未配置";
  if (value === "") return "空值";
  const raw = String(value);
  const exact = labels[raw] ?? labels[raw.toLowerCase()];
  if (exact !== undefined) return exact;
  const match = raw.match(/^([a-z_]+)(\s+.*)$/i);
  if (match && labels[match[1].toLowerCase()] !== undefined)
    return `${labels[match[1].toLowerCase()]}${match[2]}`;
  return raw.includes("_") ? formatKey(raw) : raw;
}
function displayResultKey(value: string) {
  const exact = keyLabels[value];
  if (exact) return exact;
  const normalized = value.trim().toLowerCase();
  const match = Object.entries(keyLabels).find(([key]) => key.toLowerCase() === normalized);
  return match?.[1] ?? (normalized === "outcome" ? "结果" : formatKey(value));
}
function displayResultValue(value: string) {
  const exact = resultLabels[value.trim().toLowerCase()];
  return exact ?? displayLabel(value);
}
function displayTaskMessage(value: string | null | undefined) {
  const message = String(value ?? "任务结果");
  return Object.entries(resultLabels).reduce(
    (current, [code, label]) => current.replaceAll(code, label),
    message,
  );
}
function displayText(value: string | null | undefined) {
  return String(value ?? "")
    .split(/(\s|·|：|:|\/)/)
    .map(
      (part) =>
        eventLabels[part] ??
        operationLabels[part] ??
        phaseLabels[part] ??
        statusLabels[part.toLowerCase()] ??
        part,
    )
    .join("");
}
function displayStrategy(value: string | null | undefined) {
  if (value === null || value === undefined) return "配置错误";
  return displayLabel(value, strategyLabels);
}
function displayFallback(value: string | null | undefined) {
  return displayLabel(value, fallbackLabels);
}
function displayUpstreamType(value: string | null | undefined) {
  return displayLabel(value, upstreamTypeLabels);
}
function splitConfigList(value: string) {
  return value
    .split(/[,，\n]/)
    .map((item) => item.trim())
    .filter(Boolean);
}
type PolicyDraft = Omit<
  PolicyUpdate,
  | "auto_apply"
  | "excluded_group_ids"
  | "cooldown_seconds"
  | "probe_interval_seconds"
  | "probe_model"
  | "traffic_lookback_minutes"
  | "max_samples_per_account"
> & {
  auto_apply: Record<string, unknown> | null;
  excluded_group_ids: string[] | null;
  cooldown_seconds: number | null;
  probe_interval_seconds: number | null;
  traffic_lookback_minutes: number | null;
  max_samples_per_account: number | null;
  probe_model: string | null;
  advanced_policy: Record<string, unknown>;
};
export function policyDraft(value: import("./api").PolicySnapshot): PolicyDraft {
  const autoApply =
    value.auto_apply === null
      ? null
      : Object.fromEntries(
          Object.keys(autoApplyLabels).map((key) => [key, value.auto_apply?.[key] === true]),
        );
  return {
    mode: value.mode,
    global_strategy: value.global_strategy ?? "",
    missing_rate_fallback: value.missing_rate_fallback ?? "",
    change_threshold: value.change_threshold ?? "",
    cooldown_seconds: value.cooldown_seconds,
    auto_apply: autoApply,
    excluded_group_ids: value.excluded_group_ids === null ? null : (value.excluded_group_ids ?? []),
    traffic_enabled: value.traffic_enabled === true,
    probe_interval_seconds: value.probe_interval_seconds,
    probe_model: value.probe_model,
    traffic_lookback_minutes: value.traffic_lookback_minutes,
    max_samples_per_account: value.max_samples_per_account,
    advanced_policy: policyAdvancedDraft(value.advanced_policy),
  };
}

function policyAdvancedDraft(value: Record<string, unknown>): Record<string, unknown> {
  return { ...value };
}
function policyPayload(value: PolicyDraft): PolicyUpdate | null {
  const numeric = [
    value.cooldown_seconds,
    value.probe_interval_seconds,
    value.traffic_lookback_minutes,
    value.max_samples_per_account,
  ];
  const upstreamDataInterval = policyAdvancedValue(
    value,
    "upstream_multiplier",
    "interval_seconds",
  );
  const trafficEvidenceInterval = policyAdvancedValue(value, "traffic", "refresh_seconds");
  const configuredManualPriorityMax = policyAdvancedValue(value, "manual_priority", "reserved_max");
  const manualPriorityMax =
    configuredManualPriorityMax === undefined ? 10 : configuredManualPriorityMax;
  if (value.auto_apply === null || value.excluded_group_ids === null) return null;
  if (policyRelationshipError(value)) return null;
  if (Object.values(value.auto_apply).some((item) => typeof item !== "boolean")) return null;
  if (
    !(["监控模式", "完全模式"] as string[]).includes(value.mode) ||
    !value.global_strategy.trim() ||
    !value.missing_rate_fallback.trim() ||
    !value.change_threshold.trim() ||
    numeric.some((item) => item === null || !Number.isInteger(item) || item < 0)
  )
    return null;
  if (
    value.probe_interval_seconds === null ||
    value.traffic_lookback_minutes === null ||
    value.max_samples_per_account === null ||
    value.probe_interval_seconds < 30 ||
    value.traffic_lookback_minutes < 1 ||
    value.max_samples_per_account < 1
  )
    return null;
  if (
    typeof upstreamDataInterval !== "number" ||
    !Number.isInteger(upstreamDataInterval) ||
    upstreamDataInterval < 30 ||
    upstreamDataInterval > 86400 ||
    typeof trafficEvidenceInterval !== "number" ||
    !Number.isInteger(trafficEvidenceInterval) ||
    trafficEvidenceInterval < 1 ||
    trafficEvidenceInterval > 86400 ||
    typeof manualPriorityMax !== "number" ||
    !Number.isInteger(manualPriorityMax) ||
    manualPriorityMax < 1 ||
    manualPriorityMax > 1000
  )
    return null;
  return { ...value, probe_model: value.probe_model ?? "" } as PolicyUpdate;
}

export function policyRelationshipError(value: PolicyDraft): string | null {
  const number = (section: string, path: string): number | null => {
    const raw = policyAdvancedValue(value, section, path);
    return typeof raw === "number" && Number.isFinite(raw) ? raw : null;
  };
  const relations = [
    ["scoring", "short_window", "scoring", "long_window", "长期窗口不能小于短期窗口"],
    ["breaker", "http_failures", "breaker", "http_window", "网关失败次数不能大于判定窗口"],
    [
      "breaker",
      "latency_occurrences",
      "breaker",
      "latency_window",
      "慢响应次数不能大于延迟判定窗口",
    ],
    ["weights", "min_load_factor", "weights", "max_load_factor", "负载因子下限不能大于上限"],
    ["scaling", "min_per_account", "scaling", "max_per_account", "单账号并发下限不能大于上限"],
    ["cleanup", "occurrences", "cleanup", "window", "失效次数不能大于判定窗口"],
  ] as const;
  for (const [leftSection, leftPath, rightSection, rightPath, message] of relations) {
    const left = number(leftSection, leftPath);
    const right = number(rightSection, rightPath);
    if (left !== null && right !== null && left > right) return message;
  }
  const quotaScore = number("scoring", "event_scores.quota_exhausted");
  if (quotaScore !== null && quotaScore <= 0) return "限流 / 额度耗尽分必须大于 0";
  return null;
}
function policyNumberInput(value: string): number | null {
  if (value.trim() === "") return null;
  const parsed = Number(value);
  return Number.isFinite(parsed) && Number.isInteger(parsed) ? parsed : null;
}
export function FilterMenu(props: {
  label: string;
  options: string[];
  value: string | null;
  onValueChange: (value: string | null) => void;
  optionLabel?: (value: string) => string;
  optionCount?: (value: string) => number | undefined;
}) {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const [activeIndex, setActiveIndex] = useState(0);
  const options = props.options.filter((option) =>
    searchable([option, props.optionLabel ? props.optionLabel(option) : option], query),
  );
  const selectedLabel = props.value
    ? props.optionLabel
      ? props.optionLabel(props.value)
      : props.value
    : null;
  function handleOpenChange(nextOpen: boolean) {
    setOpen(nextOpen);
    if (!nextOpen) {
      setQuery("");
      setActiveIndex(0);
    }
  }
  function toggleOption(option: string) {
    props.onValueChange(props.value === option ? null : option);
  }
  function handleSearchKeyDown(event: React.KeyboardEvent<HTMLInputElement>) {
    if (!options.length) return;
    if (event.key === "ArrowDown") {
      event.preventDefault();
      setActiveIndex((current) => (current + 1) % options.length);
    } else if (event.key === "ArrowUp") {
      event.preventDefault();
      setActiveIndex((current) => (current - 1 + options.length) % options.length);
    } else if (event.key === "Home") {
      event.preventDefault();
      setActiveIndex(0);
    } else if (event.key === "End") {
      event.preventDefault();
      setActiveIndex(options.length - 1);
    } else if (event.key === "Enter") {
      event.preventDefault();
      toggleOption(options[Math.min(activeIndex, options.length - 1)]);
    }
  }
  return (
    <Popover.Root open={open} onOpenChange={handleOpenChange}>
      <Popover.Trigger
        render={
          <Button
            type="button"
            variant="outline"
            size="sm"
            data-press-animation="none"
            className="h-8 max-w-64 border-dashed bg-transparent"
            aria-label={`${props.label}筛选`}
          />
        }
      >
        <CirclePlus size={16} />
        <span className="min-w-0 truncate">{props.label}</span>
        {selectedLabel ? (
          <>
            <span className="bg-border mx-1 h-4 w-px shrink-0" aria-hidden="true" />
            <Badge variant="secondary" className="max-w-36 truncate rounded-sm px-1 font-normal">
              {selectedLabel}
            </Badge>
          </>
        ) : null}
      </Popover.Trigger>
      <Popover.Portal>
        <Popover.Positioner
          side="bottom"
          sideOffset={4}
          align="start"
          collisionPadding={8}
          className="z-50"
        >
          <Popover.Popup
            data-slot="faceted-filter-content"
            className="bg-popover text-popover-foreground w-[min(22rem,calc(100vw-1rem))] min-w-52 rounded-lg border p-1 shadow-lg outline-none"
          >
            <div className="relative p-1 pb-0">
              <Search
                size={16}
                className="text-muted-foreground pointer-events-none absolute top-1/2 left-3 -translate-y-1/2 opacity-60"
              />
              <Input
                autoFocus
                value={query}
                onChange={(event) => {
                  setQuery(event.target.value);
                  setActiveIndex(0);
                }}
                onKeyDown={handleSearchKeyDown}
                placeholder={props.label}
                aria-label={`搜索${props.label}`}
                className="border-input/30 bg-input/30 h-8 rounded-lg pl-8 shadow-none"
              />
            </div>
            <div
              className="max-h-72 overflow-x-hidden overflow-y-auto p-1"
              role="listbox"
              aria-label={props.label}
            >
              {!options.length && (
                <div className="text-muted-foreground py-6 text-center text-sm">没有匹配项</div>
              )}
              {options.map((option, index) => (
                <button
                  type="button"
                  data-press-animation="none"
                  role="option"
                  aria-selected={props.value === option}
                  data-active={activeIndex === index || undefined}
                  className="hover:bg-muted data-[active]:bg-muted flex min-h-8 w-full cursor-default items-center gap-2 rounded-sm px-2 py-1.5 text-left text-sm outline-none"
                  key={option}
                  onMouseEnter={() => setActiveIndex(index)}
                  onClick={() => {
                    toggleOption(option);
                  }}
                >
                  <span
                    className={cn(
                      "border-primary flex size-4 shrink-0 items-center justify-center rounded-sm border",
                      props.value === option && "bg-primary text-primary-foreground border-primary",
                      props.value !== option && "opacity-50 [&_svg]:invisible",
                    )}
                    aria-hidden="true"
                  >
                    <Check size={14} />
                  </span>
                  <span className="min-w-0 flex-1 truncate">
                    {props.optionLabel ? props.optionLabel(option) : option}
                  </span>
                  {props.optionCount?.(option) !== undefined ? (
                    <span className="text-muted-foreground ml-auto font-mono text-xs tabular-nums">
                      {props.optionCount(option)}
                    </span>
                  ) : null}
                </button>
              ))}
            </div>
            {props.value ? (
              <div className="border-t p-1">
                <button
                  type="button"
                  data-press-animation="none"
                  className="hover:bg-muted flex h-8 w-full items-center justify-center rounded-sm px-2 text-sm"
                  onClick={() => props.onValueChange(null)}
                >
                  清除筛选
                </button>
              </div>
            ) : null}
          </Popover.Popup>
        </Popover.Positioner>
      </Popover.Portal>
    </Popover.Root>
  );
}
function UpstreamsPage() {
  const navigate = useNavigate();
  const upstreams = useQuery({
    queryKey: ["upstreams"],
    queryFn: api.upstreams,
    refetchInterval: 30_000,
  });
  const config = useQuery({
    queryKey: ["config"],
    queryFn: api.config,
    staleTime: 15_000,
  });
  const queryClient = useQueryClient();
  const [syncDialogOpen, setSyncDialogOpen] = useState(false);
  const [syncTaskId, setSyncTaskId] = useState<string | null>(null);
  const syncUpstreams = useMutation({
    mutationFn: api.runUpstreamSync,
    onSuccess: (task) => setSyncTaskId(task.id),
    onError: (error) => notifyOperationError(error, "上游同步启动失败"),
  });
  const syncTask = useQuery({
    queryKey: ["upstream-sync", syncTaskId],
    queryFn: () => api.task(syncTaskId!),
    enabled: Boolean(syncTaskId),
    refetchInterval: taskPollInterval,
  });
  const [managementDialog, setManagementDialog] = useState<"balance" | "groups" | "names" | null>(
    null,
  );
  const [managementTaskId, setManagementTaskId] = useState<string | null>(null);
  const managementTask = useQuery({
    queryKey: ["management-sync", managementTaskId],
    queryFn: () => api.task(managementTaskId!),
    enabled: Boolean(managementTaskId),
    refetchInterval: taskPollInterval,
  });
  const balanceSync = useMutation({
    mutationFn: api.syncUpstreamBalances,
    onSuccess: (task) => setManagementTaskId(task.id),
    onError: (error) => notifyOperationError(error, "余额同步启动失败"),
  });
  const groupSync = useMutation({
    mutationFn: api.syncUpstreamGroups,
    onSuccess: (task) => setManagementTaskId(task.id),
    onError: (error) => notifyOperationError(error, "分组同步启动失败"),
  });
  const nameRepair = useMutation({
    mutationFn: api.repairUpstreamNames,
    onSuccess: (task) => setManagementTaskId(task.id),
    onError: (error) => notifyOperationError(error, "上游名称修复启动失败"),
  });
  const [actionTaskId, setActionTaskId] = useState<string | null>(null);
  const [actionDialog, setActionDialog] = useState<{
    host: string;
    kind: "auth" | "balance";
  } | null>(null);
  const actionTask = useQuery({
    queryKey: ["upstream-action", actionTaskId],
    queryFn: () => api.task(actionTaskId!),
    enabled: Boolean(actionTaskId),
    refetchInterval: taskPollInterval,
  });
  const [deleteHost, setDeleteHost] = useState<string | null>(null);
  const [deleteTaskId, setDeleteTaskId] = useState<string | null>(null);
  const deletePreview = useQuery({
    queryKey: ["upstream-delete-preview", deleteHost],
    queryFn: () => api.upstreamDeletePreview(deleteHost!),
    enabled: Boolean(deleteHost) && !deleteTaskId,
    retry: false,
  });
  const deleteUpstream = useMutation({
    mutationFn: (preview: { host: string; accountIds: string[] }) =>
      api.deleteUpstream(preview.host, preview.accountIds),
    onSuccess: (task) => setDeleteTaskId(task.id),
    onError: (error) => notifyOperationError(error, "删除任务启动失败"),
  });
  const deleteTask = useQuery({
    queryKey: ["upstream-delete-task", deleteTaskId],
    queryFn: () => api.task(deleteTaskId!),
    enabled: Boolean(deleteTaskId),
    refetchInterval: taskPollInterval,
  });
  const recover = useMutation({
    mutationFn: (selection: { host: string; entry: string }) =>
      api.runAuthRecovery(selection.host, selection.entry),
    onSuccess: (task) => setActionTaskId(task.id),
    onError: (error) => notifyOperationError(error, "恢复鉴权启动失败"),
  });
  const refreshBalance = useMutation({
    mutationFn: (host: string) => api.runBalanceSync(host),
    onSuccess: (task) => setActionTaskId(task.id),
    onError: (error) => notifyOperationError(error, "余额同步启动失败"),
  });
  const [captchaCode, setCaptchaCode] = useState("");
  const [recoveryEntry, setRecoveryEntry] = useState("");
  const [captchaClock, setCaptchaClock] = useState(() => Date.now());
  const [manualCaptchaChallenge, setManualCaptchaChallenge] = useState<CaptchaChallenge | null>(
    null,
  );
  const taskCaptchaChallenge = captchaChallengeFromTask(actionTask.data);
  const captchaChallenge = manualCaptchaChallenge ?? taskCaptchaChallenge;
  const submitCaptcha = useMutation({
    mutationFn: (challenge: CaptchaChallenge) =>
      api.submitAuthCaptcha(challenge.challenge_id, captchaCode),
    onSuccess: (result) => {
      toast.success(`${result.host} 已通过图片验证码恢复并完成鉴权复核`);
      setCaptchaCode("");
      setManualCaptchaChallenge(null);
      setActionDialog(null);
      setActionTaskId(null);
      void queryClient.invalidateQueries({ queryKey: ["upstreams"] });
    },
    onError: (error) => notifyOperationError(error, "验证码恢复失败"),
  });
  const cancelCaptcha = useMutation({
    mutationFn: (challenge: CaptchaChallenge) => api.cancelAuthCaptcha(challenge.challenge_id),
    onSuccess: () => {
      setCaptchaCode("");
      setManualCaptchaChallenge(null);
      setActionTaskId(null);
    },
    onError: (error) => notifyOperationError(error, "验证码取消失败"),
  });
  const [selectedHost, setSelectedHost] = useState<string | null>(null);
  const [groupPage, setGroupPage] = useState(1);
  const [groupPageSize, setGroupPageSize] = useState(20);
  const [editHost, setEditHost] = useState<string | null>(null);
  const emptyUpstreamFilters = {
    keyword: "",
    upstreamType: "all",
    authStatus: "all",
    minimumBalance: "",
    maximumBalance: "",
  };
  const [filterDraft, setFilterDraft] = useState(emptyUpstreamFilters);
  const [filters, setFilters] = useState(emptyUpstreamFilters);
  const groups = useQuery({
    queryKey: ["upstream-groups", selectedHost],
    queryFn: () => api.upstreamGroups(selectedHost!),
    enabled: Boolean(selectedHost),
    retry: false,
  });
  const groupRows = groups.data ?? [];
  const groupTotalPages = Math.max(1, Math.ceil(groupRows.length / groupPageSize));
  const groupPageRows = groupRows.slice((groupPage - 1) * groupPageSize, groupPage * groupPageSize);
  useEffect(() => {
    setGroupPage(1);
  }, [selectedHost]);
  useEffect(() => {
    setGroupPage((current) => Math.min(current, groupTotalPages));
  }, [groupTotalPages]);
  const data = upstreams.data;
  const upstreamTypeOptions = Array.from(
    new Set((data?.hosts ?? []).map((host) => host.upstream_type).filter(Boolean)),
  ).sort((a, b) => displayUpstreamType(a).localeCompare(displayUpstreamType(b)));
  const authStatusOptions = Array.from(
    new Set((data?.hosts ?? []).map((host) => host.auth_status).filter(Boolean)),
  ).sort((a, b) => a.localeCompare(b));
  const minimumBalance = filters.minimumBalance ? Number(filters.minimumBalance) : null;
  const maximumBalance = filters.maximumBalance ? Number(filters.maximumBalance) : null;
  const hasActiveFilters = Object.values(filters).some((value) => value !== "" && value !== "all");
  const allHosts = upstreams.error
    ? []
    : (data?.hosts.filter((host) => {
        const balance = host.balance === null ? null : Number(host.balance);
        const balanceMatches =
          minimumBalance === null && maximumBalance === null
            ? true
            : balance !== null &&
              Number.isFinite(balance) &&
              (minimumBalance === null || balance >= minimumBalance) &&
              (maximumBalance === null || balance <= maximumBalance);
        return (
          searchable([host.host, host.name], filters.keyword) &&
          (filters.upstreamType === "all" || host.upstream_type === filters.upstreamType) &&
          (filters.authStatus === "all" || host.auth_status === filters.authStatus) &&
          balanceMatches
        );
      }) ?? []);
  const [pageSize, setPageSize] = useState(20);
  const [page, setPage] = useState(1);
  const totalPages = Math.max(1, Math.ceil(allHosts.length / pageSize));
  const hosts = allHosts.slice((page - 1) * pageSize, page * pageSize);
  useEffect(() => {
    setPage((current) => Math.min(current, totalPages));
  }, [totalPages]);
  function applyFilters(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const minimum = filterDraft.minimumBalance ? Number(filterDraft.minimumBalance) : null;
    const maximum = filterDraft.maximumBalance ? Number(filterDraft.maximumBalance) : null;
    if (minimum !== null && maximum !== null && minimum > maximum) {
      toast.error("最低余额不能大于最高余额");
      return;
    }
    setFilters({ ...filterDraft });
    setPage(1);
  }
  function resetFilters() {
    setFilterDraft({ ...emptyUpstreamFilters });
    setFilters({ ...emptyUpstreamFilters });
    setPage(1);
  }
  useEffect(() => {
    const keys = terminalRefreshKeys("upstream-action", actionTask.data);
    if (keys.length)
      void Promise.all(keys.map((queryKey) => queryClient.invalidateQueries({ queryKey })));
  }, [actionTask.data?.status, queryClient]);
  useEffect(() => {
    const keys = terminalRefreshKeys("upstream-action", syncTask.data);
    if (keys.length)
      void Promise.all(keys.map((queryKey) => queryClient.invalidateQueries({ queryKey })));
  }, [syncTask.data?.status, queryClient]);
  useEffect(() => {
    const keys = terminalRefreshKeys("management-sync", managementTask.data);
    if (keys.length)
      void Promise.all(keys.map((queryKey) => queryClient.invalidateQueries({ queryKey })));
  }, [managementTask.data?.status, queryClient]);
  useEffect(() => {
    if (!deleteTask.data || ["queued", "running", "waiting_input"].includes(deleteTask.data.status))
      return;
    void Promise.all([
      queryClient.invalidateQueries({ queryKey: ["upstreams"] }),
      queryClient.invalidateQueries({ queryKey: ["accounts"] }),
      queryClient.invalidateQueries({ queryKey: ["groups"] }),
      queryClient.invalidateQueries({ queryKey: ["auth-recovery-config"] }),
    ]);
  }, [deleteTask.data?.status, queryClient]);
  useEffect(() => {
    if (groups.isError || groups.isSuccess)
      void queryClient.invalidateQueries({ queryKey: ["upstreams"] });
  }, [groups.dataUpdatedAt, groups.isError, groups.isSuccess, queryClient]);
  useEffect(() => {
    if (captchaChallenge === null) return;
    setCaptchaCode("");
    setCaptchaClock(Date.now());
    const timer = window.setInterval(() => setCaptchaClock(Date.now()), 1_000);
    return () => window.clearInterval(timer);
  }, [captchaChallenge?.challenge_id]);
  const actionPending = taskIsPending(actionTaskId, actionTask);
  const managementPending =
    balanceSync.isPending ||
    groupSync.isPending ||
    nameRepair.isPending ||
    taskIsPending(managementTaskId, managementTask);
  const deletePending = deleteUpstream.isPending || taskIsPending(deleteTaskId, deleteTask);
  const fullMode = effectiveProjectMode(config.data?.mode) === "完全模式";
  const actionHost = data?.hosts.find((host) => host.host === actionDialog?.host);
  const captchaExpiresAt = captchaChallenge === null ? null : new Date(captchaChallenge.expires_at);
  const captchaExpired =
    captchaExpiresAt !== null &&
    (!Number.isFinite(captchaExpiresAt.getTime()) || captchaExpiresAt.getTime() <= captchaClock);
  function prepareCaptchaAgain() {
    if (captchaChallenge === null) return;
    cancelCaptcha.mutate(captchaChallenge, {
      onSuccess: () => {
        if (manualCaptchaChallenge !== null) return;
        if (!recoveryEntry) return;
        recover.mutate({
          host: captchaChallenge.host,
          entry: recoveryEntry,
        });
      },
    });
  }
  return (
    <PageLayout fixedContent>
      <PageHeading
        eyebrow="UPSTREAM / MANAGEMENT"
        title="上游管理"
        description="管理上游 Host、鉴权方式、倍率、余额和关联账号。"
        action={
          <PageActions>
            <Button
              onClick={() =>
                navigate({
                  to: "/onboarding",
                  search: {
                    host: undefined,
                    upstream_type: undefined,
                    group_id: undefined,
                  },
                })
              }
            >
              <UserPlus size={16} />
              添加账号
            </Button>
            <Button
              variant="outline"
              disabled={managementPending}
              onClick={() => {
                setManagementTaskId(null);
                balanceSync.reset();
                groupSync.reset();
                setManagementDialog("balance");
                balanceSync.mutate();
              }}
            >
              <WalletCards size={16} />
              同步余额
            </Button>
            <Button
              variant="outline"
              disabled={managementPending}
              onClick={() => {
                setManagementTaskId(null);
                nameRepair.reset();
                setManagementDialog("names");
                nameRepair.mutate();
              }}
            >
              <SpellCheck2 size={16} />
              名称修复
            </Button>
            <Button
              variant="outline"
              disabled={managementPending}
              onClick={() => {
                setManagementTaskId(null);
                balanceSync.reset();
                groupSync.reset();
                setManagementDialog("groups");
                groupSync.mutate();
              }}
            >
              <FolderOpen size={16} />
              同步分组
            </Button>
            <Button
              disabled={syncUpstreams.isPending || taskIsPending(syncTaskId, syncTask)}
              onClick={() => {
                setSyncTaskId(null);
                syncUpstreams.reset();
                setSyncDialogOpen(true);
                syncUpstreams.mutate();
              }}
            >
              <Server size={16} />
              同步上游
            </Button>
          </PageActions>
        }
      />
      {upstreams.error && <QueryError error={upstreams.error} fallback="上游数据读取失败" />}
      {config.error && <QueryError error={config.error} fallback="运行模式读取失败" />}
      <div className="flex h-full min-h-0 flex-col gap-2.5 sm:gap-3">
        <form className="flex w-full shrink-0 flex-wrap items-center gap-2" onSubmit={applyFilters}>
          <SearchField
            value={filterDraft.keyword}
            onChange={(keyword) => setFilterDraft((current) => ({ ...current, keyword }))}
            placeholder="搜索 Host 或名称"
          />
          <Select
            value={filterDraft.upstreamType}
            itemToStringLabel={(value) =>
              value === "all" ? "全部类型" : displayUpstreamType(value)
            }
            onValueChange={(upstreamType) =>
              setFilterDraft((current) => ({
                ...current,
                upstreamType: upstreamType ?? "all",
              }))
            }
          >
            <SelectTrigger className="w-36">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">全部类型</SelectItem>
              {upstreamTypeOptions.map((option) => (
                <SelectItem value={option} key={option}>
                  {displayUpstreamType(option)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <Select
            value={filterDraft.authStatus}
            itemToStringLabel={(value) => (value === "all" ? "全部状态" : value)}
            onValueChange={(authStatus) =>
              setFilterDraft((current) => ({
                ...current,
                authStatus: authStatus ?? "all",
              }))
            }
          >
            <SelectTrigger className="w-36">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">全部状态</SelectItem>
              {authStatusOptions.map((option) => (
                <SelectItem value={option} key={option}>
                  {option}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <div className="flex items-center gap-1.5">
            <span className="text-muted-foreground shrink-0 text-sm">余额</span>
            <Input
              type="number"
              inputMode="decimal"
              min="0"
              step="any"
              value={filterDraft.minimumBalance}
              onChange={(event) =>
                setFilterDraft((current) => ({
                  ...current,
                  minimumBalance: event.target.value,
                }))
              }
              placeholder="最低"
              aria-label="最低余额"
              className="w-28"
            />
            <span className="text-muted-foreground text-sm">至</span>
            <Input
              type="number"
              inputMode="decimal"
              min="0"
              step="any"
              value={filterDraft.maximumBalance}
              onChange={(event) =>
                setFilterDraft((current) => ({
                  ...current,
                  maximumBalance: event.target.value,
                }))
              }
              placeholder="最高"
              aria-label="最高余额"
              className="w-28"
            />
          </div>
          <div className="ml-auto flex shrink-0 items-center gap-2">
            <Button type="submit">
              <Search size={16} />
              搜索
            </Button>
            <Button type="button" variant="outline" onClick={resetFilters}>
              <RefreshCw size={16} />
              重置
            </Button>
          </div>
        </form>
        <Card className="min-h-0 flex-1 gap-0 py-0">
          <Table containerClassName="min-h-0 flex-1 overflow-auto" className="min-w-[1040px]">
            <TableHeader className="sticky top-0 z-10">
              <TableRow>
                <TableHead className="w-[28%]">上游</TableHead>
                <TableHead className="w-[11%]">类型</TableHead>
                <TableHead className="w-[9%]">分组</TableHead>
                <TableHead className="w-[10%]">已绑定</TableHead>
                <TableHead className="w-[14%]">状态</TableHead>
                <TableHead className="w-[20%]">余额</TableHead>
                <TableHead className="w-[8%] text-right">操作</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {upstreams.isLoading && <TableLoadingRows columns={7} />}
              {!upstreams.isLoading && !upstreams.error && !hosts.length && (
                <TableMessageRow columns={7}>
                  <EmptyRow
                    text={hasActiveFilters ? "没有匹配的上游 Host" : "当前业务库没有上游 Host"}
                  />
                </TableMessageRow>
              )}
              {hosts.map((host) => {
                const recovering = recover.isPending && recover.variables?.host === host.host;
                const readingBalance =
                  refreshBalance.isPending && refreshBalance.variables === host.host;
                return (
                  <TableRow key={host.host}>
                    <TableCell className="max-w-80">
                      <UpstreamIdentity
                        name={host.name}
                        upstreamId={host.upstream_id}
                        host={host.host}
                        hosts={host.hosts}
                        baseUrl={host.base_url}
                      />
                    </TableCell>
                    <TableCell>{displayUpstreamType(host.upstream_type)}</TableCell>
                    <TableCell>{host.group_count}</TableCell>
                    <TableCell>{host.account_count}</TableCell>
                    <TableCell>
                      <StatusPill
                        label={host.auth_status}
                        tone={
                          host.auth_status === "鉴权失效"
                            ? "danger"
                            : host.auth_status === "待验证"
                              ? "warning"
                              : "success"
                        }
                      />
                    </TableCell>
                    <TableCell>
                      <div className="grid gap-0.5">
                        <span>
                          {host.balance === null
                            ? host.balance_status
                            : formatBalance(host.balance)}
                        </span>
                        {host.checked_at && (
                          <span className="text-muted-foreground text-xs">
                            {formatDate(host.checked_at)}
                          </span>
                        )}
                      </div>
                    </TableCell>
                    <TableCell className="text-right">
                      <div className="flex items-center justify-end gap-1">
                        <TableActionButton
                          label="添加账号"
                          onClick={() =>
                            navigate({
                              to: "/onboarding",
                              search: {
                                host: host.host,
                                upstream_type: host.upstream_type,
                                group_id: undefined,
                              },
                            })
                          }
                        >
                          <UserPlus />
                        </TableActionButton>
                        <TableActionButton
                          label="查看分组"
                          onClick={() => setSelectedHost(host.host)}
                        >
                          <FolderOpen />
                        </TableActionButton>
                        <TableActionButton label="编辑上游" onClick={() => setEditHost(host.host)}>
                          <Pencil />
                        </TableActionButton>
                        <DropdownMenu>
                          <DropdownMenuTrigger
                            render={
                              <Button
                                variant="outline"
                                size="icon-sm"
                                className="data-popup-open:bg-muted"
                                aria-label="更多操作"
                              />
                            }
                          >
                            <MoreHorizontal className="size-4" />
                          </DropdownMenuTrigger>
                          <DropdownMenuContent align="end" className="w-48">
                            <DropdownMenuItem
                              disabled={recovering || actionPending}
                              onClick={() => {
                                recover.reset();
                                setActionTaskId(null);
                                setRecoveryEntry("");
                                setActionDialog({
                                  host: host.host,
                                  kind: "auth",
                                });
                              }}
                            >
                              <ShieldCheck />
                              {recovering ? "恢复鉴权中…" : "恢复鉴权"}
                            </DropdownMenuItem>
                            <DropdownMenuItem
                              disabled={readingBalance || actionPending}
                              onClick={() => {
                                refreshBalance.reset();
                                setActionTaskId(null);
                                setActionDialog({
                                  host: host.host,
                                  kind: "balance",
                                });
                                refreshBalance.mutate(host.host);
                              }}
                            >
                              <WalletCards />
                              {readingBalance ? "同步中…" : "同步余额"}
                            </DropdownMenuItem>
                            <DropdownMenuItem
                              className="text-destructive focus:text-destructive"
                              disabled={actionPending || deletePending}
                              onClick={() => {
                                setDeleteTaskId(null);
                                deleteUpstream.reset();
                                setDeleteHost(host.host);
                              }}
                            >
                              <Trash2 />
                              删除
                            </DropdownMenuItem>
                          </DropdownMenuContent>
                        </DropdownMenu>
                      </div>
                    </TableCell>
                  </TableRow>
                );
              })}
            </TableBody>
          </Table>
          {allHosts.length > 0 && (
            <DataTablePagination
              currentPage={page}
              totalPages={totalPages}
              totalItems={allHosts.length}
              pageSize={pageSize}
              onPageChange={setPage}
              onPageSizeChange={(nextPageSize) => {
                setPageSize(nextPageSize);
                setPage(1);
              }}
            />
          )}
        </Card>
      </div>
      <UpstreamEditDialog
        host={editHost}
        onOpenChange={(open) => {
          if (!open) setEditHost(null);
        }}
        onSaved={() => void upstreams.refetch()}
      />
      <Dialog
        open={managementDialog !== null}
        onOpenChange={(open) => {
          if (!open && !managementPending) {
            setManagementDialog(null);
            setManagementTaskId(null);
            balanceSync.reset();
            groupSync.reset();
            nameRepair.reset();
          }
        }}
      >
        <DialogContent
          width={operationDialogWidth(taskStopsPolling(managementTask.data))}
          height={operationDialogHeight(taskStopsPolling(managementTask.data), "medium")}
          className="grid grid-rows-[auto_minmax(0,1fr)] overflow-hidden"
        >
          <DialogHeader>
            <DialogTitle>
              {managementDialog === "balance"
                ? "同步余额"
                : managementDialog === "groups"
                  ? "同步分组"
                  : "上游名称修复"}
            </DialogTitle>
          </DialogHeader>
          <DialogBody className={cn(managementTask.data && "overflow-hidden pr-0")}>
            {!managementTask.data &&
              !managementTask.error &&
              !balanceSync.error &&
              !groupSync.error &&
              !nameRepair.error && (
                <TaskStartupState
                  message={
                    managementDialog === "balance"
                      ? "正在创建余额同步任务"
                      : managementDialog === "groups"
                        ? "正在创建分组同步任务"
                        : "正在重新读取站点名称"
                  }
                />
              )}
            {(balanceSync.error || groupSync.error || nameRepair.error) && (
              <QueryError
                error={balanceSync.error ?? groupSync.error ?? nameRepair.error}
                fallback={
                  managementDialog === "balance"
                    ? "余额同步启动失败"
                    : managementDialog === "groups"
                      ? "分组同步启动失败"
                      : "上游名称修复启动失败"
                }
                embedded
              />
            )}
            {managementTask.error && (
              <QueryError error={managementTask.error} fallback="同步任务状态读取失败" embedded />
            )}
            {managementTask.data && (
              <UpstreamSyncTaskStatus
                task={managementTask.data}
                scope={
                  managementDialog === "balance"
                    ? "balance"
                    : managementDialog === "groups"
                      ? "groups"
                      : "names"
                }
              />
            )}
          </DialogBody>
        </DialogContent>
      </Dialog>
      <Dialog
        open={syncDialogOpen}
        onOpenChange={(open) => {
          setSyncDialogOpen(open);
          if (!open && !taskIsPending(syncTaskId, syncTask)) {
            setSyncTaskId(null);
            syncUpstreams.reset();
          }
        }}
      >
        <DialogContent
          width={operationDialogWidth(taskStopsPolling(syncTask.data))}
          height={operationDialogHeight(taskStopsPolling(syncTask.data), "tall")}
          className="grid grid-rows-[auto_minmax(0,1fr)] overflow-hidden"
        >
          <DialogHeader>
            <DialogTitle>同步上游</DialogTitle>
          </DialogHeader>
          <DialogBody className={cn(syncTask.data && "overflow-hidden pr-0")}>
            {!syncTask.data && !syncTask.error && !syncUpstreams.error && (
              <TaskStartupState message="正在创建上游同步任务" />
            )}
            {syncUpstreams.error && (
              <QueryError error={syncUpstreams.error} fallback="上游同步启动失败" embedded />
            )}
            {syncTask.error && (
              <QueryError error={syncTask.error} fallback="同步状态读取失败" embedded />
            )}
            {syncTask.data && <UpstreamSyncTaskStatus task={syncTask.data} />}
          </DialogBody>
        </DialogContent>
      </Dialog>
      <Dialog
        open={actionDialog !== null && captchaChallenge === null}
        onOpenChange={(open) => {
          if (!open) {
            setActionDialog(null);
            setActionTaskId(null);
            setRecoveryEntry("");
            recover.reset();
            refreshBalance.reset();
          }
        }}
      >
        <DialogContent
          width="medium"
          height="large"
          className="grid grid-rows-[auto_minmax(0,1fr)] overflow-hidden"
        >
          <DialogHeader>
            <DialogTitle>{actionDialog?.kind === "balance" ? "同步余额" : "恢复鉴权"}</DialogTitle>
          </DialogHeader>
          <DialogBody>
            {actionDialog?.kind === "auth" &&
            (!actionTaskId || actionTask.data?.status === "failed") ? (
              <ManualAuthForm
                host={actionDialog.host}
                upstreamType={actionHost?.upstream_type ?? "sub2api"}
                vaultPending={recover.isPending}
                onVaultRecovery={(entry) => {
                  setRecoveryEntry(entry);
                  recover.mutate({ host: actionDialog.host, entry });
                }}
                onCaptchaRequired={setManualCaptchaChallenge}
                onVerified={(result) => {
                  const completion = manualAuthCompletion(actionDialog.host, result);
                  toast.success(completion.notice);
                  setActionDialog(completion.actionDialog);
                  setActionTaskId(completion.actionTaskId);
                  recover.reset();
                  refreshBalance.reset();
                }}
              />
            ) : null}
            {(actionDialog?.kind === "balance" || recover.isPending || actionTaskId !== null) &&
              !actionTask.data &&
              !actionTask.error &&
              !recover.error &&
              !refreshBalance.error && (
                <TaskStartupState
                  message={
                    actionDialog?.kind === "balance"
                      ? "正在创建余额同步任务"
                      : "正在创建鉴权恢复任务"
                  }
                />
              )}
            {(recover.error || refreshBalance.error) && (
              <QueryError
                error={recover.error ?? refreshBalance.error}
                fallback={
                  actionDialog?.kind === "balance" ? "余额同步启动失败" : "恢复鉴权启动失败"
                }
                embedded
              />
            )}
            {actionTask.error && (
              <QueryError error={actionTask.error} fallback="后台任务状态读取失败" embedded />
            )}
            {actionTask.data &&
              (actionDialog?.kind === "balance" ? (
                <BalanceTaskProgress task={actionTask.data} />
              ) : (
                <AuthTaskProgress task={actionTask.data} />
              ))}
          </DialogBody>
        </DialogContent>
      </Dialog>
      <Dialog
        open={deleteHost !== null}
        onOpenChange={(open) => {
          if (!open && !deletePending) {
            setDeleteHost(null);
            setDeleteTaskId(null);
            deleteUpstream.reset();
          }
        }}
      >
        <DialogContent
          width="medium"
          height="large"
          className="grid grid-rows-[auto_minmax(0,1fr)] overflow-hidden"
        >
          <DialogHeader>
            <DialogTitle>删除上游</DialogTitle>
          </DialogHeader>
          <DialogBody>
            {!deleteTaskId && deletePreview.isLoading && (
              <div className="grid gap-3 py-2" aria-label="正在读取删除范围">
                <Skeleton className="h-5 w-40" />
                <Skeleton className="h-24 w-full" />
              </div>
            )}
            {!deleteTaskId && deletePreview.error && (
              <QueryError error={deletePreview.error} fallback="删除范围读取失败" embedded />
            )}
            {!deleteTaskId && deletePreview.data && (
              <div className="grid gap-4">
                <div className="rounded-lg bg-destructive/10 px-3 py-2.5 text-sm text-destructive">
                  将从 Sub2API 删除 {deletePreview.data.account_count} 个账号，并清理该上游的{" "}
                  {deletePreview.data.group_count} 个分组及 Console 当前调度数据。此操作不可撤销。
                </div>
                {deletePreview.data.accounts.length > 0 ? (
                  <div className="divide-y rounded-lg border">
                    {deletePreview.data.accounts.map((account) => (
                      <div key={account.id} className="grid gap-1 px-3 py-2.5 text-sm">
                        <div className="flex items-center justify-between gap-3">
                          <strong className="min-w-0 truncate font-medium">{account.name}</strong>
                          <span className="text-muted-foreground shrink-0 font-mono text-xs">
                            ID {account.id}
                          </span>
                        </div>
                        <span className="text-muted-foreground text-xs">
                          分组：
                          {account.groups.length ? account.groups.join("、") : "未记录"}
                        </span>
                      </div>
                    ))}
                  </div>
                ) : (
                  <div className="rounded-lg border px-3 py-3 text-sm text-muted-foreground">
                    该上游当前没有关联账号。
                  </div>
                )}
                {!fullMode && (
                  <div className="rounded-lg bg-warning/10 px-3 py-2 text-sm text-warning">
                    删除会写入 Sub2API，请先切换到完全模式。
                  </div>
                )}
                <div className="flex justify-end gap-2">
                  <Button variant="outline" onClick={() => setDeleteHost(null)}>
                    取消
                  </Button>
                  <Button
                    variant="destructive"
                    disabled={!fullMode || deleteUpstream.isPending}
                    onClick={() =>
                      deleteUpstream.mutate({
                        host: deletePreview.data.host,
                        accountIds: deletePreview.data.account_ids,
                      })
                    }
                  >
                    <Trash2 size={16} />
                    {deleteUpstream.isPending ? "正在提交" : "确认删除"}
                  </Button>
                </div>
              </div>
            )}
            {deleteTask.data && <UpstreamDeleteTaskStatus task={deleteTask.data} />}
            {deleteTaskId && !deleteTask.data && !deleteTask.error && (
              <TaskStartupState message="正在创建上游删除任务" />
            )}
            {deleteTask.error && (
              <QueryError error={deleteTask.error} fallback="删除任务状态读取失败" embedded />
            )}
            {deleteTask.data &&
              !["queued", "running", "waiting_input"].includes(deleteTask.data.status) && (
                <div className="mt-4 flex justify-end">
                  <Button
                    variant="outline"
                    onClick={() => {
                      setDeleteHost(null);
                      setDeleteTaskId(null);
                    }}
                  >
                    关闭
                  </Button>
                </div>
              )}
          </DialogBody>
        </DialogContent>
      </Dialog>
      <Dialog
        open={selectedHost !== null}
        onOpenChange={(open) => {
          if (!open) setSelectedHost(null);
        }}
      >
        <DialogContent
          width="table"
          height="tall"
          className="grid grid-rows-[auto_minmax(0,1fr)] overflow-hidden"
        >
          <DialogHeader>
            <DialogTitle>上游分组</DialogTitle>
          </DialogHeader>
          <DialogBody className="grid grid-rows-[minmax(0,1fr)_auto] overflow-hidden rounded-lg border pr-0">
            <Table className="min-w-[820px] table-fixed" containerClassName="min-h-0 overflow-auto">
              <TableHeader className="sticky top-0 z-10">
                <TableRow>
                  <TableHead className="w-[18%]">分组</TableHead>
                  <TableHead className="w-[22%]">介绍</TableHead>
                  <TableHead className="w-[11%]">{upstreamRateLabels.rawRate}</TableHead>
                  <TableHead className="w-[12%]">{upstreamRateLabels.effectiveRate}</TableHead>
                  <TableHead className="w-[19%]">绑定账号</TableHead>
                  <TableHead className="w-[11%] text-right">操作</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {groups.isLoading && <TableLoadingRows columns={6} />}
                {groups.isError && (
                  <TableMessageRow columns={6}>
                    <QueryError error={groups.error} fallback="分组目录读取失败" embedded />
                  </TableMessageRow>
                )}
                {!groups.isLoading && !groups.isError && !groups.data?.length && (
                  <TableMessageRow columns={6}>
                    <EmptyRow text="上游没有返回分组" />
                  </TableMessageRow>
                )}
                {!groups.isError &&
                  groupPageRows.map((group) => {
                    return (
                      <TableRow key={`${group.host}:${group.group_id}`}>
                        <TableCell className="font-medium" tooltipContent={group.name}>
                          {group.name}
                        </TableCell>
                        <TableCell tooltipContent={group.description || "未提供"}>
                          {group.description || "未提供"}
                        </TableCell>
                        <TableCell>{group.raw_rate ?? "未读取"}</TableCell>
                        <TableCell>{group.effective_rate ?? "未计算"}</TableCell>
                        <TableCell>
                          <UpstreamBoundAccountSelect accounts={group.bound_accounts} />
                        </TableCell>
                        <TableCell className="text-right" overflowTooltip={false}>
                          <div className="flex justify-end">
                            <TableActionButton
                              label="添加账号"
                              disabled={!group.group_id}
                              onClick={() => {
                                const upstream = data?.hosts.find(
                                  (item) => item.host === selectedHost,
                                );
                                navigate({
                                  to: "/onboarding",
                                  search: {
                                    host: selectedHost ?? undefined,
                                    upstream_type: upstream?.upstream_type,
                                    group_id: group.group_id ?? undefined,
                                  },
                                });
                                setSelectedHost(null);
                              }}
                            >
                              <UserPlus />
                            </TableActionButton>
                          </div>
                        </TableCell>
                      </TableRow>
                    );
                  })}
              </TableBody>
            </Table>
            {groupRows.length > 0 && (
              <DataTablePagination
                currentPage={groupPage}
                totalPages={groupTotalPages}
                totalItems={groupRows.length}
                pageSize={groupPageSize}
                onPageChange={setGroupPage}
                onPageSizeChange={(nextPageSize) => {
                  setGroupPageSize(nextPageSize);
                  setGroupPage(1);
                }}
              />
            )}
          </DialogBody>
        </DialogContent>
      </Dialog>
      <Sheet
        open={captchaChallenge !== null}
        onOpenChange={(open) => {
          if (!open && captchaChallenge !== null && !cancelCaptcha.isPending)
            cancelCaptcha.mutate(captchaChallenge);
        }}
      >
        <SheetContent className="w-full overflow-y-auto p-0 sm:max-w-md">
          <SheetHeader className="border-b pr-12">
            <SheetTitle>完成图片验证码</SheetTitle>
            <SheetDescription>{captchaChallenge?.host ?? "上游重新鉴权"}</SheetDescription>
          </SheetHeader>
          {captchaChallenge && (
            <div className="grid gap-4 p-4">
              <div className="overflow-hidden rounded-lg bg-muted p-3">
                <img
                  src={captchaChallenge.image_data}
                  alt={`${captchaChallenge.host} 图片验证码`}
                  className="mx-auto max-h-48 max-w-full object-contain"
                />
              </div>
              <FormField label="验证码">
                <Input
                  value={captchaCode}
                  autoComplete="off"
                  autoCapitalize="characters"
                  disabled={captchaExpired || submitCaptcha.isPending || cancelCaptcha.isPending}
                  onChange={(event) => setCaptchaCode(event.target.value)}
                  onKeyDown={(event) => {
                    if (event.key === "Enter" && captchaCode.trim() && !captchaExpired)
                      submitCaptcha.mutate(captchaChallenge);
                  }}
                  placeholder="输入图片中的字符"
                  aria-invalid={submitCaptcha.isError}
                />
              </FormField>
              <div className="text-muted-foreground text-xs">
                {captchaExpired
                  ? "验证码已过期，请重新准备。"
                  : `有效期至 ${formatDate(captchaChallenge.expires_at)}`}
              </div>
              <div className="flex justify-end gap-2">
                <Button
                  variant="outline"
                  disabled={cancelCaptcha.isPending || submitCaptcha.isPending}
                  onClick={() => cancelCaptcha.mutate(captchaChallenge)}
                >
                  取消
                </Button>
                {captchaExpired ? (
                  <Button
                    disabled={cancelCaptcha.isPending || recover.isPending}
                    onClick={prepareCaptchaAgain}
                  >
                    <RefreshCw size={16} />
                    重新准备
                  </Button>
                ) : (
                  <Button
                    disabled={
                      !captchaCode.trim() || submitCaptcha.isPending || cancelCaptcha.isPending
                    }
                    onClick={() => submitCaptcha.mutate(captchaChallenge)}
                  >
                    <ShieldCheck size={16} />
                    {submitCaptcha.isPending ? "复核中…" : "提交并复核"}
                  </Button>
                )}
              </div>
            </div>
          )}
        </SheetContent>
      </Sheet>
    </PageLayout>
  );
}

export function manualAuthCompletion(
  host: string,
  result: Extract<ManualAuthVerifyResult, { verified: true }>,
) {
  const balanceMessage =
    result.balance_sync.status === "succeeded"
      ? "余额已同步"
      : `余额同步失败：${result.balance_sync.reason ?? "上游未返回余额"}`;
  return {
    notice: `${host} 凭证已验证并保存，${balanceMessage}`,
    actionDialog: null,
    actionTaskId: null,
  } as const;
}

export function manualAuthIncomplete(
  authMode: string,
  credentials: {
    accessToken: string;
    refreshToken: string;
    adminKey: string;
    userId: string;
  },
  hasCustomHeaders: boolean,
): boolean {
  if (authMode === "newapi_admin_key") {
    const hasAdminKey = credentials.adminKey.trim() !== "";
    const hasUserId = credentials.userId.trim() !== "";
    if (!hasAdminKey && !hasUserId) return !hasCustomHeaders;
    return !hasAdminKey || !hasUserId;
  }
  if (authMode === "sub2api_user_token") {
    const hasAccessToken = credentials.accessToken.trim() !== "";
    const hasRefreshToken = credentials.refreshToken.trim() !== "";
    if (!hasAccessToken && !hasRefreshToken) return !hasCustomHeaders;
    return !hasAccessToken || !hasRefreshToken;
  }
  if (authMode === "newapi_user_token" || authMode === "bearer_token") {
    return credentials.accessToken.trim() === "" && !hasCustomHeaders;
  }
  if (authMode === "custom_headers") return !hasCustomHeaders;
  return false;
}

export function ManualAuthHeadersEditor(props: {
  value: string;
  error?: string;
  onChange: (value: string) => void;
}) {
  return (
    <FormField label="Headers JSON" error={props.error}>
      <Textarea
        autoGrow
        className="min-h-24 min-w-0 max-w-full whitespace-pre-wrap [overflow-wrap:anywhere]"
        wrap="soft"
        value={props.value}
        onChange={(event) => props.onChange(event.target.value)}
        placeholder='例如 {"Authorization":"Bearer ..."}'
      />
    </FormField>
  );
}

export function ManualAuthForm(props: {
  host: string;
  upstreamType: string;
  onVerified?: (result: Extract<ManualAuthVerifyResult, { verified: true }>) => void;
  onCaptchaRequired?: (challenge: CaptchaChallenge) => void;
  onVaultRecovery?: (entry: string) => void;
  vaultPending?: boolean;
}) {
  const queryClient = useQueryClient();
  const authConfig = useQuery({
    queryKey: ["auth-recovery-config"],
    queryFn: api.authRecoveryConfig,
    staleTime: 15_000,
  });
  const manualModes = authModesForPlatform(props.upstreamType);
  const [authMode, setAuthMode] = useState(manualModes[0]?.value ?? "");
  const [credentials, setCredentials] = useState({
    accessToken: "",
    refreshToken: "",
    adminKey: "",
    userId: "",
    username: "",
    password: "",
    saveToVault: false,
    entry: "",
    headers: "",
  });
  const [entry, setEntry] = useState("");
  const [showCustomHeaders, setShowCustomHeaders] = useState(false);
  const [headerError, setHeaderError] = useState<string | null>(null);
  const mutation = useMutation({
    mutationFn: api.verifyManualAuth,
    onSuccess: (result) => {
      if (!result.verified) {
        props.onCaptchaRequired?.(result.captcha_challenge);
        return;
      }
      setCredentials({
        accessToken: "",
        refreshToken: "",
        adminKey: "",
        userId: "",
        username: "",
        password: "",
        saveToVault: false,
        entry: "",
        headers: "",
      });
      setEntry("");
      setShowCustomHeaders(false);
      setHeaderError(null);
      void queryClient.invalidateQueries({ queryKey: ["upstreams"] });
      void queryClient.invalidateQueries({
        queryKey: ["auth-recovery-config"],
      });
      if (props.onVerified) {
        props.onVerified(result);
      } else {
        toast.success("凭证已验证并保存");
      }
    },
    onError: (error) => notifyOperationError(error, "凭证验证失败"),
  });
  const usesAdminKey = authMode === "newapi_admin_key";
  const usesSub2ApiToken = authMode === "sub2api_user_token";
  const usesUserToken = authMode === "newapi_user_token" || authMode === "bearer_token";
  const usesManualLogin = ["sub2api_manual_login", "newapi_manual_login"].includes(authMode);
  const usesVaultLogin = ["sub2api_user_login", "newapi_user_login"].includes(authMode);
  const vaultOptions = useMemo(
    () =>
      vaultEntriesForHost(authConfig.data?.vault_entries ?? [], props.host, {
        requireEmail: props.upstreamType.toLowerCase() === "sub2api",
      }),
    [authConfig.data?.vault_entries, props.host, props.upstreamType],
  );
  useEffect(() => {
    if (!usesVaultLogin) return;
    if (entry && vaultOptions.some((item) => item.entry === entry)) return;
    setEntry(defaultVaultEntryForHost(vaultOptions, props.host));
  }, [entry, props.host, usesVaultLogin, vaultOptions]);
  const authRecord = authConfig.data?.auth_records.find(
    (record) => record.host.toLowerCase() === props.host.toLowerCase(),
  );
  let incomplete = manualAuthIncomplete(
    authMode,
    credentials,
    (authRecord?.has_headers ?? false) || (showCustomHeaders && credentials.headers.trim() !== ""),
  );
  if (usesManualLogin) {
    incomplete = !credentials.username.trim() || !credentials.password;
  } else if (usesVaultLogin) {
    incomplete = !entry;
  } else if (
    !usesAdminKey &&
    !usesSub2ApiToken &&
    !usesUserToken &&
    authMode !== "custom_headers"
  ) {
    incomplete = true;
  }
  return (
    <form
      className="mt-5 grid w-full min-w-0 max-w-full gap-4 overflow-x-clip border-t pt-5"
      onSubmit={(event) => {
        event.preventDefault();
        setHeaderError(null);
        const hasSubmittedHeaders = showCustomHeaders && credentials.headers.trim() !== "";
        if (usesVaultLogin && props.onVaultRecovery && !hasSubmittedHeaders) {
          props.onVaultRecovery(entry);
          return;
        }
        const payload: Parameters<typeof api.verifyManualAuth>[0] = {
          host: props.host,
          auth_mode: authMode,
        };
        if (usesAdminKey) {
          if (credentials.adminKey.trim()) payload.admin_key = credentials.adminKey.trim();
          if (credentials.userId.trim()) payload.user_id = credentials.userId.trim();
        } else if (usesSub2ApiToken) {
          if (credentials.accessToken.trim()) payload.access_token = credentials.accessToken.trim();
          if (credentials.refreshToken.trim())
            payload.refresh_token = credentials.refreshToken.trim();
        } else if (usesUserToken) {
          if (credentials.accessToken.trim()) payload.access_token = credentials.accessToken.trim();
        } else if (usesVaultLogin) {
          payload.entry = entry;
        } else if (usesManualLogin) {
          payload.username = credentials.username;
          payload.password = credentials.password;
          if (credentials.saveToVault) {
            payload.save_to_vault = true;
            if (credentials.entry.trim()) payload.entry = credentials.entry.trim();
          }
        }
        if (hasSubmittedHeaders) {
          try {
            payload.headers = parseStringMap(credentials.headers, "自定义 Headers");
          } catch (error) {
            setHeaderError(error instanceof Error ? error.message : "自定义 Headers 格式无效");
            return;
          }
        }
        mutation.mutate(payload);
      }}
    >
      <strong className="text-sm">选择鉴权方式</strong>
      <FormField label="鉴权方式">
        <Select value={authMode} onValueChange={(value) => value && setAuthMode(value)}>
          <SelectTrigger>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {manualModes.map((item) => (
              <SelectItem key={item.value} value={item.value}>
                {item.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </FormField>
      {usesAdminKey ? (
        <div className="grid min-w-0 gap-4 sm:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">
          <FormField label="Admin Key">
            <Input
              type="password"
              autoComplete="off"
              value={credentials.adminKey}
              placeholder={sensitiveFieldPlaceholder(
                authRecord?.configured === true,
                "输入 Admin Key",
              )}
              onChange={(event) =>
                setCredentials((current) => ({
                  ...current,
                  adminKey: event.target.value,
                }))
              }
            />
          </FormField>
          <FormField label="User ID">
            <Input
              autoComplete="off"
              value={credentials.userId}
              onChange={(event) =>
                setCredentials((current) => ({
                  ...current,
                  userId: event.target.value,
                }))
              }
            />
          </FormField>
        </div>
      ) : usesSub2ApiToken || usesUserToken ? (
        <div className="grid min-w-0 gap-4 sm:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">
          <FormField label="Token">
            <Input
              type="password"
              autoComplete="off"
              value={credentials.accessToken}
              placeholder={sensitiveFieldPlaceholder(authRecord?.configured === true, "输入 Token")}
              onChange={(event) =>
                setCredentials((current) => ({
                  ...current,
                  accessToken: event.target.value,
                }))
              }
            />
          </FormField>
          {usesSub2ApiToken ? (
            <FormField label="刷新 Token">
              <Input
                type="password"
                autoComplete="off"
                value={credentials.refreshToken}
                placeholder={sensitiveFieldPlaceholder(
                  authRecord?.configured === true,
                  "输入刷新 Token",
                )}
                onChange={(event) =>
                  setCredentials((current) => ({
                    ...current,
                    refreshToken: event.target.value,
                  }))
                }
              />
            </FormField>
          ) : null}
        </div>
      ) : usesVaultLogin ? (
        <FormField label="密码箱密码项">
          <Select
            value={entry}
            onValueChange={(value) => {
              if (!value) return;
              setEntry(value);
            }}
          >
            <SelectTrigger>
              <SelectValue placeholder="选择密码箱项" />
            </SelectTrigger>
            <SelectContent className="min-w-[20rem]">
              {vaultOptions.map((item) => (
                <SelectItem key={item.entry} value={item.entry}>
                  {vaultEntryLabel(item)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          {authConfig.error ? (
            <QueryError error={authConfig.error} fallback="密码箱读取失败" embedded />
          ) : null}
        </FormField>
      ) : usesManualLogin ? (
        <div className="grid min-w-0 gap-4 sm:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">
          <FormField label="用户名">
            <Input
              autoComplete="username"
              value={credentials.username}
              onChange={(event) =>
                setCredentials((current) => ({
                  ...current,
                  username: event.target.value,
                }))
              }
            />
          </FormField>
          <FormField label="密码">
            <Input
              type="password"
              autoComplete="current-password"
              value={credentials.password}
              onChange={(event) =>
                setCredentials((current) => ({
                  ...current,
                  password: event.target.value,
                }))
              }
            />
          </FormField>
          <div className="flex items-center gap-2 sm:col-span-2">
            <Switch
              id="manual-auth-save-to-vault"
              checked={credentials.saveToVault}
              onCheckedChange={(checked) =>
                setCredentials((current) => ({
                  ...current,
                  saveToVault: checked,
                }))
              }
              aria-label="登录成功后保存到密码箱"
            />
            <label className="cursor-pointer text-sm" htmlFor="manual-auth-save-to-vault">
              登录成功后自动保存到密码箱
            </label>
          </div>
          {credentials.saveToVault ? (
            <FormField label="凭据名称（可选）">
              <Input
                value={credentials.entry}
                onChange={(event) =>
                  setCredentials((current) => ({
                    ...current,
                    entry: event.target.value,
                  }))
                }
                placeholder="默认使用 Host"
              />
            </FormField>
          ) : null}
        </div>
      ) : null}
      <div className="grid min-w-0 gap-3 border-t pt-4">
        <div className="flex items-center justify-between gap-3">
          <label
            className="cursor-pointer text-sm font-medium"
            htmlFor="manual-auth-custom-headers"
          >
            自定义 Headers
          </label>
          <Switch
            id="manual-auth-custom-headers"
            checked={showCustomHeaders}
            onCheckedChange={setShowCustomHeaders}
            aria-label="添加自定义 Headers"
          />
        </div>
        {showCustomHeaders ? (
          <ManualAuthHeadersEditor
            value={credentials.headers}
            error={headerError ?? undefined}
            onChange={(value) => {
              setHeaderError(null);
              setCredentials((current) => ({
                ...current,
                headers: value,
              }));
            }}
          />
        ) : null}
      </div>
      <div className="flex justify-end">
        <Button type="submit" disabled={mutation.isPending || props.vaultPending || incomplete}>
          <ShieldCheck size={16} />
          {mutation.isPending || props.vaultPending
            ? "正在验证"
            : usesVaultLogin
              ? "开始恢复"
              : "验证并保存"}
        </Button>
      </div>
    </form>
  );
}

export function AccountsPage() {
  const accounts = useQuery({ queryKey: ["accounts"], queryFn: api.accounts });
  const policy = useQuery({ queryKey: ["policy"], queryFn: api.policy });
  const queryClient = useQueryClient();
  const rows = accounts.data ?? [];
  const manualPrioritySection = policy.data?.advanced_policy?.manual_priority;
  const configuredReservedMax =
    manualPrioritySection !== null &&
    typeof manualPrioritySection === "object" &&
    !Array.isArray(manualPrioritySection)
      ? (manualPrioritySection as Record<string, unknown>).reserved_max
      : null;
  const reservedMax =
    typeof configuredReservedMax === "number" &&
    Number.isInteger(configuredReservedMax) &&
    configuredReservedMax >= 1
      ? configuredReservedMax
      : 10;
  const [search, setSearch] = useState("");
  const [statusFilter, setStatusFilter] = useState<AccountPoolFilter>("all");
  const [groupFilter, setGroupFilter] = useState<string | null>(null);
  const [typeFilter, setTypeFilter] = useState<string | null>(null);
  const [pageSize, setPageSize] = useState(50);
  const [page, setPage] = useState(1);
  const [baseURLCheckOpen, setBaseURLCheckOpen] = useState(false);
  const [baseURLCheckTaskId, setBaseURLCheckTaskId] = useState<string | null>(null);
  const [baseURLCheckResultsReady, setBaseURLCheckResultsReady] = useState(false);
  const [baseURLRepairAccountId, setBaseURLRepairAccountId] = useState<string | null>(null);
  const [baseURLRepairKind, setBaseURLRepairKind] = useState<"base_url" | "upstream_host" | null>(
    null,
  );
  const [baseURLRepairTaskId, setBaseURLRepairTaskId] = useState<string | null>(null);
  const groupOptions = Array.from(new Set(rows.flatMap((account) => account.groups))).sort((a, b) =>
    a.localeCompare(b),
  );
  const groupCounts = new Map<string, number>();
  for (const account of rows) {
    for (const group of new Set(account.groups)) {
      groupCounts.set(group, (groupCounts.get(group) ?? 0) + 1);
    }
  }
  const typeOptions = Array.from(
    new Set(
      rows
        .map((account) => account.account_type ?? account.upstream_type)
        .filter((type): type is string => Boolean(type)),
    ),
  ).sort((a, b) => a.localeCompare(b));
  const typeCounts = new Map<string, number>();
  for (const account of rows) {
    const type = account.account_type ?? account.upstream_type;
    if (type) typeCounts.set(type, (typeCounts.get(type) ?? 0) + 1);
  }
  const filteredRows = rows.filter(
    (account) =>
      searchable(
        [
          account.id,
          account.name,
          account.upstream_host,
          account.upstream_type,
          account.base_url,
          account.upstream_base_url,
          account.key_status,
          account.sub2api_status,
          account.sub2api_error,
          account.base_url_check_reason,
          account.platform,
          account.account_type,
          ...account.groups,
          account.routing_state,
          account.health_status,
        ],
        search,
      ) &&
      accountMatchesPoolFilter(account, statusFilter) &&
      (!groupFilter || account.groups.includes(groupFilter)) &&
      (!typeFilter || (account.account_type ?? account.upstream_type) === typeFilter),
  );
  const totalPages = Math.max(1, Math.ceil(filteredRows.length / pageSize));
  const pageRows = filteredRows.slice((page - 1) * pageSize, page * pageSize);
  const automaticAccountRows = filteredRows.filter((account) => account.manual_priority == null);
  const automaticAccountIDs = automaticAccountRows.map((account) => account.id);
  const rateSyncAccountIDs = filteredRows
    .filter(
      (account) =>
        account.manual_priority == null || account.manual_sync_balance_multiplier === true,
    )
    .map((account) => account.id);
  const baseURLCheckMutation = useMutation({
    mutationFn: () => api.checkAccountConfiguration(automaticAccountIDs),
    onSuccess: (task) => setBaseURLCheckTaskId(task.id),
    onError: (error) => notifyOperationError(error, "配置校验与修复启动失败"),
  });
  const baseURLCheckTask = useQuery({
    queryKey: ["account-base-url-check", baseURLCheckTaskId],
    queryFn: () => api.task(baseURLCheckTaskId!),
    enabled: Boolean(baseURLCheckTaskId),
    refetchInterval: taskPollInterval,
  });
  const baseURLCheckPending =
    baseURLCheckMutation.isPending || taskIsPending(baseURLCheckTaskId, baseURLCheckTask);
  const baseURLRepairMutation = useMutation({
    mutationFn: (input: { accountId: string; kind: "base_url" | "upstream_host" }) =>
      input.kind === "base_url"
        ? api.repairAccountBaseURLs([input.accountId])
        : api.repairAccountUpstreamHosts([input.accountId]),
    onSuccess: (task) => setBaseURLRepairTaskId(task.id),
    onError: (error, input) =>
      notifyOperationError(
        error,
        input.kind === "base_url" ? "Base URL 修复启动失败" : "归属 Host 修复启动失败",
      ),
  });
  const baseURLRepairTask = useQuery({
    queryKey: ["account-upstream-host-repair", baseURLRepairTaskId],
    queryFn: () => api.task(baseURLRepairTaskId!),
    enabled: Boolean(baseURLRepairTaskId),
    refetchInterval: taskPollInterval,
  });
  const baseURLRepairPending =
    baseURLRepairMutation.isPending || taskIsPending(baseURLRepairTaskId, baseURLRepairTask);
  useEffect(() => {
    if (!taskStopsPolling(baseURLCheckTask.data)) return;
    void queryClient.invalidateQueries({ queryKey: ["accounts"] });
    void accounts.refetch().then((result) => {
      if (!result.error) setBaseURLCheckResultsReady(true);
    });
  }, [baseURLCheckTask.data?.status, queryClient]);
  useEffect(() => {
    if (!taskStopsPolling(baseURLRepairTask.data)) return;
    void queryClient.invalidateQueries({ queryKey: ["accounts"] });
    if (baseURLRepairTask.data?.status === "succeeded") {
      toast.success(
        baseURLRepairTask.data.message ||
          (baseURLRepairKind === "base_url" ? "Base URL 已修复" : "归属 Host 已修复"),
      );
    } else if (baseURLRepairTask.data) {
      notifyOperationError(
        new Error(
          baseURLRepairTask.data.message ||
            (baseURLRepairKind === "base_url" ? "Base URL 修复失败" : "归属 Host 修复失败"),
        ),
        baseURLRepairKind === "base_url" ? "Base URL 修复失败" : "归属 Host 修复失败",
      );
    }
    setBaseURLRepairAccountId(null);
    setBaseURLRepairKind(null);
  }, [baseURLRepairTask.data?.status, baseURLRepairKind, queryClient]);
  function startBaseURLCheck() {
    setBaseURLCheckOpen(true);
    if (baseURLCheckPending) return;
    setBaseURLCheckTaskId(null);
    setBaseURLCheckResultsReady(false);
    baseURLCheckMutation.reset();
    baseURLCheckMutation.mutate();
  }
  const [maintenanceKind, setMaintenanceKind] = useState<
    "balance" | "rate" | "revalidate" | "repair" | "cleanup" | null
  >(null);
  const [maintenanceTaskId, setMaintenanceTaskId] = useState<string | null>(null);
  const [missingBindingTargets, setMissingBindingTargets] = useState<AccountMaintenanceItem[]>([]);
  const maintenanceMutation = useMutation({
    mutationFn: (input: {
      kind: "balance" | "rate" | "revalidate" | "repair" | "cleanup";
      accountIds: string[];
    }) => {
      if (input.kind === "balance") return api.syncUpstreamBalances();
      if (input.kind === "rate") return api.syncAccountRates(input.accountIds);
      if (input.kind === "revalidate") return api.revalidateAccounts(input.accountIds);
      if (input.kind === "cleanup") return api.cleanupMissingBindings(input.accountIds);
      return api.repairAccountNames(input.accountIds);
    },
    onSuccess: (task) => setMaintenanceTaskId(task.id),
    onError: (error) => notifyOperationError(error, "账号维护任务启动失败"),
  });
  const maintenanceTask = useQuery({
    queryKey: ["account-maintenance", maintenanceTaskId],
    queryFn: () => api.task(maintenanceTaskId!),
    enabled: Boolean(maintenanceTaskId),
    refetchInterval: taskPollInterval,
  });
  const maintenancePending =
    maintenanceMutation.isPending || taskIsPending(maintenanceTaskId, maintenanceTask);
  useEffect(() => {
    if (!taskStopsPolling(maintenanceTask.data)) return;
    void Promise.all([
      queryClient.invalidateQueries({ queryKey: ["accounts"] }),
      queryClient.invalidateQueries({ queryKey: ["upstreams"] }),
      queryClient.invalidateQueries({ queryKey: ["onboarding-candidates"] }),
    ]);
  }, [maintenanceTask.data?.status, queryClient]);
  function startMaintenance(kind: "balance" | "rate" | "revalidate" | "repair") {
    setMaintenanceKind(kind);
    setMaintenanceTaskId(null);
    maintenanceMutation.reset();
    if (kind !== "repair") {
      maintenanceMutation.mutate({
        kind,
        accountIds: kind === "rate" ? rateSyncAccountIDs : automaticAccountIDs,
      });
    }
  }
  useEffect(() => {
    setPage((current) => Math.min(current, totalPages));
  }, [totalPages]);
  return (
    <PageLayout fixedContent>
      <PageHeading
        eyebrow="ACCOUNTS / MANAGEMENT"
        title="账号管理"
        description="健康分、最近结果、综合延迟、调度倍率和组内分配权重集中查看，可按状态快速定位。"
        action={
          <PageActions>
            <Button
              variant="outline"
              disabled={accounts.isLoading || automaticAccountIDs.length === 0}
              onClick={startBaseURLCheck}
            >
              <ScanSearch className={baseURLCheckPending ? "animate-pulse" : ""} size={16} />
              {baseURLCheckPending ? "配置校验中" : "配置校验与修复"}
            </Button>
            <Button
              variant="outline"
              disabled={maintenancePending}
              onClick={() => startMaintenance("balance")}
            >
              <WalletCards size={16} />
              同步余额
            </Button>
            <Button
              variant="outline"
              disabled={maintenancePending || rateSyncAccountIDs.length === 0}
              onClick={() => startMaintenance("rate")}
            >
              <RefreshCw size={16} />
              同步倍率
            </Button>
            <Button
              variant="outline"
              disabled={maintenancePending || automaticAccountIDs.length === 0}
              onClick={() => startMaintenance("revalidate")}
            >
              <BadgeCheck size={16} />
              批量复验
            </Button>
            <Button
              variant="outline"
              disabled={maintenancePending || automaticAccountIDs.length === 0}
              onClick={() => startMaintenance("repair")}
            >
              <SpellCheck2 size={16} />
              命名修复
            </Button>
          </PageActions>
        }
      />
      {accounts.error && <QueryError error={accounts.error} fallback="账号读取失败" />}
      <div className="flex h-full min-h-0 flex-col gap-2.5 sm:gap-3">
        <TableFilterToolbar data-testid="account-filter-toolbar">
          <SearchField
            value={search}
            onChange={(value) => {
              setSearch(value);
              setPage(1);
            }}
            placeholder="搜索账号、ID、Host 或分组"
          />
          <FilterMenu
            label="分组"
            options={groupOptions}
            value={groupFilter}
            onValueChange={(value) => {
              setGroupFilter(value);
              setPage(1);
            }}
            optionCount={(value) => groupCounts.get(value)}
          />
          <FilterMenu
            label="类型"
            options={typeOptions}
            value={typeFilter}
            onValueChange={(value) => {
              setTypeFilter(value);
              setPage(1);
            }}
            optionLabel={(value) => accountTypeLabel(value) ?? value}
            optionCount={(value) => typeCounts.get(value)}
          />
          <AccountStatusFilter
            value={statusFilter}
            onValueChange={(value) => {
              setStatusFilter(value);
              setPage(1);
            }}
          />
          <Tooltip>
            <TooltipTrigger render={<span className="ml-auto inline-flex" />}>
              <Button
                type="button"
                variant="outline"
                size="icon-sm"
                aria-label="刷新账号池"
                disabled={accounts.isFetching}
                onClick={() => void accounts.refetch()}
              >
                <RefreshCw className={accounts.isFetching ? "animate-spin" : ""} />
              </Button>
            </TooltipTrigger>
            <TooltipContent>刷新</TooltipContent>
          </Tooltip>
        </TableFilterToolbar>
        <Card className="min-h-0 flex-1 gap-0 py-0">
          <Table containerClassName="min-h-0 flex-1 overflow-auto" className="min-w-[1500px]">
            <TableHeader className="sticky top-0 z-10">
              <TableRow>
                <TableHead className="w-64">账号</TableHead>
                <TableHead className="w-32">Sub2API 状态</TableHead>
                <TableHead className="w-28">Key 状态</TableHead>
                <TableHead className="w-36">健康分</TableHead>
                <TableHead className="w-40">最近结果</TableHead>
                <TableHead className="w-32">综合延迟</TableHead>
                <TableHead className="w-28">调度倍率</TableHead>
                <TableHead className="w-24">调度权重</TableHead>
                <TableHead className="w-48">调度参数</TableHead>
                <TableHead className="w-36">状态</TableHead>
                <TableHead className="w-32 text-right">操作</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {accounts.isLoading &&
                Array.from({ length: 6 }, (_, row) => (
                  <TableRow key={`loading:${row}`}>
                    <TableCell colSpan={11}>
                      <div className="flex items-center gap-3 py-2">
                        {Array.from({ length: 11 }, (_, column) => (
                          <Skeleton
                            className={cn("h-4", column === 0 ? "w-44" : "w-20")}
                            key={column}
                          />
                        ))}
                      </div>
                    </TableCell>
                  </TableRow>
                ))}
              {!accounts.isLoading && !filteredRows.length && (
                <TableRow>
                  <TableCell colSpan={11}>
                    <EmptyRow
                      text={
                        search || statusFilter !== "all" || groupFilter || typeFilter
                          ? "没有匹配的账号"
                          : "当前没有账号"
                      }
                    />
                  </TableCell>
                </TableRow>
              )}
              {!accounts.isLoading &&
                pageRows.map((account) => (
                  <AccountRow
                    key={account.id}
                    account={account}
                    accounts={rows}
                    reservedMax={reservedMax}
                  />
                ))}
            </TableBody>
          </Table>
          {filteredRows.length > 0 && (
            <DataTablePagination
              currentPage={page}
              totalPages={totalPages}
              totalItems={filteredRows.length}
              pageSize={pageSize}
              onPageChange={setPage}
              onPageSizeChange={(nextPageSize) => {
                setPageSize(nextPageSize);
                setPage(1);
              }}
            />
          )}
        </Card>
      </div>
      <Dialog
        open={baseURLCheckOpen}
        onOpenChange={(open) => {
          if (!open && baseURLCheckPending) return;
          setBaseURLCheckOpen(open);
        }}
      >
        <DialogContent
          width={operationDialogWidth(baseURLCheckResultsReady, "table")}
          height={operationDialogHeight(baseURLCheckResultsReady, "tall")}
          className="grid grid-rows-[auto_minmax(0,1fr)] overflow-hidden"
        >
          <DialogHeader>
            <DialogTitle>配置校验与修复</DialogTitle>
          </DialogHeader>
          <DialogBody className="overflow-hidden pr-0">
            {baseURLCheckMutation.isPending && (
              <TaskStartupState message="正在启动配置校验与修复" />
            )}
            {baseURLCheckMutation.error && (
              <QueryError
                error={baseURLCheckMutation.error}
                fallback="配置校验与修复启动失败"
                embedded
              />
            )}
            {baseURLCheckTask.error && (
              <QueryError
                error={baseURLCheckTask.error}
                fallback="配置校验与修复状态读取失败"
                embedded
              />
            )}
            {baseURLCheckTaskId && !baseURLCheckTask.data && !baseURLCheckTask.error && (
              <TaskStartupState message="正在读取配置校验与修复任务" />
            )}
            {baseURLCheckTask.data && baseURLCheckPending && (
              <TaskProgressState
                message={displayTaskMessage(
                  baseURLCheckTask.data.message || "正在校验 Base URL 并修复账号参数",
                )}
                progress={baseURLCheckTask.data.progress}
              />
            )}
            {baseURLCheckTask.data &&
              taskStopsPolling(baseURLCheckTask.data) &&
              baseURLCheckTask.data.status !== "succeeded" &&
              (typeof baseURLCheckTask.data.result.base_url !== "object" ||
                baseURLCheckTask.data.result.base_url === null) && (
                <QueryError
                  error={new Error(baseURLCheckTask.data.message || "配置校验与修复失败")}
                  fallback="配置校验与修复失败"
                  embedded
                />
              )}
            {baseURLCheckTask.data?.status === "succeeded" && !baseURLCheckResultsReady && (
              <TaskStartupState message="正在加载校验结果" />
            )}
            {baseURLCheckResultsReady ? (
              <div className="grid h-full min-h-0 grid-rows-[auto_minmax(0,1fr)] gap-3">
                <div className="grid grid-cols-2 divide-x rounded-lg border sm:grid-cols-5">
                  <ResultSummaryRow
                    label="Base URL 已读取"
                    value={`${syncResultCount(baseURLCheckTask.data?.result.base_url_resolved)} 个`}
                  />
                  <ResultSummaryRow
                    label="参数已修复"
                    value={`${syncResultCount(baseURLCheckTask.data?.result.parameters_repaired)} 个`}
                  />
                  <ResultSummaryRow
                    label="参数正常"
                    value={`${syncResultCount(baseURLCheckTask.data?.result.parameters_unchanged)} 个`}
                  />
                  <ResultSummaryRow
                    label="已跳过"
                    value={`${syncResultCount(baseURLCheckTask.data?.result.parameters_skipped)} 个`}
                  />
                  <ResultSummaryRow
                    label="失败"
                    value={`${syncResultCount(baseURLCheckTask.data?.result.failed)} 个`}
                  />
                </div>
                <BaseURLCheckResults
                  accounts={filteredRows}
                  running={baseURLCheckPending}
                  repairing={baseURLRepairPending}
                  repairingAccountId={baseURLRepairAccountId}
                  onRerun={startBaseURLCheck}
                  onRepair={(accountId, kind) => {
                    setBaseURLRepairAccountId(accountId);
                    setBaseURLRepairKind(kind);
                    setBaseURLRepairTaskId(null);
                    baseURLRepairMutation.reset();
                    baseURLRepairMutation.mutate({ accountId, kind });
                  }}
                />
              </div>
            ) : null}
          </DialogBody>
        </DialogContent>
      </Dialog>
      <Dialog
        open={maintenanceKind !== null}
        onOpenChange={(open) => {
          if (!open && !maintenancePending) {
            setMaintenanceKind(null);
            setMaintenanceTaskId(null);
            setMissingBindingTargets([]);
            maintenanceMutation.reset();
          }
        }}
      >
        <DialogContent
          width={operationDialogWidth(taskStopsPolling(maintenanceTask.data))}
          height={operationDialogHeight(taskStopsPolling(maintenanceTask.data), "large")}
          className="grid grid-rows-[auto_minmax(0,1fr)] overflow-hidden"
        >
          <DialogHeader>
            <DialogTitle>
              {maintenanceKind === "balance"
                ? "同步账号余额"
                : maintenanceKind === "rate"
                  ? "同步账号倍率"
                  : maintenanceKind === "revalidate"
                    ? "批量复验绑定"
                    : maintenanceKind === "cleanup"
                      ? "修复失效绑定"
                      : "账号命名修复"}
            </DialogTitle>
            {maintenanceKind !== "balance" && (
              <DialogDescription>
                {maintenanceKind === "cleanup"
                  ? `将处理 ${missingBindingTargets.length} 个已确认不存在的账号。`
                  : maintenanceKind === "rate"
                    ? `将使用账号凭据向上游探测 ${rateSyncAccountIDs.length} 个允许同步的账号当前有效倍率，并写回管理平台；写后读回一致才更新本地。`
                    : `当前筛选结果中有 ${automaticAccountIDs.length} 个自动管理账号，只处理其中已有绑定的账号。`}
              </DialogDescription>
            )}
          </DialogHeader>
          <DialogBody className={cn(maintenanceTask.data && "overflow-hidden pr-0")}>
            {maintenanceKind === "repair" &&
              !maintenanceTaskId &&
              !maintenanceMutation.isPending &&
              !maintenanceMutation.error && (
                <div className="grid gap-4">
                  <div className="border-warning/40 bg-warning/10 rounded-lg border px-4 py-3 text-sm leading-6">
                    将按最新站点名称和账号倍率修复管理平台账号名称。名称已经正确、没有绑定或管理平台不存在的账号不会写入。
                  </div>
                  <div className="max-h-56 min-w-0 divide-y overflow-x-hidden overflow-y-auto rounded-lg border">
                    {automaticAccountRows.map((account) => (
                      <div className="min-w-0 px-3 py-2.5 text-sm" key={account.id}>
                        <strong className="block truncate">
                          {account.name || `账号 ${account.id}`}
                        </strong>
                        <span className="text-muted-foreground block truncate text-xs">
                          ID {account.id} · {account.upstream_host}
                        </span>
                      </div>
                    ))}
                  </div>
                  <div className="flex flex-wrap justify-end gap-2 border-t pt-4">
                    <Button variant="outline" onClick={() => setMaintenanceKind(null)}>
                      取消
                    </Button>
                    <Button
                      onClick={() =>
                        maintenanceMutation.mutate({
                          kind: "repair",
                          accountIds: automaticAccountIDs,
                        })
                      }
                    >
                      <SpellCheck2 size={16} />
                      确认修复 {automaticAccountIDs.length} 个自动管理账号
                    </Button>
                  </div>
                </div>
              )}
            {maintenanceKind === "cleanup" &&
              !maintenanceTaskId &&
              !maintenanceMutation.isPending &&
              !maintenanceMutation.error && (
                <div className="grid gap-4">
                  <div className="border-destructive/40 bg-destructive/10 rounded-lg border px-4 py-3 text-sm leading-6">
                    管理平台中已经没有这些稳定账号
                    ID，无法通过改名恢复。修复将再次复验，只清理仍然不存在的本地账号和绑定；清理后可以重新添加账号。
                  </div>
                  <div className="max-h-56 min-w-0 divide-y overflow-x-hidden overflow-y-auto rounded-lg border">
                    {missingBindingTargets.map((item) => (
                      <div className="min-w-0 px-3 py-2.5 text-sm" key={item.accountId}>
                        <strong className="block truncate">
                          {item.accountName || `账号 ${item.accountId}`}
                        </strong>
                        <span className="text-muted-foreground block truncate text-xs">
                          ID {item.accountId} · {item.upstreamHost}
                        </span>
                      </div>
                    ))}
                  </div>
                  <div className="flex flex-wrap justify-end gap-2 border-t pt-4">
                    <Button variant="outline" onClick={() => setMaintenanceKind(null)}>
                      取消
                    </Button>
                    <Button
                      variant="destructive"
                      onClick={() =>
                        maintenanceMutation.mutate({
                          kind: "cleanup",
                          accountIds: missingBindingTargets.map((item) => item.accountId),
                        })
                      }
                    >
                      <Trash2 size={16} />
                      确认清理 {missingBindingTargets.length} 个失效绑定
                    </Button>
                  </div>
                </div>
              )}
            {maintenanceMutation.isPending && <TaskStartupState message="正在创建账号维护任务" />}
            {maintenanceMutation.error && (
              <QueryError
                error={maintenanceMutation.error}
                fallback="账号维护任务启动失败"
                embedded
              />
            )}
            {maintenanceTask.error && (
              <QueryError
                error={maintenanceTask.error}
                fallback="账号维护任务状态读取失败"
                embedded
              />
            )}
            {maintenanceTask.data &&
              (maintenanceKind === "balance" ? (
                <UpstreamSyncTaskStatus task={maintenanceTask.data} scope="balance" />
              ) : maintenanceKind === "rate" ? (
                <AccountRateSyncTaskStatus task={maintenanceTask.data} />
              ) : (
                <AccountMaintenanceTaskStatus
                  task={maintenanceTask.data}
                  onCleanupMissing={(items) => {
                    setMissingBindingTargets(items);
                    setMaintenanceTaskId(null);
                    maintenanceMutation.reset();
                    setMaintenanceKind("cleanup");
                  }}
                />
              ))}
          </DialogBody>
        </DialogContent>
      </Dialog>
    </PageLayout>
  );
}

type AccountMaintenanceItem = {
  accountId: string;
  accountName: string;
  upstreamHost: string;
  status: string;
  before: string;
  after: string;
  error: string;
};

type AccountRateSyncItem = {
  accountId: string;
  accountName: string;
  upstreamHost: string;
  status: string;
  before: string;
  after: string;
  nameBefore: string;
  nameAfter: string;
  error: string;
};

function accountRateSyncItems(task: Task): AccountRateSyncItem[] {
  if (!Array.isArray(task.result.items)) return [];
  return task.result.items.flatMap((raw) => {
    if (typeof raw !== "object" || raw === null || Array.isArray(raw)) return [];
    const item = raw as Record<string, unknown>;
    return [
      {
        accountId: String(item.account_id ?? ""),
        accountName: String(item.account_name ?? ""),
        upstreamHost: String(item.upstream_host ?? ""),
        status: String(item.status ?? "未返回"),
        before: item.before == null ? "—" : String(item.before),
        after: item.after == null ? "—" : String(item.after),
        nameBefore: String(item.name_before ?? ""),
        nameAfter: String(item.name_after ?? ""),
        error: String(item.error ?? ""),
      },
    ];
  });
}

function usePaginatedItems<T>(items: T[], initialPageSize = 20) {
  const [pageSize, setPageSize] = useState(initialPageSize);
  const [currentPage, setCurrentPage] = useState(1);
  const totalPages = Math.max(1, Math.ceil(items.length / pageSize));
  const page = Math.min(currentPage, totalPages);
  const visibleItems = items.slice((page - 1) * pageSize, page * pageSize);

  useEffect(() => {
    if (currentPage > totalPages) setCurrentPage(totalPages);
  }, [currentPage, totalPages]);

  return {
    currentPage: page,
    pageSize,
    totalPages,
    visibleItems,
    setCurrentPage,
    setPageSize: (value: number) => {
      setPageSize(value);
      setCurrentPage(1);
    },
  };
}

export function AccountRateSyncTaskStatus(props: { task: Task }) {
  const pending = ["queued", "running"].includes(props.task.status);
  const items = accountRateSyncItems(props.task);
  const pagination = usePaginatedItems(items);
  if (pending) {
    return (
      <TaskProgressState
        message={displayTaskMessage(props.task.message)}
        progress={props.task.progress}
      />
    );
  }
  return (
    <div className="flex h-full min-h-0 flex-col gap-4">
      <div className="grid grid-cols-2 divide-x rounded-lg border sm:grid-cols-4">
        <ResultSummaryRow
          label="已更新"
          value={`${syncResultCount(props.task.result.updated)} 个`}
        />
        <ResultSummaryRow
          label="无需更新"
          value={`${syncResultCount(props.task.result.unchanged)} 个`}
        />
        <ResultSummaryRow
          label="未绑定/缺失"
          value={`${syncResultCount(props.task.result.missing)} 个`}
        />
        <ResultSummaryRow label="失败" value={`${syncResultCount(props.task.result.failed)} 个`} />
      </div>
      {props.task.status === "failed" && items.length === 0 && (
        <TaskFailureDetail reason={String(props.task.result.error ?? props.task.message)} />
      )}
      <div className="flex min-h-0 flex-1 flex-col overflow-hidden rounded-lg border">
        <div className="min-h-0 flex-1 divide-y overflow-y-auto">
          {pagination.visibleItems.map((item) => (
            <div
              className="grid gap-2 px-3 py-2.5 text-sm sm:grid-cols-[minmax(0,1fr)_auto_auto] sm:items-center"
              key={item.accountId}
            >
              <div className="min-w-0">
                <strong className="block truncate">
                  {item.accountName || `账号 ${item.accountId}`}
                </strong>
                <span className="text-muted-foreground block truncate text-xs">
                  ID {item.accountId}
                  {item.upstreamHost ? ` · ${item.upstreamHost}` : ""}
                </span>
                {item.status === "已同步" &&
                  item.nameBefore &&
                  item.nameAfter &&
                  item.nameBefore !== item.nameAfter && (
                    <span className="text-muted-foreground mt-1 block truncate text-xs">
                      名称 {item.nameBefore} → {item.nameAfter}
                    </span>
                  )}
              </div>
              <span className="text-muted-foreground tabular-nums">
                {item.before} → {item.after}
              </span>
              <div className="flex items-center gap-2 sm:justify-end">
                <StatusPill
                  label={item.status}
                  tone={
                    item.status === "已同步" || item.status === "已确认一致"
                      ? "success"
                      : item.error
                        ? "danger"
                        : "neutral"
                  }
                />
                {item.error && (
                  <span className="text-destructive max-w-64 text-xs">{item.error}</span>
                )}
              </div>
            </div>
          ))}
          {!items.length && (
            <div className="text-muted-foreground px-3 py-6 text-center text-sm">
              没有倍率同步结果
            </div>
          )}
        </div>
        {items.length > 0 ? (
          <DataTablePagination
            currentPage={pagination.currentPage}
            totalPages={pagination.totalPages}
            totalItems={items.length}
            pageSize={pagination.pageSize}
            pageSizes={[10, 20, 50, 100]}
            onPageChange={pagination.setCurrentPage}
            onPageSizeChange={pagination.setPageSize}
          />
        ) : null}
      </div>
    </div>
  );
}

function accountMaintenanceItems(task: Task): AccountMaintenanceItem[] {
  if (!Array.isArray(task.result.items)) return [];
  return task.result.items.flatMap((raw) => {
    if (typeof raw !== "object" || raw === null || Array.isArray(raw)) return [];
    const item = raw as Record<string, unknown>;
    return [
      {
        accountId: String(item.account_id ?? ""),
        accountName: String(item.account_name ?? ""),
        upstreamHost: String(item.upstream_host ?? ""),
        status: String(item.status ?? "未返回"),
        before: String(item.before ?? ""),
        after: String(item.after ?? ""),
        error: String(item.error ?? ""),
      },
    ];
  });
}

export function AccountDefaultsRepairTaskStatus(props: { task: Task }) {
  const pending = ["queued", "running"].includes(props.task.status);
  const items = accountMaintenanceItems(props.task);
  const pagination = usePaginatedItems(items);
  if (pending) {
    return (
      <TaskProgressState
        message={displayTaskMessage(props.task.message)}
        progress={props.task.progress}
      />
    );
  }
  return (
    <div className="flex h-full min-h-0 flex-col gap-4">
      <div className="grid grid-cols-2 divide-x rounded-lg border sm:grid-cols-4">
        <ResultSummaryRow
          label="已修复"
          value={`${syncResultCount(props.task.result.repaired)} 个`}
        />
        <ResultSummaryRow
          label="无需修复"
          value={`${syncResultCount(props.task.result.unchanged)} 个`}
        />
        <ResultSummaryRow
          label="已跳过"
          value={`${syncResultCount(props.task.result.skipped)} 个`}
        />
        <ResultSummaryRow label="失败" value={`${syncResultCount(props.task.result.failed)} 个`} />
      </div>
      {props.task.status === "failed" && (
        <TaskFailureDetail reason={String(props.task.result.error ?? props.task.message)} />
      )}
      <div className="flex min-h-0 flex-1 flex-col overflow-hidden rounded-lg border">
        <div className="min-h-0 flex-1 divide-y overflow-y-auto">
          {pagination.visibleItems.map((item) => (
            <div
              className="grid gap-1 px-3 py-2.5 text-sm sm:grid-cols-[minmax(0,1fr)_minmax(0,1.5fr)_auto] sm:items-center"
              key={item.accountId}
            >
              <div className="min-w-0">
                <strong className="block truncate">
                  {item.accountName || `账号 ${item.accountId}`}
                </strong>
                <span className="text-muted-foreground block truncate text-xs">
                  ID {item.accountId}
                  {item.upstreamHost ? ` · ${item.upstreamHost}` : ""}
                </span>
              </div>
              <div className="min-w-0 text-xs">
                {item.before && item.after ? (
                  <span className="block break-words">
                    {item.before} → {item.after}
                  </span>
                ) : null}
                {item.error ? (
                  <span className="text-destructive block break-words">{item.error}</span>
                ) : null}
              </div>
              <StatusPill
                label={item.status}
                tone={
                  item.status === "已修复" || item.status === "无需修复"
                    ? "success"
                    : item.error || item.status === "管理平台不存在"
                      ? "danger"
                      : "neutral"
                }
              />
            </div>
          ))}
          {!items.length && (
            <div className="text-muted-foreground px-3 py-6 text-center text-sm">
              当前筛选结果中没有可检查账号
            </div>
          )}
        </div>
        {items.length > 0 ? (
          <DataTablePagination
            currentPage={pagination.currentPage}
            totalPages={pagination.totalPages}
            totalItems={items.length}
            pageSize={pagination.pageSize}
            pageSizes={[10, 20, 50, 100]}
            onPageChange={pagination.setCurrentPage}
            onPageSizeChange={pagination.setPageSize}
          />
        ) : null}
      </div>
    </div>
  );
}

function ExternalUpstreamLink(props: {
  host: string;
  baseUrl?: string | null;
  label?: string;
  className?: string;
}) {
  const host = props.host.trim();
  const rawURL = props.baseUrl?.trim() || host;
  if (!rawURL) return <span className={props.className}>Host 未记录</span>;
  const href = /^https?:\/\//i.test(rawURL) ? rawURL : `https://${rawURL}`;
  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <a
            href={href}
            target="_blank"
            rel="noreferrer"
            className={cn(
              "text-primary inline-flex min-w-0 items-center gap-1 hover:underline",
              props.className,
            )}
            aria-label={`访问上游 ${host || rawURL}`}
          />
        }
      >
        <span className="truncate">{props.label || host || rawURL}</span>
        <ExternalLink size={12} className="shrink-0" aria-hidden="true" />
      </TooltipTrigger>
      <TooltipContent className="max-w-xs break-all">{rawURL}</TooltipContent>
    </Tooltip>
  );
}

export function AccountMaintenanceTaskStatus(props: {
  task: Task;
  onCleanupMissing?: (items: AccountMaintenanceItem[]) => void;
}) {
  const pending = ["queued", "running"].includes(props.task.status);
  const items = accountMaintenanceItems(props.task);
  const pagination = usePaginatedItems(items);
  const missingItems = items.filter((item) => item.status === "管理平台不存在");
  if (pending) {
    return (
      <TaskProgressState
        message={displayTaskMessage(props.task.message)}
        progress={props.task.progress}
      />
    );
  }
  return (
    <div className="flex h-full min-h-0 flex-col gap-4">
      <div className="grid grid-cols-2 divide-x rounded-lg border sm:grid-cols-4">
        <ResultSummaryRow label="已绑定" value={`${syncResultCount(props.task.result.bound)} 个`} />
        <ResultSummaryRow
          label="已确认"
          value={`${syncResultCount(props.task.result.verified)} 个`}
        />
        <ResultSummaryRow
          label="已修复"
          value={`${syncResultCount(props.task.result.renamed) + syncResultCount(props.task.result.cleaned)} 个`}
        />
        <ResultSummaryRow
          label="不存在/失败"
          value={`${syncResultCount(props.task.result.missing) + syncResultCount(props.task.result.failed)} 个`}
        />
      </div>
      {props.task.status === "failed" && (
        <TaskFailureDetail reason={String(props.task.result.error ?? props.task.message)} />
      )}
      <div className="flex min-h-0 flex-1 flex-col overflow-hidden rounded-lg border">
        <div className="min-h-0 flex-1 divide-y overflow-y-auto">
          {pagination.visibleItems.map((item) => (
            <div
              className="grid gap-1 px-3 py-2.5 text-sm sm:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto] sm:items-center"
              key={item.accountId}
            >
              <div className="min-w-0">
                <strong className="block truncate">
                  {item.accountName || `账号 ${item.accountId}`}
                </strong>
                <div className="text-muted-foreground flex min-w-0 items-center gap-1 text-xs">
                  <span className="shrink-0">ID {item.accountId} ·</span>
                  <ExternalUpstreamLink host={item.upstreamHost} />
                </div>
              </div>
              <div className="min-w-0 text-xs">
                {item.before && item.after ? (
                  <span className="block break-words">
                    {item.before} → {item.after}
                  </span>
                ) : null}
                {item.error ? (
                  <span className="text-destructive block break-words">{item.error}</span>
                ) : null}
              </div>
              <StatusPill
                label={item.status}
                tone={
                  item.status === "已确认存在" ||
                  item.status === "已修复" ||
                  item.status === "无需修复" ||
                  item.status === "已清理失效绑定"
                    ? "success"
                    : "danger"
                }
              />
            </div>
          ))}
          {!items.length && (
            <div className="text-muted-foreground px-3 py-6 text-center text-sm">
              当前筛选结果中没有已绑定账号
            </div>
          )}
        </div>
        {items.length > 0 ? (
          <DataTablePagination
            currentPage={pagination.currentPage}
            totalPages={pagination.totalPages}
            totalItems={items.length}
            pageSize={pagination.pageSize}
            pageSizes={[10, 20, 50, 100]}
            onPageChange={pagination.setCurrentPage}
            onPageSizeChange={pagination.setPageSize}
          />
        ) : null}
      </div>
      {missingItems.length > 0 && props.onCleanupMissing ? (
        <div className="flex justify-end">
          <Button
            type="button"
            variant="destructive"
            onClick={() => props.onCleanupMissing?.(missingItems)}
          >
            <Trash2 size={16} />
            修复 {missingItems.length} 个失效绑定
          </Button>
        </div>
      ) : null}
    </div>
  );
}

function AccountRow(props: {
  account: AccountStatus;
  accounts: AccountStatus[];
  reservedMax: number;
}) {
  const account = props.account;
  const queryClient = useQueryClient();
  const [taskId, setTaskId] = useState<string | null>(null);
  const [activeAction, setActiveAction] = useState<string | null>(null);
  const [confirmAction, setConfirmAction] = useState<{
    action: AccountControlAction;
    label: string;
    description: string;
  } | null>(null);
  const [confirmPending, setConfirmPending] = useState(false);
  const [detailsOpen, setDetailsOpen] = useState(false);
  const [manualPriorityOpen, setManualPriorityOpen] = useState(false);
  const details = useQuery({
    queryKey: ["account-detail", account.id],
    queryFn: () => api.account(account.id),
    enabled: detailsOpen,
    retry: false,
  });
  const task = useQuery({
    queryKey: ["account-scheduling", account.id, taskId],
    queryFn: () => api.task(taskId!),
    enabled: Boolean(taskId),
    refetchInterval: taskPollInterval,
  });
  useEffect(() => {
    let refreshScope: TaskRefreshScope = "account-scheduling";
    if (activeAction === "探活测试") refreshScope = "active-probe";
    if (activeAction === "同步账号倍率") refreshScope = "management-sync";
    const keys = terminalRefreshKeys(refreshScope, task.data);
    if (keys.length) {
      void Promise.all([
        ...keys.map((queryKey) => queryClient.invalidateQueries({ queryKey })),
        queryClient.invalidateQueries({
          queryKey: ["account-detail", account.id],
        }),
      ]);
    }
  }, [account.id, activeAction, queryClient, task.data?.status]);
  useEffect(() => {
    if (!taskStopsPolling(task.data) || activeAction === null) return;
    if (task.data?.status === "succeeded") {
      toast.success(`${account.name}：${activeAction}完成`);
    } else {
      toast.error(task.data?.message || `${account.name}：${activeAction}失败`);
    }
    setTaskId(null);
    setActiveAction(null);
  }, [account.name, activeAction, task.data]);
  const pending = taskIsPending(taskId, task);
  async function startTask(label: string, create: () => Promise<Task>): Promise<boolean> {
    try {
      setActiveAction(label);
      const created = await create();
      setTaskId(created.id);
      return true;
    } catch (error) {
      setActiveAction(null);
      notifyOperationError(error, `${label}失败`);
      return false;
    }
  }
  async function control(
    action: AccountControlAction,
    label: string,
    confirmationDescription?: string,
  ) {
    if (confirmationDescription) {
      setConfirmAction({ action, label, description: confirmationDescription });
      return;
    }
    await startTask(label, () => api.setAccountControl(account.id, action));
  }
  async function confirmControl() {
    if (!confirmAction) return;
    setConfirmPending(true);
    await startTask(confirmAction.label, () =>
      api.setAccountControl(account.id, confirmAction.action),
    );
    setConfirmPending(false);
    setConfirmAction(null);
  }
  return (
    <>
      <TableRow className="h-20">
        <TableCell className="align-middle">
          <AccountIdentityCell account={account} />
        </TableCell>
        <TableCell className="align-middle">
          <AccountSub2APIStatusCell account={account} />
        </TableCell>
        <TableCell className="align-middle">
          <AccountKeyStatusCell account={account} />
        </TableCell>
        <TableCell className="align-middle">
          <AccountHealthCell account={account} />
        </TableCell>
        <TableCell className="align-middle">
          <AccountRecentResultsCell account={account} />
        </TableCell>
        <TableCell className="align-middle">
          <AccountLatencyCell account={account} />
        </TableCell>
        <TableCell className="align-middle">
          <span className="font-medium tabular-nums">{account.multiplier ?? "—"}</span>
        </TableCell>
        <TableCell className="align-middle">
          <Tooltip>
            <TooltipTrigger render={<span className="inline-grid cursor-help gap-0.5" />}>
              <span className="font-semibold tabular-nums">{schedulingMetric(account.weight)}</span>
              {account.weight !== null && (
                <span className="text-muted-foreground text-[11px] font-normal">调度权重</span>
              )}
            </TooltipTrigger>
            <TooltipContent>
              每轮根据分组预算和质量分计算调度权重；账号属于多个分组时取各分组结果的平均值
            </TooltipContent>
          </Tooltip>
        </TableCell>
        <TableCell className="align-middle">
          <AccountRoutingParametersCell account={account} />
        </TableCell>
        <TableCell className="align-middle">
          <AccountStateCell account={account} />
        </TableCell>
        <TableCell className="align-middle text-right" overflowTooltip={false}>
          <AccountOperationButtons
            account={account}
            pending={pending || activeAction !== null}
            probePending={activeAction === "探活测试"}
            onProbe={() =>
              void startTask("探活测试", () => api.runActiveProbe({ account_id: account.id }))
            }
            onControl={(action, label, confirmation) => void control(action, label, confirmation)}
            onRateSync={() =>
              void startTask("同步账号倍率", () => api.syncAccountRates([account.id]))
            }
            onManualPriority={() => setManualPriorityOpen(true)}
            onEdit={() => setDetailsOpen(true)}
          />
          {task.error && <QueryError error={task.error} fallback="调度任务状态读取失败" />}
        </TableCell>
      </TableRow>
      <AccountDetailDialog
        open={detailsOpen}
        onOpenChange={setDetailsOpen}
        accountName={account.name}
        accountId={account.id}
      >
        <AccountSettingsPanel
          accountId={account.id}
          query={details}
          onCancel={() => setDetailsOpen(false)}
          onSaved={() => setDetailsOpen(false)}
        />
      </AccountDetailDialog>
      <ManualPriorityDialog
        open={manualPriorityOpen}
        account={account}
        accounts={props.accounts}
        reservedMax={props.reservedMax}
        pending={pending || activeAction !== null}
        onOpenChange={setManualPriorityOpen}
        onAssign={({ priority, loadFactor, concurrency, syncBalanceMultiplier }) => {
          void startTask("设置人工优先位", () =>
            api.setAccountManualPriority(
              account.id,
              priority,
              loadFactor,
              concurrency,
              syncBalanceMultiplier,
            ),
          ).then((started) => started && setManualPriorityOpen(false));
        }}
        onClear={() => {
          void startTask("取消人工优先位", () => api.clearAccountManualPriority(account.id)).then(
            (started) => started && setManualPriorityOpen(false),
          );
        }}
      />
      <ConfirmActionDialog
        open={confirmAction !== null}
        title={`确认${confirmAction?.label ?? "操作"}`}
        description={confirmAction?.description ?? ""}
        confirmLabel={confirmAction?.label ?? "确认"}
        pendingLabel="提交中…"
        pending={confirmPending}
        onOpenChange={(open) => {
          if (!open) setConfirmAction(null);
        }}
        onConfirm={() => void confirmControl()}
      />
    </>
  );
}

export function GroupsPage() {
  const groups = useQuery({
    queryKey: ["groups"],
    queryFn: api.groups,
    refetchOnMount: "always",
  });
  const policy = useQuery({ queryKey: ["policy"], queryFn: api.policy });
  const queryClient = useQueryClient();
  const [allocationGroup, setAllocationGroup] = useState<GroupStatus | null>(null);
  const [excludeTarget, setExcludeTarget] = useState<GroupStatus | null>(null);
  const allocation = useQuery({
    queryKey: ["group-allocation", allocationGroup?.id],
    queryFn: () => api.groupAllocation(allocationGroup!.id!),
    enabled: allocationGroup?.id !== null && allocationGroup?.id !== undefined,
    refetchOnMount: "always",
  });
  const [editingGroup, setEditingGroup] = useState<GroupStatus | null>(null);
  const [editor, setEditor] = useState<GroupPolicyOverrideUpdate | null>(null);
  const invalidate = () =>
    Promise.all([
      queryClient.invalidateQueries({ queryKey: ["groups"] }),
      queryClient.invalidateQueries({ queryKey: ["policy"] }),
      queryClient.invalidateQueries({ queryKey: ["logs"] }),
      queryClient.invalidateQueries({ queryKey: ["overview"] }),
    ]);
  const updateGroup = useMutation({
    mutationFn: ({ id, value }: { id: string; value: GroupPolicyOverrideUpdate }) =>
      api.updateGroupPolicy(id, value),
    onSuccess: async () => {
      await invalidate();
      setEditingGroup(null);
      setEditor(null);
      toast.success("分组策略已保存");
    },
    onError: (error) => toast.error(error instanceof Error ? error.message : "分组策略保存失败"),
  });
  const excludeGroup = useMutation({
    mutationFn: ({ id, excluded }: { id: string; excluded: boolean }) =>
      api.setGroupExcluded(id, excluded),
    onSuccess: async (_, variables) => {
      await invalidate();
      setExcludeTarget(null);
      toast.success(variables.excluded ? "分组已排除" : "分组已恢复管控");
    },
    onError: (error) =>
      toast.error(error instanceof Error ? error.message : "分组管控状态修改失败"),
  });
  const clearGroup = useMutation({
    mutationFn: (id: string) => api.clearGroupPolicy(id),
    onSuccess: async () => {
      await invalidate();
      toast.success("分组策略已回落到全局默认");
    },
    onError: (error) => toast.error(error instanceof Error ? error.message : "分组策略清除失败"),
  });
  const advanced = (policy.data?.advanced_policy ?? {}) as Record<string, unknown>;
  const section = (name: string) => {
    const value = advanced[name];
    return value && typeof value === "object" && !Array.isArray(value)
      ? (value as Record<string, unknown>)
      : {};
  };
  const openEditor = (group: GroupStatus) => {
    if (!group.id) return;
    const override = group.override ?? {};
    const breaker = section("breaker");
    const weights = section("weights");
    const recovery = section("recovery");
    const scaling = section("scaling");
    const probe = section("probe");
    setEditor({
      enabled: override.enabled ?? group.participation_status === "participating",
      strategy: override.strategy ?? (group.strategy as GroupPolicyOverrideUpdate["strategy"]),
      min_pool_size: override.min_pool_size ?? Number(breaker.min_pool_size ?? 1),
      weight_budget: override.weight_budget ?? Number(weights.budget ?? 400),
      balanced_price_ratio:
        override.balanced_price_ratio ?? Number(weights.balanced_price_ratio ?? 0.5),
      breaker_enabled: override.breaker_enabled ?? Boolean(breaker.enabled ?? true),
      recovery_enabled: override.recovery_enabled ?? Boolean(recovery.enabled ?? true),
      weights_enabled: override.weights_enabled ?? Boolean(weights.enabled ?? true),
      scaling_enabled: override.scaling_enabled ?? Boolean(scaling.enabled ?? false),
      probe_enabled: override.probe_enabled ?? Boolean(probe.enabled ?? true),
      probe_interval_seconds:
        override.probe_interval_seconds ??
        Number(probe.interval_seconds ?? policy.data?.probe_interval_seconds ?? 300),
      probe_model: override.probe_model ?? policy.data?.probe_model ?? null,
    });
    setEditingGroup(group);
  };
  const rows = groups.data ?? [];
  const [search, setSearch] = useState("");
  const [pageSize, setPageSize] = useState(20);
  const [page, setPage] = useState(1);
  const filteredRows = rows.filter((group) =>
    searchable(
      [
        group.name,
        group.id,
        group.strategy,
        group.platform,
        ...(group.platforms ?? []),
        group.strategy_source === "global_default" ? "全局默认" : displayStrategy(group.strategy),
        groupStatusMeta(group.status).label,
      ],
      search,
    ),
  );
  const totalPages = Math.max(1, Math.ceil(filteredRows.length / pageSize));
  const pageRows = filteredRows.slice((page - 1) * pageSize, page * pageSize);
  useEffect(() => setPage((current) => Math.min(current, totalPages)), [totalPages]);
  useEffect(() => setPage(1), [search]);
  if (groups.error)
    return (
      <PageLayout>
        <PageHeading
          eyebrow="ROUTING / GROUPS"
          title="分组管理"
          description="查看各分组的账号规模、调度状态和策略。"
        />
        <QueryError error={groups.error} fallback="分组读取失败" />
      </PageLayout>
    );
  return (
    <PageLayout fixedContent>
      <PageHeading
        eyebrow="ROUTING / GROUPS"
        title="分组管理"
        description="查看各分组的账号规模、调度状态和策略。"
      />
      <div className="flex h-full min-h-0 flex-col gap-2.5 sm:gap-3">
        <div className="flex shrink-0 items-center justify-between gap-3">
          <SearchField
            value={search}
            onChange={(value) => {
              setSearch(value);
              setPage(1);
            }}
            placeholder="搜索分组、类型或策略"
          />
        </div>
        <Card className="min-h-0 flex-1 gap-0 py-0">
          <Table containerClassName="min-h-0 flex-1 overflow-auto" className="min-w-[1240px]">
            <TableHeader className="sticky top-0 z-10">
              <TableRow>
                <TableHead className="w-[15%]">分组</TableHead>
                <TableHead className="w-[11%]">类型</TableHead>
                <TableHead className="w-[7%]">账号数</TableHead>
                <TableHead className="w-[10%]">调度开启</TableHead>
                <TableHead className="w-[10%]">调度关闭</TableHead>
                <TableHead className="w-[8%]">状态未知</TableHead>
                <TableHead className="w-[12%]">策略</TableHead>
                <TableHead className="w-[14%]">状态</TableHead>
                <TableHead className="w-[13%] text-right">操作</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {groups.isLoading && <TableLoadingRows columns={9} />}
              {!groups.isLoading && !filteredRows.length && (
                <TableMessageRow columns={9}>
                  <EmptyRow text={search ? "没有匹配的分组" : "当前没有分组"} />
                </TableMessageRow>
              )}
              {pageRows.map((group) => (
                <TableRow key={group.id || group.name}>
                  <TableCell className="font-medium">{group.name}</TableCell>
                  <TableCell>
                    <StatusPill
                      label={groupPlatformSummary(group)}
                      tone={group.platform ? "info" : "neutral"}
                    />
                  </TableCell>
                  <TableCell>{group.account_count}</TableCell>
                  <TableCell>
                    <StatusPill label={`${group.scheduling_open} 开启`} tone="success" />
                  </TableCell>
                  <TableCell>
                    <StatusPill label={`${group.scheduling_closed} 关闭`} tone="danger" />
                  </TableCell>
                  <TableCell>{group.scheduling_unknown}</TableCell>
                  <TableCell>
                    {group.strategy_source === "global_default"
                      ? "全局默认"
                      : displayStrategy(group.strategy)}
                  </TableCell>
                  <TableCell>
                    <StatusPill
                      label={groupStatusMeta(group.status).label}
                      tone={groupStatusMeta(group.status).tone}
                    />
                  </TableCell>
                  <TableCell className="text-right" overflowTooltip={false}>
                    <div className="flex items-center justify-end gap-1">
                      <TableActionButton
                        label="查看分组账号调度状态"
                        disabled={!group.id}
                        onClick={() => setAllocationGroup(group)}
                      >
                        <Gauge />
                      </TableActionButton>
                      <TableActionButton
                        label={group.status === "excluded" ? "恢复管控" : "排除分组"}
                        tone={group.status === "excluded" ? "primary" : "danger"}
                        disabled={!group.id || excludeGroup.isPending}
                        onClick={() => {
                          if (!group.id) return;
                          const excluded = group.status !== "excluded";
                          if (excluded) {
                            setExcludeTarget(group);
                            return;
                          }
                          excludeGroup.mutate({ id: group.id, excluded });
                        }}
                      >
                        {group.status === "excluded" ? <ShieldCheck /> : <ShieldOff />}
                      </TableActionButton>
                      <TableActionButton
                        label="回落到全局策略"
                        disabled={!group.id || !group.override || clearGroup.isPending}
                        onClick={() => group.id && clearGroup.mutate(group.id)}
                      >
                        <RefreshCw />
                      </TableActionButton>
                      <TableActionButton
                        label="编辑分组"
                        disabled={!group.id || group.status === "excluded"}
                        onClick={() => openEditor(group)}
                      >
                        <Pencil />
                      </TableActionButton>
                    </div>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
          {filteredRows.length > 0 && (
            <DataTablePagination
              currentPage={page}
              totalPages={totalPages}
              totalItems={filteredRows.length}
              pageSize={pageSize}
              onPageChange={setPage}
              onPageSizeChange={(nextPageSize) => {
                setPageSize(nextPageSize);
                setPage(1);
              }}
            />
          )}
        </Card>
      </div>
      <GroupAllocationDialog
        group={allocationGroup}
        allocation={allocation.data}
        loading={allocation.isLoading}
        error={allocation.error}
        onClose={() => setAllocationGroup(null)}
      />
      <ConfirmActionDialog
        open={excludeTarget !== null}
        title="确认排除分组"
        description={`排除“${excludeTarget?.name ?? "该分组"}”后，该分组将不再执行探测、熔断或调权，现有配置保持不动。`}
        confirmLabel="排除分组"
        pendingLabel="排除中…"
        pending={excludeGroup.isPending}
        onOpenChange={(open) => {
          if (!open) setExcludeTarget(null);
        }}
        onConfirm={() => {
          if (!excludeTarget?.id) return;
          excludeGroup.mutate({ id: excludeTarget.id, excluded: true });
        }}
      />
      <Dialog
        open={Boolean(editingGroup && editor)}
        onOpenChange={(open) => {
          if (!open && !updateGroup.isPending) {
            setEditingGroup(null);
            setEditor(null);
          }
        }}
      >
        <DialogContent
          width="wide"
          height="large"
          className="grid grid-rows-[auto_minmax(0,1fr)_auto] overflow-hidden"
        >
          <DialogHeader>
            <DialogTitle>编辑分组策略</DialogTitle>
            <DialogDescription>{editingGroup?.name}</DialogDescription>
          </DialogHeader>
          <DialogBody className="overflow-hidden pr-0">
            {editor && <GroupPolicyEditorFields value={editor} onChange={setEditor} />}
          </DialogBody>
          <DialogFooter>
            <Button
              variant="outline"
              disabled={updateGroup.isPending}
              onClick={() => {
                setEditingGroup(null);
                setEditor(null);
              }}
            >
              取消
            </Button>
            <Button
              disabled={!editingGroup?.id || !editor || updateGroup.isPending}
              onClick={() =>
                editingGroup?.id &&
                editor &&
                updateGroup.mutate({ id: editingGroup.id, value: editor })
              }
            >
              {updateGroup.isPending ? "保存中…" : "保存策略"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </PageLayout>
  );
}

function AlertsPage() {
  const alerts = useQuery({
    queryKey: ["alerts"],
    queryFn: api.alerts,
    refetchInterval: 15_000,
  });
  const notifications = useQuery({
    queryKey: ["notification-status"],
    queryFn: api.notificationStatus,
    refetchInterval: 15_000,
  });
  const queryClient = useQueryClient();
  const [search, setSearch] = useState("");
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(10);
  const [taskId, setTaskId] = useState<string | null>(null);
  const [clearDialogOpen, setClearDialogOpen] = useState(false);
  const task = useQuery({
    queryKey: ["alert-evaluation", taskId],
    queryFn: () => api.task(taskId!),
    enabled: Boolean(taskId),
    refetchInterval: taskPollInterval,
  });
  const evaluate = useMutation({
    mutationFn: api.evaluateAlerts,
    onSuccess: (created) => setTaskId(created.id),
    onError: (error) => toast.error(error instanceof Error ? error.message : "告警检测失败"),
  });
  const clearAlerts = useMutation({
    mutationFn: api.clearAlerts,
    onSuccess: async (result) => {
      setPage(1);
      setClearDialogOpen(false);
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["alerts"] }),
        queryClient.invalidateQueries({ queryKey: ["overview"] }),
        queryClient.invalidateQueries({ queryKey: ["notification-status"] }),
      ]);
      toast.success(`已清理 ${result.deleted} 条已结束告警；告警中的记录和通知去重状态已保留`);
    },
    onError: (error) => notifyOperationError(error, "清空告警失败"),
  });
  useEffect(() => {
    const keys = terminalRefreshKeys("alerts", task.data);
    if (keys.length)
      void Promise.all(keys.map((queryKey) => queryClient.invalidateQueries({ queryKey })));
  }, [queryClient, task.data?.status]);
  const evaluating = evaluate.isPending || taskIsPending(taskId, task);
  const alertRows = Array.isArray(alerts.data) ? alerts.data : [];
  const clearableAlertCount = alertRows.filter((alert) => alert.status !== "firing").length;
  const filteredAlerts =
    alertRows.filter((alert) => {
      const objectLabel = alertObjectLabel(alert);
      return searchable(
        [
          alert.event_type,
          alertTypeLabel(alert.event_type),
          alert.object_kind,
          alert.object_id,
          objectLabel,
          alert.cause_code,
          alertCauseLabel(alert.cause_code),
          alert.status,
          alertStatusLabel(alert.status),
          alert.delivery_status,
          alertDeliveryLabel(alert.delivery_status),
          alert.last_error,
        ],
        search,
      );
    }) ?? [];
  const totalPages = Math.max(1, Math.ceil(filteredAlerts.length / pageSize));
  const pageAlerts = filteredAlerts.slice((page - 1) * pageSize, page * pageSize);
  useEffect(() => {
    setPage((current) => Math.min(current, totalPages));
  }, [totalPages]);
  return (
    <PageLayout>
      <PageHeading
        eyebrow="OPERATIONS / ALERTS"
        title="告警通知"
        description="查看上游、鉴权、余额和主动探测告警，以及通知发送结果。"
        action={
          <PageActions>
            <Button variant="outline" disabled={evaluating} onClick={() => evaluate.mutate()}>
              <BellRing size={16} />
              {evaluating ? "检测中…" : "立即检测"}
            </Button>
            <Button variant="outline" onClick={() => void alerts.refetch()}>
              <RefreshCw size={16} />
              刷新
            </Button>
          </PageActions>
        }
      />
      {!notifications.isLoading &&
        !notifications.error &&
        notifications.data?.configured !== true && (
          <div className="text-muted-foreground mb-3 text-sm">
            尚未配置通知渠道；仍可检测并记录告警，但不会发送通知。
          </div>
        )}
      {notifications.error && (
        <QueryError error={notifications.error} fallback="告警队列状态读取失败" />
      )}
      {notifications.data ? (
        <NotificationQueueStatus
          queues={notifications.data.queues}
          loadDetails={api.notificationQueue}
        />
      ) : null}
      {alerts.error && <QueryError error={alerts.error} fallback="告警读取失败" />}
      {task.error && <QueryError error={task.error} fallback="告警检测任务状态读取失败" />}
      {task.data && <TaskStatus task={task.data} onClose={() => setTaskId(null)} />}
      <Card>
        <PanelHeading
          title="告警列表"
          subtitle="每条记录包含对象、原因、时间和通知状态"
          action={
            <AlertListActions
              loading={alerts.isLoading}
              failed={Boolean(alerts.error)}
              clearableCount={clearableAlertCount}
              onClear={() => {
                clearAlerts.reset();
                setClearDialogOpen(true);
              }}
            />
          }
        />
        <TableFilterToolbar className="border-b px-4 py-3">
          <SearchField
            value={search}
            onChange={(value) => {
              setSearch(value);
              setPage(1);
            }}
            placeholder="搜索类型、对象或原因"
          />
        </TableFilterToolbar>
        {alerts.isLoading && <LoadingRows columns={1} />}
        {!alerts.isLoading && !alerts.error && !filteredAlerts.length && (
          <EmptyRow
            text={search ? "没有匹配的告警" : "暂无告警"}
            detail="新的鉴权、余额或主动探测异常会出现在这里。"
          />
        )}
        {!alerts.error &&
          pageAlerts.map((alert) => (
            <RunRow
              key={alert.incident_key}
              status={
                alert.status === "firing"
                  ? "failed"
                  : alert.status === "suppressed"
                    ? "suppressed"
                    : "succeeded"
              }
              title={`${alertTypeLabel(alert.event_type)} · ${alertObjectLabel(alert)}`}
              detail={`${alertCauseLabel(alert.cause_code)} · 首次发现 ${formatDate(alert.first_seen_at)} · 最近检测 ${formatDate(alert.last_seen_at)}${alert.delivered_at ? ` · 最近通知 ${formatDate(alert.delivered_at)}` : ""}`}
              state={`${alertStatusLabel(alert.status)} · ${alertDeliveryLabel(alert.delivery_status, alert.delivery_attempts)}`}
              icon={<BellRing size={15} />}
            />
          ))}
        {!alerts.error && filteredAlerts.length > 0 && (
          <DataTablePagination
            currentPage={page}
            totalPages={totalPages}
            totalItems={filteredAlerts.length}
            pageSize={pageSize}
            onPageChange={setPage}
            onPageSizeChange={(nextPageSize) => {
              setPageSize(nextPageSize);
              setPage(1);
            }}
          />
        )}
      </Card>
      <Dialog
        open={clearDialogOpen}
        onOpenChange={(open) => {
          if (!clearAlerts.isPending) setClearDialogOpen(open);
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>清理已结束告警</DialogTitle>
            <DialogDescription>
              只删除已恢复、已关闭或已停用的告警。告警中的记录和通知去重状态会保留，不会因为清理而重复发送。
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button
              variant="outline"
              disabled={clearAlerts.isPending}
              onClick={() => setClearDialogOpen(false)}
            >
              取消
            </Button>
            <Button
              variant="destructive"
              disabled={clearAlerts.isPending}
              onClick={() => clearAlerts.mutate()}
            >
              <Trash2 size={15} />
              {clearAlerts.isPending ? "清理中…" : "确认清理"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </PageLayout>
  );
}

const onboardingSchema = z.object({
  host: z
    .string()
    .trim()
    .min(1, "请输入上游 Host")
    .refine((value) => normalizeOnboardingHost(value) !== "", "请输入有效的域名或 IP，可包含端口"),
  base_url_protocol: z.enum(["https", "http"]),
  base_url: z
    .string()
    .trim()
    .min(1, "请输入请求 Base URL")
    .refine((value) => !value.includes("://"), "协议请使用左侧下拉选择")
    .refine((value) => {
      try {
        const parsed = new URL(`https://${value}`);
        return Boolean(parsed.hostname) && !parsed.username && !parsed.password;
      } catch {
        return false;
      }
    }, "请输入有效的域名或 IP，可包含端口和路径"),
  upstream_type: z.string().min(2, "请选择上游类型"),
  multiplier: z
    .string()
    .regex(/^\d+(\.\d+)?$/, "倍率必须是正数")
    .refine((value) => Number(value) > 0, "倍率必须大于 0"),
  concurrency: z
    .string()
    .regex(/^\d+$/, "并发必须是整数")
    .refine(
      (value) => Number(value) >= 1 && Number(value) <= 10_000_000,
      "并发必须在 1 到 10000000 之间",
    ),
  priority: z
    .string()
    .regex(/^\d+$/, "优先级必须是整数")
    .refine(
      (value) => Number(value) >= 1 && Number(value) <= 10_000_000,
      "优先级必须在 1 到 10000000 之间",
    ),
  target_group: z.string().min(1, "请选择本地分组"),
  local_group_id: z.string().regex(/^\d+$/, "请选择有效的本地分组"),
  notes: z.string().optional(),
});
type OnboardingForm = z.infer<typeof onboardingSchema>;
type OnboardingConfirmation = {
  mode: "single" | "batch";
  requests: OnboardingRequest[];
  previews: OnboardingBindingPreview[];
};
function onboardingBaseUrl(values: Pick<OnboardingForm, "base_url_protocol" | "base_url">) {
  return composeOnboardingBaseUrl({
    baseUrlProtocol: values.base_url_protocol,
    baseUrl: values.base_url,
  });
}
function onboardingProbeTarget(
  candidate: OnboardingCandidate | undefined,
): (OnboardingProbeTarget & ProbeDialogTarget) | null {
  if (!candidate?.group_id) return null;
  return {
    kind: "onboarding",
    host: candidate.host,
    groupId: candidate.group_id,
    name: candidate.group_name,
  };
}
function OnboardingPage() {
  const navigate = useNavigate();
  const onboardingSearch = useSearch({ from: "/onboarding" });
  const entryKind = onboardingEntryKind(onboardingSearch.host, onboardingSearch.group_id);
  const entryHost = onboardingSearch.host?.trim() || null;
  const activeEntryHost = React.useRef(entryHost);
  activeEntryHost.current = entryHost;
  const entryGroupId = entryKind === "group" ? onboardingSearch.group_id?.trim() || null : null;
  const [taskId, setTaskId] = useState<string | null>(null);
  const [balanceDialogOpen, setBalanceDialogOpen] = useState(false);
  const [balanceTaskId, setBalanceTaskId] = useState<string | null>(null);
  const [onboardingMaintenanceKind, setOnboardingMaintenanceKind] = useState<
    "revalidate" | "repair" | "cleanup" | null
  >(null);
  const [missingBindingTargets, setMissingBindingTargets] = useState<AccountMaintenanceItem[]>([]);
  const [onboardingMaintenanceTaskId, setOnboardingMaintenanceTaskId] = useState<string | null>(
    null,
  );
  const [verifiedUpstream, setVerifiedUpstream] = useState<UpstreamConfiguration | null>(null);
  const [selectedGroupId, setSelectedGroupId] = useState<string | null>(null);
  const [batchBindings, setBatchBindings] = useState<Record<string, string[]>>({});
  const [onboardingConfirmation, setOnboardingConfirmation] =
    useState<OnboardingConfirmation | null>(null);
  const [onboardingSubmitting, setOnboardingSubmitting] = useState(false);
  const [probeTarget, setProbeTarget] = useState<ProbeDialogTarget | null>(null);
  const [detectionAction, setDetectionAction] = useState<"type" | "name" | null>(null);
  const [upstreamName, setUpstreamName] = useState("");
  const [rechargeRate, setRechargeRate] = useState("1");
  const [authMode, setAuthMode] = useState("sub2api_user_token");
  const [showCustomHeaders, setShowCustomHeaders] = useState(false);
  const [credentials, setCredentials] = useState({
    accessToken: "",
    refreshToken: "",
    adminKey: "",
    userId: "",
    username: "",
    password: "",
    saveToVault: false,
    entry: "",
    headers: "",
    cookies: "",
  });
  const queryClient = useQueryClient();
  const form = useForm<OnboardingForm>({
    resolver: zodResolver(onboardingSchema),
    defaultValues: {
      host: "",
      base_url_protocol: "https",
      base_url: "",
      upstream_type: "sub2api",
      multiplier: "1",
      concurrency: "10",
      priority: "1",
      target_group: "",
      local_group_id: "",
      notes: "",
    },
  });
  useEffect(() => {
    if (!onboardingSearch.host) return;
    form.setValue("host", normalizeOnboardingHost(onboardingSearch.host), {
      shouldValidate: true,
    });
    if (onboardingSearch.upstream_type) {
      form.setValue("upstream_type", onboardingSearch.upstream_type);
    }
    setVerifiedUpstream(null);
    setSelectedGroupId(null);
    setBatchBindings({});
  }, [form, onboardingSearch.host, onboardingSearch.upstream_type]);
  const groups = useQuery({
    queryKey: ["groups"],
    queryFn: api.groups,
    refetchOnMount: "always",
  });
  const accountDefaults = useQuery({ queryKey: ["config"], queryFn: api.config });
  useEffect(() => {
    if (!accountDefaults.data) return;
    if (!form.getFieldState("concurrency").isDirty) {
      form.setValue("concurrency", String(accountDefaults.data.account_default_concurrency));
    }
    if (!form.getFieldState("priority").isDirty) {
      form.setValue("priority", String(accountDefaults.data.account_default_priority));
    }
  }, [accountDefaults.data, form]);
  const upstreams = useQuery({
    queryKey: ["upstreams"],
    queryFn: api.upstreams,
  });
  const entryConfiguration = useQuery({
    queryKey: ["upstream-configuration", entryHost],
    queryFn: () => api.upstreamConfiguration(entryHost!),
    enabled: entryHost !== null,
    retry: false,
  });
  const authConfig = useQuery({
    queryKey: ["auth-recovery-config"],
    queryFn: api.authRecoveryConfig,
    staleTime: 15_000,
  });
  const task = useQuery({
    queryKey: ["onboarding-task", taskId],
    queryFn: () => api.task(taskId!),
    enabled: Boolean(taskId),
    refetchInterval: (query) => taskPollInterval(query, 300),
  });
  const balanceTask = useQuery({
    queryKey: ["onboarding-balance-task", balanceTaskId],
    queryFn: () => api.task(balanceTaskId!),
    enabled: balanceTaskId !== null,
    refetchInterval: (query) => taskPollInterval(query, 300),
  });
  const onboardingMaintenanceTask = useQuery({
    queryKey: ["onboarding-account-maintenance", onboardingMaintenanceTaskId],
    queryFn: () => api.task(onboardingMaintenanceTaskId!),
    enabled: onboardingMaintenanceTaskId !== null,
    refetchInterval: (query) => taskPollInterval(query, 300),
  });
  const createUpstream = useMutation({
    mutationFn: api.createUpstream,
    onSuccess: (upstream) => {
      setVerifiedUpstream(upstream);
      void queryClient.invalidateQueries({ queryKey: ["upstreams"] });
      void queryClient.invalidateQueries({
        queryKey: ["auth-recovery-config"],
      });
      toast.success("上游已添加并通过验证");
    },
  });
  const detection = useMutation({ mutationFn: api.detectUpstream });
  const prepare = useMutation({
    mutationFn: api.prepareOnboarding,
    onSuccess: (context, requestedHost) => {
      if (
        activeEntryHost.current !== null &&
        activeEntryHost.current.toLocaleLowerCase() !== requestedHost.trim().toLocaleLowerCase()
      ) {
        return;
      }
      setVerifiedUpstream(context.upstream);
      setSelectedGroupId(null);
      setBatchBindings({});
      setOnboardingConfirmation(null);
      setTaskId(null);
    },
    onError: (error) => notifyOperationError(error, "上游信息获取失败"),
  });
  const balanceSync = useMutation({
    mutationFn: api.runBalanceSync,
    onSuccess: (queuedTask) => setBalanceTaskId(queuedTask.id),
    onError: (error) => notifyOperationError(error, "余额同步启动失败"),
  });
  const onboardingMaintenance = useMutation({
    mutationFn: (input: { kind: "revalidate" | "repair" | "cleanup"; accountIds: string[] }) => {
      if (input.kind === "revalidate") return api.revalidateAccounts(input.accountIds);
      if (input.kind === "cleanup") return api.cleanupMissingBindings(input.accountIds);
      return api.repairAccountNames(input.accountIds);
    },
    onSuccess: (queuedTask) => setOnboardingMaintenanceTaskId(queuedTask.id),
    onError: (error) => notifyOperationError(error, "绑定账号维护任务启动失败"),
  });
  const preparedEntryHost = React.useRef<string | null>(null);
  useEffect(() => {
    if (!entryHost) {
      preparedEntryHost.current = null;
      return;
    }
    const configuration = entryConfiguration.data;
    if (!configuration) return;
    const parsedBaseUrl = parseOnboardingBaseUrl(configuration.base_url);
    form.setValue("host", configuration.host, { shouldValidate: true });
    form.setValue("base_url_protocol", parsedBaseUrl.baseUrlProtocol);
    form.setValue("base_url", parsedBaseUrl.baseUrl, { shouldValidate: true });
    form.setValue("upstream_type", configuration.upstream_type, {
      shouldValidate: true,
    });
    setVerifiedUpstream(configuration);
    setUpstreamName(configuration.name);
    setRechargeRate(configuration.recharge_rate || "1");
    setAuthMode(configuration.auth_mode);
    if (preparedEntryHost.current === entryHost) return;
    preparedEntryHost.current = entryHost;
    setSelectedGroupId(null);
    prepare.reset();
    prepare.mutate(entryHost);
  }, [entryConfiguration.data, entryHost]);
  const upstreamType = form.watch("upstream_type");
  const authModes = authModesForPlatform(upstreamType);
  useEffect(() => {
    if (authModes.some((item) => item.value === authMode)) return;
    setAuthMode(authModes[0]?.value ?? "custom_headers");
  }, [authMode, authModes]);

  async function runUpstreamDetection(action: "type" | "name") {
    const valid = await form.trigger("base_url");
    if (!valid) return;
    setDetectionAction(action);
    try {
      const result = await detection.mutateAsync(onboardingBaseUrl(form.getValues()));
      if (action === "type") {
        if (!result.type_detected || !result.upstream_type) {
          toast.error("未能识别上游类型，请手动选择");
          return;
        }
        form.setValue("upstream_type", result.upstream_type, {
          shouldValidate: true,
        });
        const defaultAuthMode =
          result.auth_mode ?? authModesForPlatform(result.upstream_type)[0]?.value;
        if (defaultAuthMode) setAuthMode(defaultAuthMode);
        if (result.name && !upstreamName.trim()) setUpstreamName(result.name);
        toast.success(`已识别为 ${displayUpstreamType(result.upstream_type)}`);
        return;
      }
      if (!result.name_detected || !result.name) {
        toast.error("未读取到上游名称，请手动填写");
        return;
      }
      setUpstreamName(result.name);
      toast.success("已获取上游名称");
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "上游公开信息读取失败");
    } finally {
      setDetectionAction(null);
    }
  }

  async function completeUpstreamStep() {
    const valid = await form.trigger(["host", "base_url", "upstream_type"]);
    if (!valid) return;
    const values = form.getValues();
    const baseUrl = onboardingBaseUrl(values);
    const normalizedHost = normalizeOnboardingHost(values.host);
    const existing = upstreams.data?.hosts.find(
      (item) => item.host.toLowerCase() === normalizedHost,
    );
    if (existing) {
      const authReady = new Set([
        "已鉴权",
        "已恢复",
        "已认证",
        "authenticated",
        "authorized",
        "healthy",
        "valid",
        "ok",
        "succeeded",
      ]).has(existing.auth_status.trim().toLowerCase());
      if (!authReady) {
        toast.error(`该上游当前${existing.auth_status}，请先在上游管理中恢复鉴权`);
        return;
      }
      try {
        const configuration = await queryClient.fetchQuery({
          queryKey: ["upstream-configuration", existing.host],
          queryFn: () => api.upstreamConfiguration(existing.host),
        });
        setVerifiedUpstream(configuration);
        setUpstreamName(configuration.name);
        setRechargeRate(configuration.recharge_rate || "1");
        setAuthMode(configuration.auth_mode);
        setSelectedGroupId(null);
        prepare.reset();
      } catch (error) {
        notifyOperationError(error, "已有上游读取失败");
      }
      return;
    }
    try {
      const payload = {
        host: normalizedHost,
        name: upstreamName.trim() || normalizedHost,
        base_url: baseUrl,
        upstream_type: upstreamType,
        auth_mode: authMode,
        recharge_rate: rechargeRate,
      } as Parameters<typeof api.createUpstream>[0];
      let customHeaders: Record<string, string> | undefined;
      if ((showCustomHeaders || authMode === "custom_headers") && credentials.headers.trim()) {
        customHeaders = parseStringMap(credentials.headers, "自定义 Headers");
        payload.headers = customHeaders;
      }
      const headerAuthenticationConfigured = Object.values(customHeaders ?? {}).some(
        (value) => value.trim() !== "",
      );
      if (
        ["sub2api_user_token", "newapi_admin_key", "newapi_user_token", "bearer_token"].includes(
          authMode,
        ) &&
        manualAuthIncomplete(authMode, credentials, headerAuthenticationConfigured)
      ) {
        throw new Error("请填写完整鉴权凭据，或配置包含鉴权信息的自定义 Header");
      }
      if (authMode === "sub2api_user_token") {
        if (credentials.accessToken.trim()) payload.access_token = credentials.accessToken.trim();
        if (credentials.refreshToken.trim()) {
          payload.refresh_token = credentials.refreshToken.trim();
        }
      } else if (authMode === "newapi_admin_key") {
        if (credentials.adminKey.trim()) payload.admin_key = credentials.adminKey.trim();
        if (credentials.userId.trim()) payload.user_id = credentials.userId.trim();
      } else if (["newapi_user_token", "bearer_token"].includes(authMode)) {
        if (credentials.accessToken.trim()) payload.access_token = credentials.accessToken.trim();
      } else if (["sub2api_user_login", "newapi_user_login"].includes(authMode)) {
        if (!credentials.entry) {
          throw new Error("请选择密码箱项");
        }
        payload.entry = credentials.entry;
      } else if (["sub2api_manual_login", "newapi_manual_login"].includes(authMode)) {
        payload.username = credentials.username.trim();
        payload.password = credentials.password;
        payload.save_to_vault = credentials.saveToVault;
        if (credentials.entry.trim()) payload.entry = credentials.entry.trim();
      }
      if (authMode === "custom_headers") {
        payload.headers = customHeaders ?? parseStringMap(credentials.headers, "自定义 Headers");
      }
      if (authMode === "cookie") {
        payload.cookies = parseStringMap(credentials.cookies, "Cookies");
      }
      await createUpstream.mutateAsync(payload);
    } catch (error) {
      notifyOperationError(error, "上游添加失败");
    }
  }
  async function execute() {
    const valid = await form.trigger(["multiplier", "concurrency", "priority", "local_group_id"]);
    if (!valid) return;
    const values = form.getValues();
    form.clearErrors("host");
    const candidate = visibleCandidates.find((item) => item.group_id === selectedGroupId);
    const existingBinding = candidate ? candidateHasExistingBinding(candidate) : false;
    if (
      !candidate ||
      !candidate.group_id ||
      (!candidateCanCreateKey(candidate) && !existingBinding)
    ) {
      form.setError("host", {
        type: "manual",
        message: "所选上游分组当前不可用于添加账号",
      });
      return;
    }
    const localGroupIDs =
      batchBindings[candidate.group_id] ?? candidateBoundLocalGroupIDs(candidate);
    const selectedLocalGroups = localGroupIDs.flatMap((groupID) => {
      const group = localGroups.find((item) => item.id === groupID);
      return group ? [group] : [];
    });
    if (selectedLocalGroups.length !== localGroupIDs.length || selectedLocalGroups.length === 0) {
      form.setError("local_group_id", {
        type: "manual",
        message: "请至少选择一个有效的本地分组",
      });
      return;
    }
    const existingGroupIDs = candidateBoundLocalGroupIDs(candidate);
    if (existingBinding && sameOnboardingGroupSelection(localGroupIDs, existingGroupIDs)) {
      toast.info("本地分组没有变化，无需提交");
      return;
    }
    const request: OnboardingRequest = {
      host: onboardingRequestHost(verifiedUpstream, values.host),
      upstream_type: verifiedUpstream?.upstream_type ?? values.upstream_type,
      notes: values.notes,
      multiplier: values.multiplier,
      concurrency: Number(values.concurrency),
      priority: Number(values.priority),
      local_group_ids: localGroupIDs.map(Number),
      upstream_group_id: candidate.group_id,
      account_ids: existingBinding ? candidateBoundAccountIDs(candidate) : undefined,
      schedulable: false,
    };
    setOnboardingConfirmation({
      mode: "single",
      requests: [request],
      previews: [
        {
          upstream: preparedData?.upstream.name ?? verifiedUpstream?.name ?? request.host,
          upstreamGroup: candidate.group_name,
          multiplier: request.multiplier,
          localGroupMultiplier: localGroupMultiplierLabel(selectedLocalGroups),
          localGroup: selectedLocalGroups.map((group) => group.name).join("、"),
          concurrency: request.concurrency ?? 0,
          priority: request.priority ?? 0,
          status: existingBinding ? "待更新" : "待添加",
        },
      ],
    });
  }
  async function executeBatch() {
    const valid = await form.trigger(["concurrency", "priority"]);
    if (!valid) return;
    const values = form.getValues();
    const selections = visibleCandidates.flatMap((candidate) => {
      if (!candidate.group_id || !candidate.multiplier) return [];
      const existingBinding = candidateHasExistingBinding(candidate);
      if (!candidateCanCreateKey(candidate) && !existingBinding) return [];
      const localGroupIDs =
        batchBindings[candidate.group_id] ?? candidateBoundLocalGroupIDs(candidate);
      if (localGroupIDs.length === 0) return [];
      const selectedLocalGroups = localGroupIDs.flatMap((groupID) => {
        const group = localGroups.find((item) => item.id === groupID);
        return group ? [group] : [];
      });
      if (selectedLocalGroups.length !== localGroupIDs.length) return [];
      if (
        existingBinding &&
        sameOnboardingGroupSelection(localGroupIDs, candidateBoundLocalGroupIDs(candidate))
      ) {
        return [];
      }
      return [
        {
          request: {
            host: onboardingRequestHost(verifiedUpstream, values.host),
            upstream_type: verifiedUpstream?.upstream_type ?? values.upstream_type,
            notes: values.notes,
            multiplier: candidate.multiplier,
            concurrency: Number(values.concurrency),
            priority: Number(values.priority),
            local_group_ids: localGroupIDs.map(Number),
            upstream_group_id: candidate.group_id,
            account_ids: existingBinding ? candidateBoundAccountIDs(candidate) : undefined,
            schedulable: false,
          } satisfies OnboardingRequest,
          preview: {
            upstream: preparedData?.upstream.name ?? verifiedUpstream?.name ?? values.host.trim(),
            upstreamGroup: candidate.group_name,
            multiplier: candidate.multiplier,
            localGroupMultiplier: localGroupMultiplierLabel(selectedLocalGroups),
            localGroup: selectedLocalGroups.map((group) => group.name).join("、"),
            concurrency: Number(values.concurrency),
            priority: Number(values.priority),
            status: existingBinding ? "待更新" : "待添加",
          } satisfies OnboardingBindingPreview,
        },
      ];
    });
    if (selections.length === 0) {
      toast.error("请至少为一个上游分组选择本地分组");
      return;
    }
    setOnboardingConfirmation({
      mode: "batch",
      requests: selections.map((selection) => selection.request),
      previews: selections.map((selection) => selection.preview),
    });
  }
  async function confirmOnboarding() {
    if (!onboardingConfirmation || onboardingSubmitting) return;
    setOnboardingSubmitting(true);
    try {
      const created =
        onboardingConfirmation.mode === "single"
          ? await api.onboard(onboardingConfirmation.requests[0]!)
          : await api.onboardBatch(onboardingConfirmation.requests);
      setOnboardingConfirmation(null);
      setTaskId(created.id);
    } catch (error) {
      notifyOperationError(error, "账号绑定变更失败");
    } finally {
      setOnboardingSubmitting(false);
    }
  }
  useEffect(() => {
    const keys = terminalRefreshKeys("onboarding", task.data);
    if (keys.length)
      void Promise.all(keys.map((queryKey) => queryClient.invalidateQueries({ queryKey })));
  }, [queryClient, task.data?.status]);
  useEffect(() => {
    if (balanceTask.data?.status !== "succeeded") return;
    const host = verifiedUpstream?.host;
    if (!host) return;
    void Promise.all([
      queryClient.invalidateQueries({ queryKey: ["upstreams"] }),
      queryClient.invalidateQueries({
        queryKey: ["upstream-configuration", host],
      }),
    ]);
    prepare.mutate(host);
  }, [balanceTask.data?.status]);
  useEffect(() => {
    if (!taskStopsPolling(onboardingMaintenanceTask.data)) return;
    const host = verifiedUpstream?.host;
    void Promise.all([
      queryClient.invalidateQueries({ queryKey: ["accounts"] }),
      queryClient.invalidateQueries({ queryKey: ["upstreams"] }),
      queryClient.invalidateQueries({ queryKey: ["onboarding-candidates"] }),
    ]);
    if (host) prepare.mutate(host);
  }, [onboardingMaintenanceTask.data?.status]);
  const prepareMatchesEntry =
    entryHost === null ||
    prepare.variables?.trim().toLocaleLowerCase() === entryHost.toLocaleLowerCase();
  const preparedData = prepareMatchesEntry ? prepare.data : undefined;
  const preparedError = prepareMatchesEntry ? prepare.error : null;
  const preparing = prepareMatchesEntry && prepare.isPending;
  const localGroups = groups.error ? [] : (groups.data?.filter((group) => group.id) ?? []);
  const visibleCandidates = (preparedData?.candidates ?? []).map((candidate) =>
    candidate.unavailable_reason === "" ? { ...candidate, unavailable_reason: "空值" } : candidate,
  );
  useEffect(() => {
    if (!preparedData) return;
    const bindings: Record<string, string[]> = {};
    for (const candidate of preparedData.candidates) {
      if (!candidate.group_id) continue;
      const groupIDs = candidateBoundLocalGroupIDs(candidate);
      if (groupIDs.length > 0) bindings[candidate.group_id] = groupIDs;
    }
    setBatchBindings(bindings);
    if (entryGroupId) {
      form.setValue("local_group_id", bindings[entryGroupId]?.[0] ?? "", {
        shouldValidate: false,
      });
    }
  }, [entryGroupId, form, preparedData]);
  const entryCandidate = entryGroupId
    ? visibleCandidates.find((candidate) => candidate.group_id === entryGroupId)
    : undefined;
  const entrySelectedLocalGroups =
    entryCandidate && entryGroupId
      ? (batchBindings[entryGroupId] ?? candidateBoundLocalGroupIDs(entryCandidate)).flatMap(
          (groupID) => {
            const group = localGroups.find((item) => item.id === groupID);
            return group ? [group] : [];
          },
        )
      : [];
  const displayedCandidates = entryGroupId
    ? entryCandidate
      ? [entryCandidate]
      : []
    : visibleCandidates;
  const displayedBoundAccounts = Array.from(
    new Map(
      displayedCandidates
        .flatMap((candidate) => candidate.bound_accounts)
        .map((account) => [account.account_id, account]),
    ).values(),
  );
  const boundAccountIDs = displayedBoundAccounts.map((account) => account.account_id);
  const entryCandidateSelectable = entryCandidate
    ? candidateCanCreateKey(entryCandidate) || candidateHasExistingBinding(entryCandidate)
    : false;
  const entryProbeTarget = onboardingProbeTarget(entryCandidate);
  const candidateStats = onboardingCandidateStats(visibleCandidates);
  const batchBindingCount = visibleCandidates.filter((candidate) => {
    if (!candidate.group_id) return false;
    const selected = batchBindings[candidate.group_id] ?? candidateBoundLocalGroupIDs(candidate);
    if (
      selected.length === 0 ||
      selected.some((id) => !localGroups.some((group) => group.id === id))
    ) {
      return false;
    }
    if (candidateHasExistingBinding(candidate)) {
      return !sameOnboardingGroupSelection(selected, candidateBoundLocalGroupIDs(candidate));
    }
    return candidateCanCreateKey(candidate);
  }).length;
  let entrySubmitLabel = "预览添加账号";
  if (onboardingSubmitting || taskIsPending(taskId, task)) {
    entrySubmitLabel = "正在提交";
  } else if (entryCandidate && candidateHasExistingBinding(entryCandidate)) {
    entrySubmitLabel = "预览更新绑定";
  }
  const onboardingMaintenancePending =
    onboardingMaintenance.isPending ||
    taskIsPending(onboardingMaintenanceTaskId, onboardingMaintenanceTask);
  const adjacentUpstreams = adjacentOnboardingUpstreams(upstreams.data?.hosts ?? [], entryHost);
  function switchOnboardingUpstream(target: NonNullable<typeof adjacentUpstreams.previous>) {
    setTaskId(null);
    setBalanceTaskId(null);
    setOnboardingMaintenanceKind(null);
    setOnboardingMaintenanceTaskId(null);
    setMissingBindingTargets([]);
    setVerifiedUpstream(null);
    setSelectedGroupId(null);
    setBatchBindings({});
    setOnboardingConfirmation(null);
    setProbeTarget(null);
    prepare.reset();
    void navigate({
      to: "/onboarding",
      search: {
        host: target.host,
        upstream_type: target.upstream_type,
        group_id: undefined,
      },
    });
  }
  const onboardingHeadingActions = (
    <OnboardingHeadingActions
      onBack={() => void navigate({ to: "/upstreams" })}
      previousUpstream={
        adjacentUpstreams.previous
          ? {
              label: adjacentUpstreams.previous.name || adjacentUpstreams.previous.host,
              onSelect: () => switchOnboardingUpstream(adjacentUpstreams.previous!),
            }
          : null
      }
      nextUpstream={
        adjacentUpstreams.next
          ? {
              label: adjacentUpstreams.next.name || adjacentUpstreams.next.host,
              onSelect: () => switchOnboardingUpstream(adjacentUpstreams.next!),
            }
          : null
      }
    />
  );
  function startOnboardingMaintenance(kind: "revalidate" | "repair") {
    setOnboardingMaintenanceKind(kind);
    setMissingBindingTargets([]);
    setOnboardingMaintenanceTaskId(null);
    onboardingMaintenance.reset();
    if (kind === "revalidate") {
      onboardingMaintenance.mutate({ kind, accountIds: boundAccountIDs });
    }
  }
  useEffect(() => {
    if (!entryGroupId || !preparedData) return;
    const candidate = preparedData.candidates.find((item) => item.group_id === entryGroupId);
    const canSelect = candidate
      ? candidateCanCreateKey(candidate) || candidateHasExistingBinding(candidate)
      : false;
    if (!candidate || !canSelect) {
      setSelectedGroupId(null);
      return;
    }
    setSelectedGroupId(entryGroupId);
    if (candidate.multiplier && Number(candidate.multiplier) > 0) {
      form.setValue("multiplier", candidate.multiplier, {
        shouldValidate: true,
      });
    }
  }, [entryGroupId, form, preparedData]);
  if (groups.error)
    return (
      <PageLayout>
        <PageHeading
          eyebrow="ACCOUNT / ONBOARDING"
          title="账号添加"
          description="读取上游分组候选，选择本地分组后创建账号。凭据不进入数据库。"
          action={onboardingHeadingActions}
        />
        <QueryError error={groups.error} fallback="本地分组读取失败" />
      </PageLayout>
    );
  const onboardingPending = onboardingSubmitting || taskIsPending(taskId, task);
  const usesAdminKey = authMode === "newapi_admin_key";
  const usesSub2ApiToken = authMode === "sub2api_user_token";
  const usesToken = ["newapi_user_token", "bearer_token"].includes(authMode);
  const usesVaultLogin = ["sub2api_user_login", "newapi_user_login"].includes(authMode);
  const usesManualLogin = ["sub2api_manual_login", "newapi_manual_login"].includes(authMode);
  const vaultOptions = vaultEntriesForHost(
    authConfig.data?.vault_entries ?? [],
    form.watch("host"),
    { requireEmail: upstreamType === "sub2api" },
  );
  let selectionCardClass: string | undefined;
  if (entryKind === "host") {
    selectionCardClass = "h-full min-h-0";
  } else if (!entryHost) {
    selectionCardClass = "mt-3 sm:mt-4";
  }
  const balanceSyncPending = balanceSync.isPending || taskIsPending(balanceTaskId, balanceTask);
  return (
    <PageLayout fixedContent={entryKind === "host"}>
      <PageHeading
        eyebrow="ACCOUNT / ONBOARDING"
        title="账号添加"
        description={onboardingEntryDescription(entryKind)}
        action={onboardingHeadingActions}
      />
      {!entryHost ? (
        <div className="mb-3 grid grid-cols-2 overflow-hidden rounded-lg border sm:mb-4">
          <div
            className={cn(
              "flex items-center gap-2 px-3 py-2.5 text-sm",
              verifiedUpstream ? "bg-success/10 text-success" : "bg-muted/50 font-medium",
            )}
          >
            <span className="flex size-6 items-center justify-center rounded-full border">
              {verifiedUpstream ? <Check size={14} /> : "1"}
            </span>
            添加上游并完成鉴权
          </div>
          <div
            className={cn(
              "flex items-center gap-2 border-l px-3 py-2.5 text-sm",
              verifiedUpstream ? "bg-muted/50 font-medium" : "text-muted-foreground",
            )}
          >
            <span className="flex size-6 items-center justify-center rounded-full border">2</span>
            选择分组并添加账号
          </div>
        </div>
      ) : null}

      {!entryHost ? (
        <Card>
          <CardHeader>
            <CardTitle>第一步：添加上游并完成鉴权</CardTitle>
          </CardHeader>
          <CardContent>
            {verifiedUpstream ? (
              <div className="flex flex-wrap items-center justify-between gap-3 rounded-lg border px-3 py-3">
                <div className="min-w-0">
                  <div className="flex flex-wrap items-center gap-2">
                    <strong>{verifiedUpstream.name}</strong>
                    <StatusPill label="已鉴权" tone="success" />
                    <Badge variant="outline">
                      {displayUpstreamType(verifiedUpstream.upstream_type)}
                    </Badge>
                    <Badge variant="outline">Host {verifiedUpstream.host}</Badge>
                  </div>
                  <ExternalUpstreamLink
                    host={verifiedUpstream.host}
                    baseUrl={verifiedUpstream.base_url}
                    label={verifiedUpstream.base_url}
                    className="mt-1 max-w-full text-xs"
                  />
                </div>
                <Button
                  variant="outline"
                  onClick={() => {
                    setVerifiedUpstream(null);
                    setSelectedGroupId(null);
                    prepare.reset();
                  }}
                >
                  重新选择
                </Button>
              </div>
            ) : (
              <form
                className="grid gap-4"
                onSubmit={(event) => {
                  event.preventDefault();
                  void completeUpstreamStep();
                }}
              >
                <div className="grid gap-4 sm:grid-cols-2">
                  <FormField
                    label="上游 Host（账号归属）"
                    error={form.formState.errors.host?.message}
                  >
                    <Input {...form.register("host")} placeholder="origin.example.com:8080" />
                  </FormField>
                  <FormField
                    label="请求 Base URL（实际访问）"
                    error={form.formState.errors.base_url?.message}
                  >
                    <div className="grid min-w-0 grid-cols-[6.75rem_minmax(0,1fr)] gap-2 sm:grid-cols-[6.75rem_minmax(0,1fr)_6.5rem]">
                      <Controller
                        control={form.control}
                        name="base_url_protocol"
                        render={({ field }) => (
                          <Select value={field.value} onValueChange={field.onChange}>
                            <SelectTrigger className="w-[6.75rem] shrink-0">
                              <SelectValue />
                            </SelectTrigger>
                            <SelectContent>
                              <SelectItem value="https">HTTPS</SelectItem>
                              <SelectItem value="http">HTTP</SelectItem>
                            </SelectContent>
                          </Select>
                        )}
                      />
                      <Input
                        {...form.register("base_url")}
                        placeholder="accelerated.example.com:8443/api"
                      />
                      <Button
                        type="button"
                        variant="outline"
                        className="col-span-2 sm:col-span-1"
                        disabled={detection.isPending}
                        onClick={() => void runUpstreamDetection("type")}
                      >
                        <ScanSearch />
                        {detectionAction === "type" ? "识别中" : "自动识别"}
                      </Button>
                    </div>
                  </FormField>
                  <FormField label="名称">
                    <div className="flex min-w-0 gap-2">
                      <Input
                        value={upstreamName}
                        onChange={(event) => setUpstreamName(event.target.value)}
                        placeholder="留空时使用 Host"
                      />
                      <Button
                        type="button"
                        variant="outline"
                        className="min-w-[6.5rem]"
                        disabled={detection.isPending}
                        onClick={() => void runUpstreamDetection("name")}
                      >
                        <RefreshCw />
                        {detectionAction === "name" ? "获取中" : "自动获取"}
                      </Button>
                    </div>
                  </FormField>
                  <FormField label="上游类型">
                    <Controller
                      control={form.control}
                      name="upstream_type"
                      render={({ field }) => (
                        <Select value={field.value} onValueChange={field.onChange}>
                          <SelectTrigger>
                            <SelectValue />
                          </SelectTrigger>
                          <SelectContent>
                            <SelectItem value="sub2api">Sub2API</SelectItem>
                            <SelectItem value="newapi">New API</SelectItem>
                            <SelectItem value="oneapi">OneAPI</SelectItem>
                            <SelectItem value="custom">自定义上游</SelectItem>
                          </SelectContent>
                        </Select>
                      )}
                    />
                  </FormField>
                  <FormField label="鉴权方式">
                    <Select value={authMode} onValueChange={(value) => value && setAuthMode(value)}>
                      <SelectTrigger>
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        {authModes.map((item) => (
                          <SelectItem key={item.value} value={item.value}>
                            {item.label}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </FormField>
                  <FormField label="倍率">
                    <Input
                      value={rechargeRate}
                      onChange={(event) => setRechargeRate(event.target.value)}
                      inputMode="decimal"
                      placeholder="1"
                    />
                  </FormField>
                </div>
                {usesAdminKey ? (
                  <div className="grid gap-4 sm:grid-cols-2">
                    <FormField label="Admin Key">
                      <Input
                        type="password"
                        value={credentials.adminKey}
                        onChange={(event) =>
                          setCredentials((current) => ({
                            ...current,
                            adminKey: event.target.value,
                          }))
                        }
                      />
                    </FormField>
                    <FormField label="用户 ID">
                      <Input
                        value={credentials.userId}
                        onChange={(event) =>
                          setCredentials((current) => ({
                            ...current,
                            userId: event.target.value,
                          }))
                        }
                      />
                    </FormField>
                  </div>
                ) : usesSub2ApiToken || usesToken ? (
                  <div className="grid gap-4 sm:grid-cols-2">
                    <FormField label="Token">
                      <Input
                        type="password"
                        value={credentials.accessToken}
                        onChange={(event) =>
                          setCredentials((current) => ({
                            ...current,
                            accessToken: event.target.value,
                          }))
                        }
                      />
                    </FormField>
                    {usesSub2ApiToken ? (
                      <FormField label="刷新 Token">
                        <Input
                          type="password"
                          value={credentials.refreshToken}
                          onChange={(event) =>
                            setCredentials((current) => ({
                              ...current,
                              refreshToken: event.target.value,
                            }))
                          }
                        />
                      </FormField>
                    ) : null}
                  </div>
                ) : usesVaultLogin ? (
                  <FormField label="密码箱密码项">
                    <Select
                      value={credentials.entry}
                      onValueChange={(value) => {
                        if (!value) return;
                        setCredentials((current) => ({
                          ...current,
                          entry: value,
                        }));
                      }}
                    >
                      <SelectTrigger>
                        <SelectValue placeholder="选择密码箱项" />
                      </SelectTrigger>
                      <SelectContent className="min-w-[20rem]">
                        {vaultOptions.map((item) => (
                          <SelectItem key={item.entry} value={item.entry}>
                            {vaultEntryLabel(item)}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </FormField>
                ) : usesManualLogin ? (
                  <div className="grid gap-4 sm:grid-cols-2">
                    <FormField label="用户名">
                      <Input
                        autoComplete="username"
                        value={credentials.username}
                        onChange={(event) =>
                          setCredentials((current) => ({
                            ...current,
                            username: event.target.value,
                          }))
                        }
                      />
                    </FormField>
                    <FormField label="密码">
                      <Input
                        type="password"
                        autoComplete="current-password"
                        value={credentials.password}
                        onChange={(event) =>
                          setCredentials((current) => ({
                            ...current,
                            password: event.target.value,
                          }))
                        }
                      />
                    </FormField>
                    <div className="flex items-center gap-2 sm:col-span-2">
                      <Switch
                        id="onboarding-save-to-vault"
                        checked={credentials.saveToVault}
                        onCheckedChange={(checked) =>
                          setCredentials((current) => ({
                            ...current,
                            saveToVault: checked,
                          }))
                        }
                      />
                      <label className="cursor-pointer text-sm" htmlFor="onboarding-save-to-vault">
                        登录成功后保存到密码箱
                      </label>
                    </div>
                    {credentials.saveToVault ? (
                      <FormField label="凭据名称（可选）">
                        <Input
                          value={credentials.entry}
                          onChange={(event) =>
                            setCredentials((current) => ({
                              ...current,
                              entry: event.target.value,
                            }))
                          }
                          placeholder="默认使用 Host"
                        />
                      </FormField>
                    ) : null}
                  </div>
                ) : authMode === "custom_headers" ? (
                  <FormField label="Headers JSON">
                    <Textarea
                      className="min-h-24"
                      value={credentials.headers}
                      onChange={(event) =>
                        setCredentials((current) => ({
                          ...current,
                          headers: event.target.value,
                        }))
                      }
                      placeholder='例如 {"Authorization":"Bearer ..."}'
                    />
                  </FormField>
                ) : authMode === "cookie" ? (
                  <FormField label="Cookies JSON">
                    <Textarea
                      className="min-h-24"
                      value={credentials.cookies}
                      onChange={(event) =>
                        setCredentials((current) => ({
                          ...current,
                          cookies: event.target.value,
                        }))
                      }
                      placeholder='例如 {"session":"..."}'
                    />
                  </FormField>
                ) : null}
                {authMode !== "custom_headers" ? (
                  <div className="grid gap-3 border-t pt-4">
                    <div className="flex items-center justify-between gap-3">
                      <label
                        className="cursor-pointer text-sm font-medium"
                        htmlFor="onboarding-custom-headers"
                      >
                        自定义请求头
                      </label>
                      <Switch
                        id="onboarding-custom-headers"
                        checked={showCustomHeaders}
                        onCheckedChange={setShowCustomHeaders}
                        aria-label="添加自定义请求头"
                      />
                    </div>
                    {showCustomHeaders ? (
                      <Textarea
                        className="min-h-24"
                        value={credentials.headers}
                        onChange={(event) =>
                          setCredentials((current) => ({
                            ...current,
                            headers: event.target.value,
                          }))
                        }
                        placeholder='例如 {"X-Custom-Header":"value"}'
                      />
                    ) : null}
                  </div>
                ) : null}
                {authConfig.error && usesVaultLogin ? (
                  <QueryError error={authConfig.error} fallback="密码箱读取失败" embedded />
                ) : null}
                <div className="flex justify-end">
                  <Button
                    type="submit"
                    disabled={
                      createUpstream.isPending || upstreams.isLoading || detection.isPending
                    }
                  >
                    <ShieldCheck size={16} />
                    {createUpstream.isPending ? "正在验证" : "添加并验证上游"}
                  </Button>
                </div>
              </form>
            )}
          </CardContent>
        </Card>
      ) : null}

      {verifiedUpstream || entryHost ? (
        <Card className={selectionCardClass}>
          <CardHeader>
            <div className="flex flex-wrap items-center justify-between gap-3">
              <CardTitle>{onboardingSelectionTitle(entryKind)}</CardTitle>
              {verifiedUpstream ? (
                <div className="flex flex-wrap items-center justify-end gap-2">
                  {!preparedData ? (
                    <Button
                      onClick={() => prepare.mutate(verifiedUpstream.host)}
                      disabled={preparing}
                    >
                      <RefreshCw className={preparing ? "animate-spin" : undefined} size={16} />
                      {preparing ? "正在获取" : "获取上游信息"}
                    </Button>
                  ) : null}
                  <Button
                    variant="outline"
                    disabled={balanceSyncPending || preparing}
                    onClick={() => {
                      setBalanceTaskId(null);
                      balanceSync.reset();
                      setBalanceDialogOpen(true);
                      balanceSync.mutate(verifiedUpstream.host);
                    }}
                  >
                    <WalletCards />
                    {balanceSyncPending ? "同步中" : "同步余额"}
                  </Button>
                  <OnboardingMaintenanceActions
                    accountCount={boundAccountIDs.length}
                    pending={onboardingMaintenancePending || preparing}
                    onRevalidate={() => startOnboardingMaintenance("revalidate")}
                    onRepairNames={() => startOnboardingMaintenance("repair")}
                  />
                </div>
              ) : null}
            </div>
          </CardHeader>
          <CardContent
            className={
              entryKind === "host" ? "grid min-h-0 flex-1 gap-4 overflow-hidden" : "grid gap-4"
            }
          >
            {(entryConfiguration.isLoading && !verifiedUpstream) || (preparing && !preparedData) ? (
              <OnboardingSelectionSkeleton
                fillAvailableHeight={entryKind === "host"}
                groupLocked={entryGroupId !== null}
              />
            ) : null}
            {entryConfiguration.error && !verifiedUpstream ? (
              <QueryError error={entryConfiguration.error} fallback="已选上游读取失败" embedded />
            ) : null}
            {verifiedUpstream && !preparedData && !preparedError && !preparing ? (
              <div className="text-muted-foreground rounded-lg border border-dashed px-4 py-8 text-center text-sm">
                获取最新余额、Key 和分组后才能选择并添加账号。
              </div>
            ) : null}
            {preparedData ? (
              <div
                className={
                  entryKind === "host"
                    ? "grid min-h-0 grid-rows-[auto_minmax(0,1fr)_auto] gap-4 overflow-hidden"
                    : "grid gap-4"
                }
              >
                <div className="grid grid-cols-2 divide-x rounded-lg border lg:grid-cols-6">
                  <ResultSummaryRow
                    label="上游"
                    value={preparedData.upstream.name}
                    href={preparedData.upstream.base_url}
                  />
                  <ResultSummaryRow
                    label="类型"
                    value={displayUpstreamType(preparedData.upstream.upstream_type)}
                  />
                  <ResultSummaryRow
                    label="余额"
                    value={preparedData.upstream.balance ?? "未返回"}
                  />
                  <ResultSummaryRow
                    label="充值比例"
                    value={rechargeRatioLabel(preparedData.upstream.recharge_rate)}
                  />
                  <ResultSummaryRow label="可选分组" value={`${candidateStats.selectable} 个`} />
                  <ResultSummaryRow label="已绑定分组" value={`${candidateStats.bound} 个`} />
                </div>
                {entryGroupId && entryCandidate ? (
                  <div className="grid gap-3">
                    <div className="grid grid-cols-2 divide-x divide-y rounded-lg border lg:grid-cols-5 lg:divide-y-0">
                      <ResultSummaryRow label="上游分组" value={entryCandidate.group_name} />
                      <ResultSummaryRow
                        label="上游倍率"
                        value={entryCandidate.multiplier ?? "未计算"}
                      />
                      <ResultSummaryRow
                        label="本地分组倍率"
                        value={localGroupMultiplierLabel(entrySelectedLocalGroups)}
                      />
                      <ResultSummaryRow
                        label="本地分组"
                        value={candidateBoundLocalGroups(entryCandidate).join("、") || "待选择"}
                      />
                      <ResultSummaryRow
                        label="状态"
                        value={candidateHasExistingBinding(entryCandidate) ? "已绑定" : "未绑定"}
                      />
                    </div>
                    <div className="flex min-w-0 items-center justify-between gap-3 rounded-lg border px-3 py-2.5">
                      {candidateHasExistingBinding(entryCandidate) ? (
                        <>
                          <span className="text-muted-foreground shrink-0 text-sm">
                            当前绑定账号
                          </span>
                          <div className="ml-auto min-w-0 max-w-md flex-1">
                            <UpstreamBoundAccountSelect accounts={entryCandidate.bound_accounts} />
                          </div>
                        </>
                      ) : (
                        <span className="text-muted-foreground text-sm">该上游分组尚未绑定</span>
                      )}
                      <OnboardingProbeAction
                        target={entryProbeTarget}
                        groupName={entryCandidate.group_name}
                        pending={false}
                        onProbe={() => setProbeTarget(entryProbeTarget)}
                      />
                    </div>
                    {!entryCandidateSelectable && !candidateHasExistingBinding(entryCandidate) ? (
                      <QueryError
                        error={candidateCreationUnavailableReason(entryCandidate)}
                        fallback="指定的上游分组不可用"
                        embedded
                      />
                    ) : null}
                  </div>
                ) : null}
                {entryGroupId && !entryCandidate ? (
                  <QueryError
                    error="指定的上游分组不存在或当前不可用于添加账号"
                    fallback="指定的上游分组不可用"
                    embedded
                  />
                ) : null}
                {!entryGroupId ? (
                  <Table
                    className="min-w-[1180px] table-fixed"
                    containerClassName={
                      entryKind === "host"
                        ? "min-h-0 overflow-auto rounded-lg border"
                        : "max-h-[32rem] overflow-auto rounded-lg border"
                    }
                  >
                    <TableHeader>
                      <TableRow>
                        <TableHead className="w-[14%]">上游分组</TableHead>
                        <TableHead className="w-[25%]">介绍</TableHead>
                        <TableHead className="w-[8%]">上游倍率</TableHead>
                        <TableHead className="w-[10%]">本地分组倍率</TableHead>
                        <TableHead className="w-[26%]">本地分组</TableHead>
                        <TableHead className="w-[9%]">状态</TableHead>
                        <TableHead className="w-[8%] text-right">操作</TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {!visibleCandidates.length ? (
                        <TableMessageRow columns={7}>
                          <EmptyRow text="上游没有返回分组" />
                        </TableMessageRow>
                      ) : null}
                      {visibleCandidates.map((candidate) => {
                        const alreadyBound = candidateHasExistingBinding(candidate);
                        const canSelect = candidateCanCreateKey(candidate);
                        const canEdit = canSelect || alreadyBound;
                        const unavailableReason = canEdit
                          ? null
                          : candidateCreationUnavailableReason(candidate);
                        const selectedLocalGroupIDs = candidate.group_id
                          ? (batchBindings[candidate.group_id] ??
                            candidateBoundLocalGroupIDs(candidate))
                          : [];
                        const selectedLocalGroupRecords = selectedLocalGroupIDs.flatMap(
                          (groupID) => {
                            const group = localGroups.find((item) => item.id === groupID);
                            return group ? [group] : [];
                          },
                        );
                        const pendingChange = alreadyBound
                          ? !sameOnboardingGroupSelection(
                              selectedLocalGroupIDs,
                              candidateBoundLocalGroupIDs(candidate),
                            )
                          : selectedLocalGroupIDs.length > 0;
                        const candidateProbeTarget = onboardingProbeTarget(candidate);
                        return (
                          <TableRow
                            key={`${candidate.host}:${candidate.group_id}`}
                            data-state={pendingChange ? "selected" : undefined}
                          >
                            <TableCell className="font-medium">{candidate.group_name}</TableCell>
                            <TableCell tooltipContent={candidate.description ?? "未提供说明"}>
                              {candidate.description ?? "未提供说明"}
                            </TableCell>
                            <TableCell>{candidate.multiplier ?? "未计算"}</TableCell>
                            <TableCell className="tabular-nums">
                              {localGroupMultiplierLabel(selectedLocalGroupRecords)}
                            </TableCell>
                            <TableCell overflowTooltip={false}>
                              <OnboardingGroupBindingSelect
                                upstreamGroupName={candidate.group_name}
                                groups={localGroups}
                                value={selectedLocalGroupIDs}
                                disabled={!canEdit || onboardingPending}
                                disabledReason={unavailableReason}
                                onValueChange={(value) => {
                                  if (!candidate.group_id) return;
                                  setBatchBindings((current) => {
                                    return { ...current, [candidate.group_id!]: value };
                                  });
                                }}
                              />
                            </TableCell>
                            <TableCell>
                              <StatusPill
                                label={alreadyBound ? "已绑定" : "未绑定"}
                                tone={alreadyBound ? "success" : "neutral"}
                              />
                            </TableCell>
                            <TableCell className="text-right" overflowTooltip={false}>
                              <OnboardingProbeAction
                                target={candidateProbeTarget}
                                groupName={candidate.group_name}
                                pending={false}
                                onProbe={() => setProbeTarget(candidateProbeTarget)}
                              />
                            </TableCell>
                          </TableRow>
                        );
                      })}
                    </TableBody>
                  </Table>
                ) : null}
                {entryGroupId && entryCandidateSelectable ? (
                  <>
                    <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-5">
                      <FormField
                        label="本地分组"
                        error={form.formState.errors.local_group_id?.message}
                      >
                        <OnboardingGroupBindingSelect
                          upstreamGroupName={entryCandidate?.group_name ?? "当前上游分组"}
                          groups={localGroups}
                          value={
                            entryGroupId
                              ? (batchBindings[entryGroupId] ??
                                candidateBoundLocalGroupIDs(entryCandidate ?? {}))
                              : []
                          }
                          disabled={!selectedGroupId || onboardingPending}
                          disabledReason={null}
                          onValueChange={(value) => {
                            if (!entryGroupId) return;
                            setBatchBindings((current) => ({ ...current, [entryGroupId]: value }));
                            form.setValue("local_group_id", value[0] ?? "", {
                              shouldValidate: true,
                            });
                            form.setValue(
                              "target_group",
                              value
                                .flatMap((id) => {
                                  const group = localGroups.find((item) => item.id === id);
                                  return group ? [group.name] : [];
                                })
                                .join("、"),
                            );
                          }}
                        />
                      </FormField>
                      <FormField label="账号倍率" error={form.formState.errors.multiplier?.message}>
                        <Input
                          {...form.register("multiplier")}
                          disabled={!selectedGroupId}
                          inputMode="decimal"
                        />
                      </FormField>
                      <FormField label="并发" error={form.formState.errors.concurrency?.message}>
                        <Input
                          {...form.register("concurrency")}
                          disabled={!selectedGroupId}
                          type="number"
                          min={1}
                          max={10_000_000}
                          inputMode="numeric"
                        />
                      </FormField>
                      <FormField label="优先级" error={form.formState.errors.priority?.message}>
                        <Input
                          {...form.register("priority")}
                          disabled={!selectedGroupId}
                          type="number"
                          min={1}
                          max={10_000_000}
                          inputMode="numeric"
                        />
                      </FormField>
                      <FormField label="备注">
                        <Input
                          {...form.register("notes")}
                          disabled={!selectedGroupId}
                          placeholder="添加在规范记录之后"
                        />
                      </FormField>
                    </div>
                    <div className="flex justify-end">
                      <Button
                        type="button"
                        disabled={
                          !canSubmitOnboarding(
                            onboardingPending,
                            selectedGroupId,
                            entryGroupId
                              ? ((batchBindings[entryGroupId] ??
                                  candidateBoundLocalGroupIDs(entryCandidate ?? {}))[0] ?? "")
                              : "",
                          )
                        }
                        onClick={() => void execute()}
                      >
                        <Eye size={16} />
                        {entrySubmitLabel}
                      </Button>
                    </div>
                  </>
                ) : null}
                {!entryGroupId ? (
                  <div className="flex flex-wrap items-end justify-between gap-3">
                    <div className="grid min-w-64 flex-1 gap-3 sm:grid-cols-3">
                      <FormField label="批量备注（可选）">
                        <Input
                          {...form.register("notes")}
                          disabled={onboardingPending}
                          placeholder="应用到本批次的所有账号"
                        />
                      </FormField>
                      <FormField label="并发" error={form.formState.errors.concurrency?.message}>
                        <Input
                          {...form.register("concurrency")}
                          disabled={onboardingPending}
                          type="number"
                          min={1}
                          max={10_000_000}
                          inputMode="numeric"
                        />
                      </FormField>
                      <FormField label="优先级" error={form.formState.errors.priority?.message}>
                        <Input
                          {...form.register("priority")}
                          disabled={onboardingPending}
                          type="number"
                          min={1}
                          max={10_000_000}
                          inputMode="numeric"
                        />
                      </FormField>
                    </div>
                    <div className="flex items-center gap-3">
                      <span className="text-muted-foreground text-sm tabular-nums">
                        已选择 {batchBindingCount} 个
                      </span>
                      <Button
                        type="button"
                        disabled={
                          onboardingPending || batchBindingCount === 0 || batchBindingCount > 50
                        }
                        onClick={() => void executeBatch()}
                      >
                        <Eye size={16} />
                        {onboardingPending ? "正在提交" : `预览提交 ${batchBindingCount} 项变更`}
                      </Button>
                    </div>
                  </div>
                ) : null}
              </div>
            ) : null}
          </CardContent>
        </Card>
      ) : null}

      {probeTarget ? (
        <AccountProbeDialog
          target={probeTarget}
          open
          onOpenChange={(open) => {
            if (!open) setProbeTarget(null);
          }}
        />
      ) : null}

      <OnboardingConfirmDialog
        open={onboardingConfirmation !== null}
        items={onboardingConfirmation?.previews ?? []}
        pending={onboardingSubmitting}
        onOpenChange={(open) => {
          if (!open) setOnboardingConfirmation(null);
        }}
        onConfirm={() => void confirmOnboarding()}
      />

      <Dialog
        open={balanceDialogOpen}
        onOpenChange={(open) => {
          if (!open && !balanceSyncPending) {
            setBalanceDialogOpen(false);
            setBalanceTaskId(null);
            balanceSync.reset();
          }
        }}
      >
        <DialogContent width="medium">
          <DialogHeader>
            <DialogTitle>同步余额</DialogTitle>
          </DialogHeader>
          <div className="grid gap-4">
            {!balanceTask.data && !balanceTask.error && !balanceSync.error ? (
              <TaskStartupState message="正在创建余额同步任务" />
            ) : null}
            {balanceSync.error ? (
              <QueryError error={balanceSync.error} fallback="余额同步启动失败" embedded />
            ) : null}
            {balanceTask.error ? (
              <QueryError error={balanceTask.error} fallback="余额同步状态读取失败" embedded />
            ) : null}
            {balanceTask.data ? <BalanceTaskProgress task={balanceTask.data} /> : null}
            {!balanceSyncPending && (balanceSync.error || balanceTask.error || balanceTask.data) ? (
              <div className="flex justify-end">
                <Button
                  variant="outline"
                  onClick={() => {
                    setBalanceDialogOpen(false);
                    setBalanceTaskId(null);
                    balanceSync.reset();
                  }}
                >
                  关闭
                </Button>
              </div>
            ) : null}
          </div>
        </DialogContent>
      </Dialog>

      <Dialog
        open={onboardingMaintenanceKind !== null}
        onOpenChange={(open) => {
          if (!open && !onboardingMaintenancePending) {
            setOnboardingMaintenanceKind(null);
            setOnboardingMaintenanceTaskId(null);
            setMissingBindingTargets([]);
            onboardingMaintenance.reset();
          }
        }}
      >
        <DialogContent
          width={operationDialogWidth(taskStopsPolling(onboardingMaintenanceTask.data))}
          height={operationDialogHeight(taskStopsPolling(onboardingMaintenanceTask.data), "large")}
          className="grid grid-rows-[auto_minmax(0,1fr)] overflow-hidden"
        >
          <DialogHeader>
            <DialogTitle>
              {onboardingMaintenanceKind === "revalidate"
                ? "复验已有绑定"
                : onboardingMaintenanceKind === "cleanup"
                  ? "修复失效绑定"
                  : "修复绑定账号名称"}
            </DialogTitle>
            <DialogDescription>
              {onboardingMaintenanceKind === "cleanup"
                ? `将处理 ${missingBindingTargets.length} 个已确认不存在的账号。`
                : `当前页面显示 ${boundAccountIDs.length} 个已绑定账号，不需要勾选账号。`}
            </DialogDescription>
          </DialogHeader>
          <DialogBody className={cn(onboardingMaintenanceTask.data && "overflow-hidden pr-0")}>
            {onboardingMaintenanceKind === "repair" &&
              !onboardingMaintenanceTaskId &&
              !onboardingMaintenance.isPending &&
              !onboardingMaintenance.error && (
                <div className="grid gap-4">
                  <div className="border-warning/40 bg-warning/10 rounded-lg border px-4 py-3 text-sm leading-6">
                    将按当前上游名称和账号倍率修复管理平台账号名称。名称正确的账号不会产生写请求。
                  </div>
                  <div className="max-h-56 min-w-0 divide-y overflow-x-hidden overflow-y-auto rounded-lg border">
                    {displayedBoundAccounts.map((account) => (
                      <div className="min-w-0 px-3 py-2.5 text-sm" key={account.account_id}>
                        <strong className="block truncate">
                          {account.account_name || `账号 ${account.account_id}`}
                        </strong>
                        <div className="text-muted-foreground flex min-w-0 items-center gap-1 text-xs">
                          <span className="shrink-0">ID {account.account_id} ·</span>
                          <ExternalUpstreamLink
                            host={verifiedUpstream?.host ?? ""}
                            baseUrl={verifiedUpstream?.base_url}
                          />
                        </div>
                      </div>
                    ))}
                  </div>
                  <div className="flex flex-wrap justify-end gap-2 border-t pt-4">
                    <Button variant="outline" onClick={() => setOnboardingMaintenanceKind(null)}>
                      取消
                    </Button>
                    <Button
                      onClick={() =>
                        onboardingMaintenance.mutate({
                          kind: "repair",
                          accountIds: boundAccountIDs,
                        })
                      }
                    >
                      <SpellCheck2 size={16} />
                      确认修复 {boundAccountIDs.length} 个账号
                    </Button>
                  </div>
                </div>
              )}
            {onboardingMaintenanceKind === "cleanup" &&
              !onboardingMaintenanceTaskId &&
              !onboardingMaintenance.isPending &&
              !onboardingMaintenance.error && (
                <div className="grid gap-4">
                  <div className="border-destructive/40 bg-destructive/10 rounded-lg border px-4 py-3 text-sm leading-6">
                    管理平台中已经没有这些稳定账号
                    ID，无法通过改名恢复。修复将再次复验，只清理仍然不存在的本地账号和绑定；清理后对应分组可以重新添加账号。
                  </div>
                  <div className="max-h-56 min-w-0 divide-y overflow-x-hidden overflow-y-auto rounded-lg border">
                    {missingBindingTargets.map((item) => (
                      <div className="min-w-0 px-3 py-2.5 text-sm" key={item.accountId}>
                        <strong className="block truncate">
                          {item.accountName || `账号 ${item.accountId}`}
                        </strong>
                        <div className="text-muted-foreground flex min-w-0 items-center gap-1 text-xs">
                          <span className="shrink-0">ID {item.accountId} ·</span>
                          <ExternalUpstreamLink host={item.upstreamHost} />
                        </div>
                      </div>
                    ))}
                  </div>
                  <div className="flex flex-wrap justify-end gap-2 border-t pt-4">
                    <Button variant="outline" onClick={() => setOnboardingMaintenanceKind(null)}>
                      取消
                    </Button>
                    <Button
                      variant="destructive"
                      onClick={() =>
                        onboardingMaintenance.mutate({
                          kind: "cleanup",
                          accountIds: missingBindingTargets.map((item) => item.accountId),
                        })
                      }
                    >
                      <Trash2 size={16} />
                      确认清理 {missingBindingTargets.length} 个失效绑定
                    </Button>
                  </div>
                </div>
              )}
            {onboardingMaintenance.isPending && (
              <TaskStartupState message="正在创建绑定账号维护任务" />
            )}
            {onboardingMaintenance.error && (
              <QueryError
                error={onboardingMaintenance.error}
                fallback="绑定账号维护任务启动失败"
                embedded
              />
            )}
            {onboardingMaintenanceTask.error && (
              <QueryError
                error={onboardingMaintenanceTask.error}
                fallback="绑定账号维护任务状态读取失败"
                embedded
              />
            )}
            {onboardingMaintenanceTask.data && (
              <AccountMaintenanceTaskStatus
                task={onboardingMaintenanceTask.data}
                onCleanupMissing={(items) => {
                  setMissingBindingTargets(items);
                  setOnboardingMaintenanceTaskId(null);
                  onboardingMaintenance.reset();
                  setOnboardingMaintenanceKind("cleanup");
                }}
              />
            )}
          </DialogBody>
        </DialogContent>
      </Dialog>

      <Dialog
        open={taskId !== null}
        onOpenChange={(open) => {
          if (!open && !onboardingPending) {
            setTaskId(null);
            setBatchBindings({});
            const host = verifiedUpstream?.host;
            if (host) prepare.mutate(host);
          }
        }}
      >
        <DialogContent
          width={operationDialogWidth(Boolean(task.data && !onboardingPending))}
          height={operationDialogHeight(Boolean(task.data && !onboardingPending), "medium")}
          className="grid grid-rows-[auto_minmax(0,1fr)_auto] overflow-hidden"
        >
          <DialogHeader>
            <DialogTitle>账号绑定变更</DialogTitle>
          </DialogHeader>
          <DialogBody className={cn(task.data && "overflow-hidden pr-0")}>
            {!task.data && !task.error && <TaskStartupState message="正在创建账号绑定变更任务" />}
            {task.error && (
              <QueryError error={task.error} fallback="账号绑定变更状态读取失败" embedded />
            )}
            {task.data && <OnboardingTaskProgress task={task.data} />}
          </DialogBody>
          {task.data && !onboardingPending ? (
            <DialogFooter>
              <Button
                onClick={() => {
                  setTaskId(null);
                  setBatchBindings({});
                  const host = verifiedUpstream?.host;
                  if (host) prepare.mutate(host);
                }}
              >
                <Check size={16} />
                完成
              </Button>
            </DialogFooter>
          ) : null}
        </DialogContent>
      </Dialog>
    </PageLayout>
  );
}

function RequestTracePage() {
  return (
    <PageLayout>
      <PageHeading
        eyebrow="OBSERVABILITY / REQUEST TRACE"
        title="请求追踪"
        description="查询 Sub2API 系统日志，直接查看请求状态、耗时、账号和模型等关键信息。"
      />
      <SystemLogSearchPanel />
    </PageLayout>
  );
}

function TaskStatus(props: { task: Task; onClose: () => void }) {
  return (
    <Card className={cn("relative mb-4", props.task.status === "failed" && "border-destructive")}>
      <CardContent className="pt-4">
        <TaskProgress task={props.task} onClose={props.onClose} />
      </CardContent>
    </Card>
  );
}
function TaskFailureDetail(props: { reason: string }) {
  return (
    <div className="text-destructive flex min-w-0 items-start gap-2 text-sm leading-5">
      <CircleHelp size={16} className="mt-0.5 shrink-0" />
      <span className="min-w-0 break-words">{props.reason}</span>
    </div>
  );
}

type OnboardingTaskItem = {
  action: string;
  upstreamGroup: string;
  localGroup: string;
  succeeded: boolean;
  error: string | null;
};

function onboardingTaskItems(task: Task): OnboardingTaskItem[] {
  if (!Array.isArray(task.result.items)) return [];
  return task.result.items.flatMap((raw) => {
    if (typeof raw !== "object" || raw === null || Array.isArray(raw)) return [];
    const item = raw as Record<string, unknown>;
    const status = String(item.status ?? "")
      .trim()
      .toLowerCase();
    return [
      {
        action: String(item.action ?? "账号绑定变更"),
        upstreamGroup: String(item.upstream_group ?? "未返回"),
        localGroup: String(item.local_group ?? "未返回"),
        succeeded: status === "成功" || status === "succeeded" || status === "success",
        error:
          typeof item.error === "string" && item.error.trim() !== "" ? item.error.trim() : null,
      },
    ];
  });
}

export function OnboardingTaskProgress(props: { task: Task }) {
  const pending = ["queued", "running", "waiting_input"].includes(props.task.status);
  const failed = props.task.status === "failed";
  const items = onboardingTaskItems(props.task);
  const pagination = usePaginatedItems(items);
  const batch = props.task.operation === "onboard-batch" || items.length > 0;
  const succeeded = syncResultCount(props.task.result.succeeded);
  const failedCount = syncResultCount(props.task.result.failed);
  const total = Math.max(
    syncResultCount(props.task.result.total),
    succeeded + failedCount,
    items.length,
  );
  let title = batch ? "账号批量绑定变更完成" : "账号绑定变更完成";
  if (pending) title = batch ? "正在批量处理账号绑定" : "正在处理账号绑定";
  if (failed) title = batch && succeeded > 0 ? "部分账号绑定变更失败" : "账号绑定变更失败";
  const bindingUpdate = props.task.result.operation === "account.groups";

  return (
    <div className={cn("flex h-full min-h-0 flex-col gap-4", pending && "justify-center")}>
      <div className="flex min-w-0 items-start gap-3">
        <span
          className={cn(
            "flex size-8 shrink-0 items-center justify-center rounded-md",
            pending
              ? "bg-primary/10 text-primary"
              : failed
                ? "bg-destructive/10 text-destructive"
                : "bg-success/10 text-success",
          )}
        >
          {pending ? (
            <RefreshCw className="animate-spin" size={16} />
          ) : failed ? (
            <CircleAlert size={16} />
          ) : (
            <Check size={16} />
          )}
        </span>
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2">
            <strong className="text-sm">{title}</strong>
            <StatusPill
              label={pending ? "进行中" : failed ? (succeeded > 0 ? "部分成功" : "失败") : "成功"}
              tone={pending ? "info" : failed ? (succeeded > 0 ? "warning" : "danger") : "success"}
            />
          </div>
          <p className="text-muted-foreground mt-1 text-xs leading-5">
            {displayTaskMessage(props.task.message)}
          </p>
        </div>
        {pending ? (
          <span className="text-muted-foreground shrink-0 text-xs tabular-nums">
            {props.task.progress}%
          </span>
        ) : null}
      </div>

      {pending ? <Progress value={props.task.progress} /> : null}

      {!pending && batch ? (
        <>
          <div className="grid grid-cols-3 divide-x rounded-lg border">
            <ResultSummaryRow label="计划变更" value={`${total} 项`} />
            <ResultSummaryRow label="处理成功" value={`${succeeded} 项`} />
            <ResultSummaryRow label="处理失败" value={`${failedCount} 项`} />
          </div>
          <div className="flex min-h-0 flex-1 flex-col overflow-hidden rounded-lg border">
            <Table className="min-w-[520px]" containerClassName="min-h-0 flex-1 overflow-auto">
              <TableHeader className="sticky top-0 z-10">
                <TableRow>
                  <TableHead className="w-[18%]">操作</TableHead>
                  <TableHead className="w-[28%]">上游分组</TableHead>
                  <TableHead className="w-[26%]">本地分组</TableHead>
                  <TableHead className="w-[28%]">处理结果</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {pagination.visibleItems.map((item, index) => (
                  <TableRow
                    key={`${item.upstreamGroup}:${item.localGroup}:${(pagination.currentPage - 1) * pagination.pageSize + index}`}
                  >
                    <TableCell>{item.action}</TableCell>
                    <TableCell className="font-medium">{item.upstreamGroup}</TableCell>
                    <TableCell>{item.localGroup}</TableCell>
                    <TableCell
                      className="whitespace-normal break-words leading-5"
                      overflowTooltip={false}
                    >
                      {item.succeeded ? (
                        <StatusPill label="处理成功" tone="success" />
                      ) : (
                        <div className="grid gap-1">
                          <StatusPill label="处理失败" tone="danger" />
                          <span className="text-destructive text-xs">
                            {item.error ?? "未返回失败原因"}
                          </span>
                        </div>
                      )}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
            {items.length > 0 ? (
              <DataTablePagination
                currentPage={pagination.currentPage}
                totalPages={pagination.totalPages}
                totalItems={items.length}
                pageSize={pagination.pageSize}
                pageSizes={[10, 20, 50, 100]}
                onPageChange={pagination.setCurrentPage}
                onPageSizeChange={pagination.setPageSize}
              />
            ) : null}
          </div>
        </>
      ) : null}

      {!pending && !batch && !failed ? (
        <div className="divide-y rounded-lg border">
          <ResultSummaryRow label="操作" value={bindingUpdate ? "更新已有绑定" : "添加账号"} />
          <ResultSummaryRow
            label="账号"
            value={String(props.task.result.account_name ?? (bindingUpdate ? "已更新" : "已添加"))}
          />
          <ResultSummaryRow
            label="账号 ID"
            value={
              bindingUpdate && Array.isArray(props.task.result.accounts)
                ? props.task.result.accounts
                    .map((item) => String((item as Record<string, unknown>).account_id ?? ""))
                    .filter(Boolean)
                    .join("、")
                : String(props.task.result.account_id ?? "未返回")
            }
          />
          <ResultSummaryRow
            label="上游分组"
            value={String(props.task.result.upstream_group_name ?? "未返回")}
          />
          <ResultSummaryRow
            label="本地分组"
            value={
              Array.isArray(props.task.result.local_group_names)
                ? props.task.result.local_group_names.join("、")
                : String(props.task.result.local_group_name ?? "未返回")
            }
          />
          {!bindingUpdate ? (
            <ResultSummaryRow
              label="调度状态"
              value={props.task.result.schedulable === true ? "已启用" : "已添加，暂未启用"}
            />
          ) : null}
        </div>
      ) : null}

      {!pending && !batch && failed ? (
        <div className="rounded-lg border border-destructive/30 bg-destructive/5 p-3">
          <TaskFailureDetail
            reason={String(props.task.result.error ?? props.task.message ?? "账号绑定变更失败")}
          />
        </div>
      ) : null}
    </div>
  );
}

export function AccountSyncTaskStatus(props: {
  task: Task;
  accountId: string;
  onClose: () => void;
}) {
  if (props.task.status !== "failed") {
    return <TaskStatus task={props.task} onClose={props.onClose} />;
  }
  const remoteWritten = props.task.result.remote_write === true;
  const reason = String(props.task.result.error ?? props.task.message ?? "账号同步失败");
  const operationName =
    props.task.operation === "account-groups-sync" ? "账号分组同步" : "账号字段同步";
  return (
    <Card className="relative mb-4 border-destructive">
      <CardContent className="grid gap-4 pt-4">
        <div className="flex items-center gap-3">
          <CircleAlert className="text-destructive" size={17} />
          <div className="min-w-0 flex-1">
            <strong className="block text-sm">{operationName}失败</strong>
            <span className="text-muted-foreground mt-1 block text-xs">
              账号 ID {props.accountId}
            </span>
          </div>
          <StatusPill label="失败" tone="danger" />
          <Button variant="ghost" size="icon-sm" aria-label="关闭任务结果" onClick={props.onClose}>
            <X size={15} />
          </Button>
        </div>
        <TaskFailureDetail reason={reason} />
        <div className="divide-y rounded-lg border">
          <ResultSummaryRow label="账号 ID" value={props.accountId} />
          <ResultSummaryRow
            label="远端写入"
            value={remoteWritten ? "已写入，后续读回或本地提交失败" : "未写入"}
          />
        </div>
      </CardContent>
    </Card>
  );
}
function TaskProgress(props: { task: Task; onClose?: () => void }) {
  return (
    <div>
      <div className="flex items-center gap-3">
        <Activity size={15} className="text-primary" />
        <div className="min-w-0 flex-1">
          <strong className="block text-sm">{displayTaskMessage(props.task.message)}</strong>
          <span className="text-muted-foreground mt-1 block text-xs">
            任务 {props.task.id} · {displayLabel(props.task.status)} · {props.task.progress}%
          </span>
        </div>
        {props.onClose && (
          <Button variant="ghost" size="icon-sm" aria-label="关闭任务结果" onClick={props.onClose}>
            <X size={15} />
          </Button>
        )}
      </div>
      <div className="bg-muted mt-3 h-1 overflow-hidden rounded-full">
        <span
          className="bg-primary block h-full transition-[width] duration-200"
          style={{ width: `${props.task.progress}%` }}
        />
      </div>
      {Object.keys(props.task.result).length > 0 && <TaskResult task={props.task} />}
    </div>
  );
}
function AuthTaskProgress(props: { task: Task }) {
  const pending = ["queued", "running"].includes(props.task.status);
  const succeeded = props.task.status === "succeeded";
  const outcome =
    typeof props.task.result.outcome === "object" &&
    props.task.result.outcome !== null &&
    !Array.isArray(props.task.result.outcome)
      ? (props.task.result.outcome as Record<string, unknown>)
      : null;
  const reasonValue = outcome?.reason ?? props.task.result.reason;
  const reason =
    reasonValue === undefined || reasonValue === null
      ? displayTaskMessage(props.task.message)
      : displayTaskMessage(String(reasonValue));
  const balanceResult =
    typeof props.task.result.balance_sync === "object" &&
    props.task.result.balance_sync !== null &&
    !Array.isArray(props.task.result.balance_sync)
      ? (props.task.result.balance_sync as Record<string, unknown>)
      : null;
  const balanceSucceeded = balanceResult?.status === "succeeded";
  let balanceText: string | null = null;
  if (balanceSucceeded && balanceResult !== null) {
    if (typeof balanceResult.balance === "string" || typeof balanceResult.balance === "number") {
      balanceText = `余额 ${formatBalance(String(balanceResult.balance))}`;
    } else {
      balanceText = "上游未返回余额";
    }
  } else if (balanceResult !== null) {
    balanceText = displayTaskMessage(String(balanceResult.reason ?? "余额读取失败"));
  }
  const statusText = pending ? "正在恢复鉴权" : succeeded ? "鉴权已恢复" : "鉴权未恢复";
  return (
    <div className="grid gap-3">
      <div className="flex items-center gap-3">
        {pending ? (
          <RefreshCw className="animate-spin text-primary" size={17} />
        ) : (
          <ShieldCheck className={succeeded ? "text-success" : "text-destructive"} size={17} />
        )}
        <div className="min-w-0 flex-1">
          <strong className="text-sm">{statusText}</strong>
          {!pending && (
            <p className="text-muted-foreground mt-1 text-sm break-words">
              {succeeded ? displayTaskMessage(props.task.message) : reason}
            </p>
          )}
          {!pending && succeeded && balanceText && (
            <p
              className={cn(
                "mt-1 text-xs break-words",
                balanceSucceeded ? "text-muted-foreground" : "text-warning",
              )}
            >
              {balanceText}
            </p>
          )}
        </div>
        <StatusPill
          label={pending ? "处理中" : succeeded ? "成功" : "失败"}
          tone={pending ? "info" : succeeded ? "success" : "danger"}
        />
      </div>
    </div>
  );
}
export function UpstreamDeleteTaskStatus(props: { task: Task }) {
  const pending = ["queued", "running"].includes(props.task.status);
  const succeeded = props.task.status === "succeeded";
  const deletedAccounts = props.task.result.deleted_accounts;
  const deletedGroups = props.task.result.deleted_groups;
  const reason = String(props.task.result.reason ?? props.task.message ?? "上游删除失败");
  return (
    <div className="flex h-full min-h-0 flex-col gap-4">
      <div className="flex items-center gap-3">
        {pending ? (
          <RefreshCw className="animate-spin text-primary" size={17} />
        ) : (
          <Trash2 className={succeeded ? "text-success" : "text-destructive"} size={17} />
        )}
        <div className="min-w-0 flex-1">
          <strong className="text-sm">
            {pending ? "正在删除上游及关联账号" : succeeded ? "删除完成" : "删除失败"}
          </strong>
          {!pending && !succeeded && (
            <div className="mt-1 grid gap-1">
              <p className="text-muted-foreground text-sm break-words">
                {displayTaskMessage(reason)}
              </p>
              {Number(props.task.result.remote_deleted_accounts ?? 0) > 0 && (
                <p className="text-warning text-xs">
                  Sub2API 已删除 {String(props.task.result.remote_deleted_accounts)}{" "}
                  个账号，本地数据尚未清理；可重新执行完成剩余删除。
                </p>
              )}
            </div>
          )}
        </div>
        <StatusPill
          label={pending ? "处理中" : succeeded ? "成功" : "失败"}
          tone={pending ? "info" : succeeded ? "success" : "danger"}
        />
      </div>
      {succeeded && (
        <div className="divide-y rounded-lg border">
          <ResultSummaryRow label="删除账号" value={`${String(deletedAccounts ?? 0)} 个`} />
          <ResultSummaryRow label="清理分组" value={`${String(deletedGroups ?? 0)} 个`} />
        </div>
      )}
    </div>
  );
}
function BalanceTaskProgress(props: { task: Task }) {
  const result = props.task.result;
  const balance =
    typeof result.balance === "string" || typeof result.balance === "number"
      ? formatBalance(String(result.balance))
      : null;
  const pending = ["queued", "running"].includes(props.task.status);
  const completedWithoutBalance = !pending && props.task.status === "succeeded" && balance === null;
  const failedTask = props.task.status === "failed";
  const statusText = pending
    ? "正在同步余额"
    : failedTask
      ? "同步失败"
      : completedWithoutBalance
        ? "上游未返回余额"
        : "同步成功";
  return (
    <div>
      <div className="flex items-start gap-3">
        <WalletCards
          size={17}
          className={cn(
            "mt-0.5 shrink-0",
            failedTask || completedWithoutBalance ? "text-destructive" : "text-primary",
          )}
        />
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2">
            <strong className="text-sm">{statusText}</strong>
            <StatusPill
              label={statusText}
              tone={failedTask || completedWithoutBalance ? "danger" : pending ? "info" : "success"}
            />
          </div>
          {pending && (
            <span className="text-muted-foreground mt-1 block text-xs">正在向上游同步最新余额</span>
          )}
        </div>
      </div>
      {!pending && (
        <div className="mt-4 divide-y rounded-lg border">
          <ResultSummaryRow label="上游 Host" value={String(result.host ?? "未返回")} />
          <ResultSummaryRow label="当前余额" value={balance ?? "未返回"} />
          <ResultSummaryRow
            label="读取时间"
            value={formatDate(String(result.checked_at ?? props.task.updated_at))}
          />
        </div>
      )}
      {!pending && failedTask && (
        <div className="mt-4">
          <TaskFailureDetail reason={String(result.reason ?? props.task.message)} />
        </div>
      )}
    </div>
  );
}
type UpstreamSyncRow = {
  host: string;
  status: "succeeded" | "auth_failed" | "failed";
  authStatus: string;
  balanceStatus: string;
  balance: string | number | null;
  groupCount: number;
  reason: string | null;
};

function upstreamSyncRows(task: Task): UpstreamSyncRow[] {
  const rawHosts = task.result.hosts;
  if (!Array.isArray(rawHosts)) return [];
  return rawHosts.flatMap((item) => {
    if (typeof item !== "object" || item === null || Array.isArray(item)) return [];
    const row = item as Record<string, unknown>;
    if (typeof row.host !== "string") return [];
    const status =
      row.status === "succeeded" || row.status === "auth_failed" || row.status === "failed"
        ? row.status
        : "failed";
    const rawGroupCount = Number(row.group_count);
    const balance =
      typeof row.balance === "string" || typeof row.balance === "number" ? row.balance : null;
    return [
      {
        host: row.host,
        status,
        authStatus: typeof row.auth_status === "string" ? row.auth_status : "未确认",
        balanceStatus: typeof row.balance_status === "string" ? row.balance_status : "未读取",
        balance,
        groupCount:
          Number.isFinite(rawGroupCount) && rawGroupCount >= 0 ? Math.trunc(rawGroupCount) : 0,
        reason: typeof row.reason === "string" ? row.reason : null,
      },
    ];
  });
}

function syncResultCount(value: unknown): number {
  const count = Number(value);
  return Number.isFinite(count) && count >= 0 ? Math.trunc(count) : 0;
}

function syncRowResult(row: ReturnType<typeof upstreamSyncRows>[number]): string {
  if (row.status === "succeeded") return "同步成功";
  if (row.reason) return row.reason;
  return row.status === "auth_failed" ? "鉴权恢复未通过" : "同步失败";
}

export function UpstreamSyncTaskStatus(props: {
  task: Task;
  scope?: "all" | "balance" | "groups" | "names";
}) {
  const scope = props.scope ?? "all";
  const pending = ["queued", "running"].includes(props.task.status);
  const failed = props.task.status === "failed";
  const rows = upstreamSyncRows(props.task);
  const failedRows = rows.filter((row) => row.status !== "succeeded");
  const visibleRows = failed ? failedRows : rows;
  const pagination = usePaginatedItems(visibleRows);
  if (pending) {
    return (
      <TaskProgressState
        message={displayTaskMessage(props.task.message)}
        progress={props.task.progress}
      />
    );
  }
  if (failed && failedRows.length === 0) {
    return <TaskFailureDetail reason={String(props.task.result.reason ?? props.task.message)} />;
  }
  return (
    <div className="flex h-full min-h-0 flex-col gap-4">
      <div className="grid grid-cols-3 divide-x rounded-lg border">
        <ResultSummaryRow
          label="正常"
          value={`${syncResultCount(props.task.result.succeeded)} 个`}
        />
        <ResultSummaryRow
          label="鉴权失效"
          value={`${syncResultCount(props.task.result.auth_failed)} 个`}
        />
        <ResultSummaryRow
          label="其他失败"
          value={`${syncResultCount(props.task.result.failed)} 个`}
        />
      </div>
      {failed && (
        <div className="flex items-center justify-between gap-3">
          <strong className="text-sm">失败明细</strong>
          <span className="text-muted-foreground text-xs">共 {failedRows.length} 个 Host</span>
        </div>
      )}
      <div className="flex min-h-0 flex-1 flex-col overflow-hidden rounded-lg border">
        <Table
          className={
            scope === "all"
              ? "min-w-[900px]"
              : scope === "names"
                ? "min-w-[520px]"
                : "min-w-[680px]"
          }
          containerClassName="min-h-0 flex-1 overflow-auto"
        >
          <TableHeader className="sticky top-0 z-10">
            <TableRow>
              <TableHead className={scope === "all" ? "w-[22%]" : "w-[25%]"}>Host</TableHead>
              <TableHead className={scope === "all" ? "w-[13%]" : "w-[15%]"}>鉴权</TableHead>
              {scope !== "groups" && scope !== "names" && (
                <TableHead className={scope === "all" ? "w-[12%]" : "w-[15%]"}>余额</TableHead>
              )}
              {scope !== "balance" && scope !== "names" && (
                <TableHead className={scope === "all" ? "w-[8%]" : "w-[12%]"}>分组</TableHead>
              )}
              <TableHead className={scope === "all" ? "w-[45%]" : "w-[48%]"}>
                {scope === "names" ? "名称读取" : "结果"}
              </TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {!visibleRows.length && (
              <TableMessageRow columns={scope === "all" ? 5 : scope === "names" ? 3 : 4}>
                <EmptyRow text="当前没有需要同步的上游 Host" />
              </TableMessageRow>
            )}
            {pagination.visibleItems.map((row) => (
              <TableRow key={row.host}>
                <TableCell className="font-medium">{row.host}</TableCell>
                <TableCell>
                  <StatusPill
                    label={row.authStatus}
                    tone={
                      row.status === "auth_failed"
                        ? "danger"
                        : row.status === "succeeded"
                          ? "success"
                          : "warning"
                    }
                  />
                </TableCell>
                {scope !== "groups" && scope !== "names" && (
                  <TableCell>
                    {row.balance === null ? row.balanceStatus : formatBalance(String(row.balance))}
                  </TableCell>
                )}
                {scope !== "balance" && scope !== "names" && (
                  <TableCell>{row.groupCount}</TableCell>
                )}
                <TableCell
                  className="whitespace-normal break-words leading-5"
                  data-column="result"
                  overflowTooltip={false}
                >
                  {row.status === "succeeded" ? (
                    <StatusPill
                      label={scope === "names" ? "已重新读取" : "同步成功"}
                      tone="success"
                    />
                  ) : (
                    <span className="text-destructive text-sm">{syncRowResult(row)}</span>
                  )}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
        {visibleRows.length > 0 ? (
          <DataTablePagination
            currentPage={pagination.currentPage}
            totalPages={pagination.totalPages}
            totalItems={visibleRows.length}
            pageSize={pagination.pageSize}
            pageSizes={[10, 20, 50, 100]}
            onPageChange={pagination.setCurrentPage}
            onPageSizeChange={pagination.setPageSize}
          />
        ) : null}
      </div>
    </div>
  );
}
function TaskResult(props: { task: Task }) {
  const result = { ...props.task.result };
  if (Object.prototype.hasOwnProperty.call(result, "captcha_challenge"))
    result.captcha_challenge = "请在验证码操作面板完成";
  const rows = flattenTaskResult(result);
  return (
    <div className="border-border mt-4 divide-y rounded-lg border">
      {rows.map((row, index) => {
        const labels = row.path.map((key) => displayResultKey(key));
        return (
          <div
            className="grid grid-cols-[minmax(100px,160px)_minmax(0,1fr)] gap-3 px-3 py-2 text-sm"
            key={`${row.path.join(".")}:${index}`}
          >
            <span className="text-muted-foreground break-words">
              {labels.join(" / ") || "结果"}
            </span>
            <strong className="break-words font-medium">{displayResultValue(row.value)}</strong>
          </div>
        );
      })}
    </div>
  );
}

function SettingsControlRow(props: {
  title: string;
  description: string;
  children: React.ReactNode;
  controlId?: string;
  controlDisabled?: boolean;
}) {
  const content = (
    <>
      <h3 className="text-sm font-medium">{props.title}</h3>
      <p className="text-muted-foreground mt-0.5 text-xs leading-4">{props.description}</p>
    </>
  );
  return (
    <div className="border-border/70 bg-muted/35 flex min-h-14 flex-col items-stretch justify-between gap-3 rounded-lg border px-3 py-2.5 sm:flex-row sm:items-center sm:gap-4">
      {props.controlId ? (
        <label
          className={cn("min-w-0", props.controlDisabled ? "cursor-not-allowed" : "cursor-pointer")}
          htmlFor={props.controlId}
        >
          {content}
        </label>
      ) : (
        <div className="min-w-0">{content}</div>
      )}
      <div className="shrink-0 self-end sm:self-center">{props.children}</div>
    </div>
  );
}

export function targetFormFromConfig(
  value: Pick<RuntimeConfig, "admin_base_url" | "request_timeout_seconds">,
) {
  return {
    admin_base_url: value.admin_base_url ?? "",
    admin_key: "",
    request_timeout_seconds: String(value.request_timeout_seconds),
  };
}

export function notificationFormFromStatus(value?: NotificationStatus): {
  app_id: string;
  client_secret: string;
  home_channel: string;
  home_channel_type: "c2c" | "group" | "channel";
} {
  const channelType = value?.channel_type;
  return {
    app_id: value?.app_id ?? "",
    client_secret: "",
    home_channel: value?.home_channel ?? "",
    home_channel_type: (channelType === "group" || channelType === "channel"
      ? channelType
      : "c2c") as "c2c" | "group" | "channel",
  };
}

const qqBotDeveloperSettingsURL = "https://q.qq.com/qqbot/#/developer/developer-setting";
const qqBotEventDocsURL =
  "https://bot.q.qq.com/wiki/develop/api-v2/dev-prepare/interface-framework/event-emit.html";

function NotificationFieldHelp(props: { children: React.ReactNode; href: string; link: string }) {
  return (
    <p className="text-muted-foreground text-xs leading-5 font-normal">
      {props.children}{" "}
      <a
        className="text-foreground inline-flex items-center gap-1 font-medium underline underline-offset-4"
        href={props.href}
        target="_blank"
        rel="noreferrer"
      >
        {props.link}
        <ExternalLink size={12} aria-hidden="true" />
      </a>
    </p>
  );
}

export function notificationTargetField(type: "c2c" | "group" | "channel"): {
  placeholder: string;
  description: string;
} {
  if (type === "group") {
    return {
      placeholder: "输入 group_openid",
      description:
        "点击“连接获取”后，在目标群里 @机器人并发送任意消息，系统会自动填入 group_openid；机器人无需回复。",
    };
  }
  if (type === "channel") {
    return {
      placeholder: "输入 channel_id",
      description:
        "点击“连接获取”后，在目标子频道里 @机器人并发送任意消息，系统会自动填入 channel_id；机器人无需回复。",
    };
  }
  return {
    placeholder: "输入 user_openid",
    description:
      "点击“连接获取”后，给机器人发送任意私聊消息，系统会自动填入 user_openid；机器人无需回复。",
  };
}

export function notificationTargetResultFromTask(task?: Task): {
  id: string;
  type: "c2c" | "group" | "channel";
  eventType: string;
  sourceName: string;
  capturedAt: string;
} | null {
  if (task?.status !== "succeeded") return null;
  const id = typeof task.result.target_id === "string" ? task.result.target_id.trim() : "";
  const type = task.result.target_type;
  if (!id || (type !== "c2c" && type !== "group" && type !== "channel")) return null;
  return {
    id,
    type,
    eventType: typeof task.result.event_type === "string" ? task.result.event_type : "",
    sourceName: typeof task.result.source_name === "string" ? task.result.source_name : "",
    capturedAt: typeof task.result.captured_at === "string" ? task.result.captured_at : "",
  };
}

function notificationTargetTaskPresentation(status?: Task["status"]): {
  label: string;
  tone: "success" | "warning" | "danger" | "info" | "neutral";
} {
  switch (status) {
    case "queued":
      return { label: "准备连接", tone: "neutral" };
    case "running":
      return { label: "连接中", tone: "info" };
    case "waiting_input":
      return { label: "等待消息", tone: "warning" };
    case "succeeded":
      return { label: "已获取", tone: "success" };
    case "failed":
      return { label: "获取失败", tone: "danger" };
    case "cancelled":
      return { label: "已取消", tone: "neutral" };
    default:
      return { label: "正在启动", tone: "neutral" };
  }
}

export function ConfigPage() {
  const queryClient = useQueryClient();
  const config = useQuery({ queryKey: ["config"], queryFn: api.config });
  const notifications = useQuery({
    queryKey: ["notification-status"],
    queryFn: api.notificationStatus,
    refetchInterval: 15_000,
  });
  const logCleanup = useQuery({
    queryKey: ["log-cleanup"],
    queryFn: api.logCleanupStatus,
    refetchInterval: 30_000,
  });
  const [notificationForm, setNotificationForm] = useState(() =>
    notificationFormFromStatus(notifications.data),
  );
  const notificationTarget = notificationTargetField(notificationForm.home_channel_type);
  const [notificationEdited, setNotificationEdited] = useState(false);
  const [targetForm, setTargetForm] = useState({
    admin_base_url: "",
    admin_key: "",
    request_timeout_seconds: "30",
  });
  const [targetEdited, setTargetEdited] = useState(false);
  const [accountDefaultsEdited, setAccountDefaultsEdited] = useState(false);
  const [accountDefaultsForm, setAccountDefaultsForm] = useState({
    concurrency: "10",
    priority: "1",
  });
  const [logCleanupEdited, setLogCleanupEdited] = useState(false);
  const [clearLogsOpen, setClearLogsOpen] = useState(false);
  const [logCleanupForm, setLogCleanupForm] = useState({
    enabled: false,
    retention_days: "30",
  });
  const [managementTaskId, setManagementTaskId] = useState<string | null>(null);
  const managementTask = useQuery({
    queryKey: ["task", managementTaskId],
    queryFn: () => api.task(managementTaskId!),
    enabled: managementTaskId !== null,
    refetchInterval: taskPollInterval,
  });
  const [notificationTargetTaskId, setNotificationTargetTaskId] = useState<string | null>(null);
  const notificationTargetTask = useQuery({
    queryKey: ["task", notificationTargetTaskId],
    queryFn: () => api.task(notificationTargetTaskId!),
    enabled: notificationTargetTaskId !== null,
    refetchInterval: (query) => {
      const status = query.state.data?.status;
      return status === "succeeded" || status === "failed" || status === "cancelled"
        ? false
        : 1_000;
    },
  });
  const notificationTargetResult = notificationTargetResultFromTask(notificationTargetTask.data);
  const notificationTargetTaskActive =
    notificationTargetTaskId !== null &&
    notificationTargetTask.data?.status !== "succeeded" &&
    notificationTargetTask.data?.status !== "failed" &&
    notificationTargetTask.data?.status !== "cancelled";
  useEffect(() => {
    if (!config.data || targetEdited) return;
    setTargetForm(targetFormFromConfig(config.data));
  }, [
    config.data,
    config.data?.admin_base_url,
    config.data?.request_timeout_seconds,
    targetEdited,
  ]);
  useEffect(() => {
    if (!config.data || accountDefaultsEdited) return;
    setAccountDefaultsForm({
      concurrency: String(config.data.account_default_concurrency),
      priority: String(config.data.account_default_priority),
    });
  }, [
    accountDefaultsEdited,
    config.data,
    config.data?.account_default_concurrency,
    config.data?.account_default_priority,
  ]);
  useEffect(() => {
    if (!notifications.data || notificationEdited) return;
    setNotificationForm(notificationFormFromStatus(notifications.data));
  }, [notificationEdited, notifications.data]);
  useEffect(() => {
    if (!notificationTargetResult) return;
    setNotificationEdited(true);
    setNotificationForm((current) => ({
      ...current,
      home_channel: notificationTargetResult.id,
      home_channel_type: notificationTargetResult.type,
    }));
  }, [notificationTargetResult?.id, notificationTargetResult?.type]);
  useEffect(() => {
    if (!logCleanup.data || logCleanupEdited) return;
    setLogCleanupForm({
      enabled: logCleanup.data.enabled,
      retention_days: String(logCleanup.data.retention_days),
    });
  }, [
    logCleanup.data,
    logCleanup.data?.enabled,
    logCleanup.data?.retention_days,
    logCleanupEdited,
  ]);
  useEffect(() => {
    if (managementTask.data?.status === "succeeded") {
      toast.success("Sub2API 连接测试与管理数据同步完成");
      void Promise.all([
        queryClient.invalidateQueries({ queryKey: ["overview"] }),
        queryClient.invalidateQueries({ queryKey: ["accounts"] }),
        queryClient.invalidateQueries({ queryKey: ["groups"] }),
        queryClient.invalidateQueries({ queryKey: ["logs"] }),
      ]);
    } else if (managementTask.data?.status === "failed") {
      toast.error(managementTask.data.message || "Sub2API 连接测试同步失败");
    }
  }, [managementTask.data?.message, managementTask.data?.status, queryClient]);
  const saveNotification = useMutation({
    mutationFn: () => api.configureNotification(notificationForm),
    onSuccess: (value) => {
      queryClient.setQueryData(["notification-status"], value);
      setNotificationForm(notificationFormFromStatus(value));
      setNotificationEdited(false);
      toast.success("通知配置已保存");
    },
    onError: (error) => toast.error(error instanceof Error ? error.message : "通知配置保存失败"),
  });
  const discoverNotificationTarget = useMutation({
    mutationFn: () =>
      api.discoverNotificationTarget({
        app_id: notificationForm.app_id,
        client_secret: notificationForm.client_secret,
        target_type: notificationForm.home_channel_type,
      }),
    onSuccess: (task) => setNotificationTargetTaskId(task.id),
    onError: (error) =>
      toast.error(error instanceof Error ? error.message : "QQBot 目标获取启动失败"),
  });
  const cancelNotificationTargetDiscovery = useMutation({
    mutationFn: () => api.cancelNotificationTargetDiscovery(notificationTargetTaskId!),
    onSuccess: () => void notificationTargetTask.refetch(),
    onError: (error) =>
      toast.error(error instanceof Error ? error.message : "QQBot 目标获取取消失败"),
  });
  const saveLogCleanup = useMutation({
    mutationFn: () =>
      api.updateLogCleanup({
        enabled: logCleanupForm.enabled,
        retention_days: Number(logCleanupForm.retention_days),
      }),
    onSuccess: (value) => {
      queryClient.setQueryData(["log-cleanup"], value);
      setLogCleanupEdited(false);
      toast.success(value.enabled ? "日志自动清理已开启" : "日志自动清理已关闭");
    },
    onError: (error) =>
      toast.error(error instanceof Error ? error.message : "日志清理设置保存失败"),
  });
  const clearLogs = useMutation({
    mutationFn: () => api.clearLogs(Number(logCleanupForm.retention_days)),
    onSuccess: async (result) => {
      setClearLogsOpen(false);
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["logs"] }),
        queryClient.invalidateQueries({ queryKey: ["log-cleanup"] }),
        queryClient.invalidateQueries({ queryKey: ["overview"] }),
      ]);
      const protectedText = result.protected_tasks
        ? `，保留 ${result.protected_tasks} 个执行中任务`
        : "";
      toast.success(
        `已清理 ${result.retention_days} 天以前的 ${result.total} 条日志${protectedText}`,
      );
    },
    onError: (error) => toast.error(error instanceof Error ? error.message : "日志清空失败"),
  });
  const testNotification = useMutation({
    mutationFn: api.testNotification,
    onSuccess: (result) => {
      toast.success(
        result.detail === undefined
          ? result.sent
            ? "测试通知已发送"
            : "测试通知已完成"
          : explicitValue(result.detail),
      );
      void Promise.all([
        queryClient.invalidateQueries({ queryKey: ["logs"] }),
        queryClient.invalidateQueries({ queryKey: ["overview"] }),
      ]);
    },
    onError: (error) => toast.error(error instanceof Error ? error.message : "测试通知失败"),
  });
  const saveTarget = useMutation({
    mutationFn: async (syncAfterSave: boolean) => {
      const value = await api.setAdminTarget({
        admin_base_url: targetForm.admin_base_url,
        admin_key: targetForm.admin_key,
        request_timeout_seconds: Number(targetForm.request_timeout_seconds),
      });
      const task = syncAfterSave ? await api.syncManagement() : null;
      return { value, task };
    },
    onSuccess: ({ value, task }) => {
      setTargetForm(targetFormFromConfig(value));
      setTargetEdited(false);
      queryClient.setQueryData(["config"], value);
      void queryClient.invalidateQueries({ queryKey: ["overview"] });
      if (task) {
        setManagementTaskId(task.id);
        toast.success("连接已保存，正在测试同步");
      } else {
        toast.success("连接配置已保存");
      }
    },
    onError: (error) => toast.error(error instanceof Error ? error.message : "管理目标保存失败"),
  });
  const saveAccountDefaults = useMutation({
    mutationFn: () =>
      api.setAccountDefaults({
        concurrency: Number(accountDefaultsForm.concurrency),
        priority: Number(accountDefaultsForm.priority),
      }),
    onSuccess: (value) => {
      queryClient.setQueryData(["config"], value);
      setAccountDefaultsForm({
        concurrency: String(value.account_default_concurrency),
        priority: String(value.account_default_priority),
      });
      setAccountDefaultsEdited(false);
      toast.success("账号开户默认参数已保存");
    },
    onError: (error) =>
      toast.error(error instanceof Error ? error.message : "账号开户默认参数保存失败"),
  });
  if (config.error)
    return (
      <PageLayout>
        <PageHeading
          eyebrow="SYSTEM / SETTINGS"
          title="系统设置"
          description="管理平台连接、通知渠道和日志保留；调度决策与执行参数统一在调度策略中配置。"
        />
        <QueryError error={config.error} fallback="系统设置读取失败" />
      </PageLayout>
    );
  return (
    <PageLayout>
      <PageHeading
        eyebrow="SYSTEM / SETTINGS"
        title="系统设置"
        description="管理平台连接、通知渠道和日志保留；调度决策与执行参数统一在调度策略中配置。"
      />
      <div className="mx-auto w-full max-w-7xl space-y-4">
        {config.data?.configuration_errors?.length ? (
          <div className="border-warning/40 bg-warning/10 text-warning rounded-lg border px-3 py-2 text-sm">
            配置存在无效值：{config.data.configuration_errors.join("、")}
            ；相关功能已停止执行，请修正配置。
          </div>
        ) : null}

        <div className="grid items-start gap-4 xl:grid-cols-2" data-testid="system-settings-flow">
          <Card size="sm">
            <CardHeader>
              <CardTitle>Sub2API 连接</CardTitle>
              <CardDescription>
                Admin API Key 可在 Sub2API 后台的系统设置中获取，保存后不会回显
              </CardDescription>
            </CardHeader>
            <CardContent className="grid gap-4">
              <FormField label="Sub2API 地址">
                <Input
                  type="url"
                  value={targetForm.admin_base_url}
                  onChange={(event) => {
                    setTargetEdited(true);
                    setTargetForm({
                      ...targetForm,
                      admin_base_url: event.target.value,
                    });
                  }}
                  placeholder="https://sub2api.example.com"
                />
              </FormField>
              <FormField label="Admin API Key">
                <>
                  <Input
                    type="password"
                    value={targetForm.admin_key}
                    onChange={(event) => {
                      setTargetEdited(true);
                      setTargetForm({
                        ...targetForm,
                        admin_key: event.target.value,
                      });
                    }}
                    placeholder={sensitiveFieldPlaceholder(
                      config.data?.target_configured === true,
                      "输入 Admin API Key",
                    )}
                  />
                  {config.data?.target_configured ? (
                    <p className="text-muted-foreground text-xs font-normal">
                      已配置，留空则不修改。
                    </p>
                  ) : null}
                </>
              </FormField>
              <FormField label="请求超时（秒）">
                <>
                  <Input
                    type="number"
                    min={1}
                    max={120}
                    inputMode="numeric"
                    value={targetForm.request_timeout_seconds}
                    onChange={(event) => {
                      setTargetEdited(true);
                      setTargetForm({
                        ...targetForm,
                        request_timeout_seconds: event.target.value,
                      });
                    }}
                  />
                  <p className="text-muted-foreground text-xs font-normal">
                    Admin API 请求超时，允许 1–120 秒。
                  </p>
                </>
              </FormField>
              <div className="flex flex-wrap gap-2 pt-1">
                <Button
                  onClick={() => saveTarget.mutate(false)}
                  disabled={
                    saveTarget.isPending ||
                    !targetForm.admin_base_url.trim() ||
                    (!config.data?.target_configured && !targetForm.admin_key.trim()) ||
                    !Number.isInteger(Number(targetForm.request_timeout_seconds)) ||
                    Number(targetForm.request_timeout_seconds) < 1 ||
                    Number(targetForm.request_timeout_seconds) > 120
                  }
                >
                  <Save size={16} />
                  {saveTarget.isPending ? "保存中…" : "保存连接"}
                </Button>
                <Button
                  variant="outline"
                  onClick={() => saveTarget.mutate(true)}
                  disabled={
                    saveTarget.isPending ||
                    taskIsPending(managementTaskId, managementTask) ||
                    !targetForm.admin_base_url.trim() ||
                    (!config.data?.target_configured && !targetForm.admin_key.trim()) ||
                    !Number.isInteger(Number(targetForm.request_timeout_seconds)) ||
                    Number(targetForm.request_timeout_seconds) < 1 ||
                    Number(targetForm.request_timeout_seconds) > 120
                  }
                >
                  <RefreshCw size={16} />
                  {taskIsPending(managementTaskId, managementTask)
                    ? `测试同步 ${managementTask.data?.progress ?? 0}%`
                    : "保存并测试同步"}
                </Button>
              </div>
            </CardContent>
          </Card>

          <Card size="sm">
            <CardHeader>
              <CardTitle>账号开户默认参数</CardTitle>
              <CardDescription>添加账号和参数修复统一使用；添加时仍可单独覆盖</CardDescription>
            </CardHeader>
            <CardContent className="grid gap-4">
              <div className="grid gap-4 sm:grid-cols-2">
                <FormField label="默认并发">
                  <Input
                    type="number"
                    min={1}
                    max={10_000_000}
                    inputMode="numeric"
                    value={accountDefaultsForm.concurrency}
                    onChange={(event) => {
                      setAccountDefaultsEdited(true);
                      setAccountDefaultsForm({
                        ...accountDefaultsForm,
                        concurrency: event.target.value,
                      });
                    }}
                  />
                </FormField>
                <FormField label="默认优先级">
                  <Input
                    type="number"
                    min={1}
                    max={10_000_000}
                    inputMode="numeric"
                    value={accountDefaultsForm.priority}
                    onChange={(event) => {
                      setAccountDefaultsEdited(true);
                      setAccountDefaultsForm({
                        ...accountDefaultsForm,
                        priority: event.target.value,
                      });
                    }}
                  />
                </FormField>
              </div>
              <div>
                <Button
                  onClick={() => saveAccountDefaults.mutate()}
                  disabled={
                    saveAccountDefaults.isPending ||
                    !accountDefaultsEdited ||
                    ![accountDefaultsForm.concurrency, accountDefaultsForm.priority].every(
                      (value) =>
                        Number.isInteger(Number(value)) &&
                        Number(value) >= 1 &&
                        Number(value) <= 10_000_000,
                    )
                  }
                >
                  <Save size={16} />
                  {saveAccountDefaults.isPending ? "保存中…" : "保存默认参数"}
                </Button>
              </div>
            </CardContent>
          </Card>

          <Card size="sm" className="xl:row-span-2">
            <CardHeader className="flex items-start justify-between gap-3">
              <div className="min-w-0">
                <CardTitle>通知设置</CardTitle>
                <CardDescription className="mt-1">
                  配置 QQBot 通知发送目标，密钥只写入后端
                </CardDescription>
              </div>
              <StatusPill
                label={
                  notifications.isLoading
                    ? "读取中"
                    : notifications.error
                      ? "读取失败"
                      : notifications.data?.configured
                        ? "已配置"
                        : "未配置"
                }
                tone={
                  notifications.data?.configured && !notifications.error ? "success" : "neutral"
                }
              />
            </CardHeader>
            <CardContent className="grid gap-5">
              {notifications.error ? (
                <QueryError error={notifications.error} fallback="通知状态读取失败" embedded />
              ) : null}
              <div
                className="grid items-start gap-x-5 gap-y-4 sm:grid-cols-2"
                data-testid="notification-credentials"
              >
                <FormField label="App ID">
                  <>
                    <Input
                      value={notificationForm.app_id}
                      disabled={notificationTargetTaskActive}
                      onChange={(event) => {
                        setNotificationEdited(true);
                        setNotificationForm({
                          ...notificationForm,
                          app_id: event.target.value,
                        });
                      }}
                      placeholder="输入 App ID"
                    />
                    <NotificationFieldHelp href={qqBotDeveloperSettingsURL} link="打开 QQ 开放平台">
                      在机器人管理页的“开发设置”中复制 App ID。
                    </NotificationFieldHelp>
                  </>
                </FormField>
                <FormField label="Client Secret">
                  <>
                    <Input
                      type="password"
                      value={notificationForm.client_secret}
                      disabled={notificationTargetTaskActive}
                      onChange={(event) => {
                        setNotificationEdited(true);
                        setNotificationForm({
                          ...notificationForm,
                          client_secret: event.target.value,
                        });
                      }}
                      placeholder={sensitiveFieldPlaceholder(
                        notifications.data?.client_secret_configured === true,
                        "输入 Client Secret",
                      )}
                    />
                    <NotificationFieldHelp href={qqBotDeveloperSettingsURL} link="打开 QQ 开放平台">
                      与 App ID 在同一“开发设置”页面获取；保存后不会回显。
                    </NotificationFieldHelp>
                  </>
                </FormField>
              </div>
              <div
                className="grid items-start gap-x-5 gap-y-4 sm:grid-cols-[minmax(10rem,0.7fr)_minmax(0,1.3fr)]"
                data-testid="notification-destination"
              >
                <FormField label="目标类型">
                  <Select
                    value={notificationForm.home_channel_type}
                    disabled={notificationTargetTaskActive}
                    onValueChange={(value) => {
                      if (!value) return;
                      setNotificationEdited(true);
                      setNotificationForm({
                        ...notificationForm,
                        home_channel_type: value as "c2c" | "group" | "channel",
                      });
                    }}
                  >
                    <SelectTrigger>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="c2c">私聊</SelectItem>
                      <SelectItem value="group">群聊</SelectItem>
                      <SelectItem value="channel">频道</SelectItem>
                    </SelectContent>
                  </Select>
                </FormField>
                <FormField label="目标 ID">
                  <>
                    <div className="grid gap-2 sm:grid-cols-[minmax(0,1fr)_auto]">
                      <Input
                        value={notificationForm.home_channel}
                        disabled={notificationTargetTaskActive}
                        onChange={(event) => {
                          setNotificationEdited(true);
                          setNotificationForm({
                            ...notificationForm,
                            home_channel: event.target.value,
                          });
                        }}
                        placeholder={notificationTarget.placeholder}
                      />
                      <Button
                        type="button"
                        variant="outline"
                        onClick={() => discoverNotificationTarget.mutate()}
                        disabled={
                          discoverNotificationTarget.isPending ||
                          notificationTargetTaskActive ||
                          !notificationForm.app_id.trim() ||
                          (!notifications.data?.client_secret_configured &&
                            !notificationForm.client_secret.trim())
                        }
                      >
                        <ScanSearch size={16} aria-hidden="true" />
                        {notificationTargetTaskActive ? "等待消息" : "连接获取"}
                      </Button>
                    </div>
                    <NotificationFieldHelp href={qqBotEventDocsURL} link="查看事件服务接入说明">
                      {notificationTarget.description}
                    </NotificationFieldHelp>
                  </>
                </FormField>
                {(discoverNotificationTarget.isPending || notificationTargetTaskId) && (
                  <div
                    className="border-border/70 bg-muted/30 grid gap-3 rounded-md border px-3 py-3 sm:col-span-2"
                    data-testid="notification-target-discovery"
                    role="status"
                  >
                    {discoverNotificationTarget.isPending && !notificationTargetTaskId ? (
                      <div className="flex items-center gap-2 text-sm">
                        <RefreshCw className="animate-spin" size={16} aria-hidden="true" />
                        正在创建 QQBot 连接任务
                      </div>
                    ) : null}
                    {notificationTargetTask.error ? (
                      <QueryError
                        error={notificationTargetTask.error}
                        fallback="QQBot 目标获取状态读取失败"
                        embedded
                      />
                    ) : null}
                    {notificationTargetTask.data ? (
                      <>
                        <div className="flex flex-wrap items-start justify-between gap-3">
                          <div className="min-w-0">
                            <p className="text-sm font-medium">自动获取目标 ID</p>
                            <p className="text-muted-foreground mt-1 text-xs leading-5">
                              {notificationTargetTask.data.message}
                            </p>
                          </div>
                          <StatusPill
                            label={
                              notificationTargetTaskPresentation(notificationTargetTask.data.status)
                                .label
                            }
                            tone={
                              notificationTargetTaskPresentation(notificationTargetTask.data.status)
                                .tone
                            }
                          />
                        </div>
                        {notificationTargetResult ? (
                          <div className="border-border/70 bg-background grid gap-1 rounded-md border px-3 py-2">
                            <span className="text-muted-foreground text-xs">已自动填入目标 ID</span>
                            <code className="break-all text-sm">{notificationTargetResult.id}</code>
                            {notificationTargetResult.sourceName ? (
                              <span className="text-muted-foreground text-xs">
                                触发用户：{notificationTargetResult.sourceName}
                              </span>
                            ) : null}
                          </div>
                        ) : null}
                        {notificationTargetTaskActive ? (
                          <div className="flex justify-end">
                            <Button
                              type="button"
                              variant="outline"
                              size="sm"
                              onClick={() => cancelNotificationTargetDiscovery.mutate()}
                              disabled={cancelNotificationTargetDiscovery.isPending}
                            >
                              <X size={15} aria-hidden="true" />
                              {cancelNotificationTargetDiscovery.isPending ? "取消中…" : "取消等待"}
                            </Button>
                          </div>
                        ) : null}
                      </>
                    ) : null}
                  </div>
                )}
              </div>
              {notifications.data?.configuration_errors?.length ? (
                <p className="text-destructive text-sm">
                  通知配置无效：
                  {notifications.data.configuration_errors.join("、")}
                </p>
              ) : null}
              <div className="border-border/70 flex flex-wrap justify-end gap-2 border-t pt-4">
                <Button
                  variant="outline"
                  onClick={() => testNotification.mutate()}
                  disabled={
                    testNotification.isPending ||
                    notificationTargetTaskActive ||
                    notifications.error != null ||
                    !notifications.data?.configured
                  }
                >
                  <BellRing size={16} />
                  {testNotification.isPending ? "测试中…" : "测试通知"}
                </Button>
                <Button
                  onClick={() => saveNotification.mutate()}
                  disabled={
                    saveNotification.isPending ||
                    notificationTargetTaskActive ||
                    notifications.isLoading ||
                    notifications.error != null ||
                    !notificationForm.app_id.trim() ||
                    !notificationForm.home_channel.trim() ||
                    (!notifications.data?.client_secret_configured &&
                      !notificationForm.client_secret.trim())
                  }
                >
                  <Save size={16} />
                  {saveNotification.isPending ? "保存中…" : "保存通知设置"}
                </Button>
              </div>
            </CardContent>
          </Card>

          <Card size="sm">
            <CardHeader className="flex items-start justify-between gap-3">
              <div className="min-w-0">
                <CardTitle>日志保留</CardTitle>
                <CardDescription className="mt-1">
                  管理日志中心的任务、运行、事件和变更记录，不影响业务数据
                </CardDescription>
              </div>
              <StatusPill
                label={
                  logCleanup.isLoading
                    ? "读取中"
                    : logCleanup.error
                      ? "读取失败"
                      : logCleanup.data?.enabled
                        ? "自动清理已开启"
                        : "自动清理已关闭"
                }
                tone={logCleanup.data?.enabled && !logCleanup.error ? "success" : "neutral"}
              />
            </CardHeader>
            <CardContent className="grid gap-3">
              {logCleanup.error ? (
                <div>
                  <QueryError error={logCleanup.error} fallback="日志清理配置读取失败" embedded />
                </div>
              ) : null}
              <div className="grid gap-x-5 gap-y-3 sm:grid-cols-2">
                <div className="flex min-w-0 items-center justify-between gap-3">
                  <label className="min-w-0 cursor-pointer" htmlFor="log-cleanup-enabled">
                    <span className="block text-sm font-medium">定时清理</span>
                    <span className="text-muted-foreground block text-xs leading-4">
                      每天检查并删除超过保留期的日志
                    </span>
                  </label>
                  <Switch
                    id="log-cleanup-enabled"
                    className="shrink-0"
                    checked={logCleanupForm.enabled}
                    disabled={saveLogCleanup.isPending || logCleanup.isLoading}
                    aria-label="定时清理"
                    onCheckedChange={(enabled) => {
                      setLogCleanupEdited(true);
                      setLogCleanupForm({ ...logCleanupForm, enabled });
                    }}
                  />
                </div>
                <div className="flex min-w-0 items-center justify-between gap-3">
                  <div className="min-w-0">
                    <span className="block text-sm font-medium">日志保留天数</span>
                    <span className="text-muted-foreground block text-xs leading-4">
                      允许 1–3650 天
                    </span>
                  </div>
                  <div className="flex shrink-0 items-center gap-2">
                    <Input
                      className="w-20"
                      type="number"
                      min={1}
                      max={3650}
                      aria-label="日志保留天数"
                      value={logCleanupForm.retention_days}
                      onChange={(event) => {
                        setLogCleanupEdited(true);
                        setLogCleanupForm({
                          ...logCleanupForm,
                          retention_days: event.target.value,
                        });
                      }}
                    />
                    <span className="text-muted-foreground text-sm">天</span>
                  </div>
                </div>
                <div className="border-border/70 flex flex-wrap items-end justify-between gap-3 border-t pt-3 sm:col-span-2">
                  <div className="min-w-0">
                    <span className="block text-sm font-medium">
                      {logCleanup.data?.last_run_at
                        ? `上次完成 ${formatDate(logCleanup.data.last_run_at, true)}`
                        : "尚未执行"}
                    </span>
                    <span className="text-muted-foreground block text-xs leading-4">
                      {logCleanup.data?.next_run_at
                        ? `下次清理 ${formatDate(logCleanup.data.next_run_at, true)}`
                        : "开启后立即检查，之后每 24 小时检查一次"}
                    </span>
                  </div>
                  <div className="flex flex-wrap justify-end gap-2">
                    <Button
                      variant="outline"
                      disabled={
                        clearLogs.isPending ||
                        !Number.isInteger(Number(logCleanupForm.retention_days)) ||
                        Number(logCleanupForm.retention_days) < 1 ||
                        Number(logCleanupForm.retention_days) > 3650
                      }
                      onClick={() => setClearLogsOpen(true)}
                    >
                      <Trash2 size={16} />
                      立即按期限清理
                    </Button>
                    <Button
                      disabled={
                        saveLogCleanup.isPending ||
                        !Number.isInteger(Number(logCleanupForm.retention_days)) ||
                        Number(logCleanupForm.retention_days) < 1 ||
                        Number(logCleanupForm.retention_days) > 3650
                      }
                      onClick={() => saveLogCleanup.mutate()}
                    >
                      <Save size={16} />
                      {saveLogCleanup.isPending ? "保存中…" : "保存日志设置"}
                    </Button>
                  </div>
                </div>
              </div>
            </CardContent>
          </Card>
        </div>
      </div>
      <Dialog
        open={clearLogsOpen}
        onOpenChange={(open) => {
          if (!clearLogs.isPending) setClearLogsOpen(open);
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>按保留期限清理日志</DialogTitle>
            <DialogDescription>
              将删除 {logCleanupForm.retention_days}{" "}
              天以前的任务、运行、事件和变更记录。期限内日志和执行中的任务会保留；账号、分组、策略、告警、健康样本和上游使用记录不受影响。
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button
              variant="outline"
              disabled={clearLogs.isPending}
              onClick={() => setClearLogsOpen(false)}
            >
              取消
            </Button>
            <Button
              variant="destructive"
              disabled={clearLogs.isPending}
              onClick={() => clearLogs.mutate()}
            >
              <Trash2 size={15} />
              {clearLogs.isPending ? "清理中…" : "确认清理"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </PageLayout>
  );
}

function autoInspectionConfig(value: AutoInspectionStatus): AutoInspectionConfig {
  return {
    enabled: value.enabled,
    interval_seconds: value.interval_seconds,
  };
}

const autoInspectionOperationLabels: Record<string, string> = {
  upstream_sync: "上游数据同步",
  upstream_rate_sync: "上游数据同步",
  account_rate_sync: "账号倍率与名称同步",
  traffic_refresh: "真实流量同步",
  active_probe: "主动探测",
  evidence_collection: "请求记录与探针",
  routing_calculation: "调度计算",
  inspection_calculation: "调度计算",
  routing_writeback: "自动执行",
  alert_evaluation: "告警检测",
};
const autoInspectionUpstreamSyncOperation = "upstream_sync";

function formatDurationSeconds(durationSeconds: number): string {
  const totalSeconds = durationSeconds > 0 ? Math.max(1, Math.floor(durationSeconds)) : 0;
  const hours = Math.floor(totalSeconds / 3600);
  const minutes = Math.floor((totalSeconds % 3600) / 60);
  const seconds = totalSeconds % 60;
  if (hours > 0) return `${hours}小时${minutes}分${seconds}秒`;
  if (minutes > 0) return `${minutes}分${seconds}秒`;
  return `${seconds}秒`;
}

function formatAutoInspectionDuration(
  checkedAt: string,
  completedAt: string | null,
  clock: number,
): string {
  const started = new Date(checkedAt).getTime();
  const completed = completedAt ? new Date(completedAt).getTime() : clock;
  if (!Number.isFinite(started) || !Number.isFinite(completed)) return "—";
  return formatDurationSeconds((completed - started) / 1000);
}

function formatInspectionRunDuration(durationMs: number): string {
  if (!Number.isFinite(durationMs) || durationMs < 0) return "—";
  if (durationMs < 1_000) return `${Math.round(durationMs)} 毫秒`;
  if (durationMs < 60_000) {
    const seconds = Math.round(durationMs / 100) / 10;
    return `${seconds} 秒`;
  }
  const totalSeconds = Math.round(durationMs / 1_000);
  const minutes = Math.floor(totalSeconds / 60);
  const seconds = totalSeconds % 60;
  return `${minutes} 分 ${seconds} 秒`;
}

function autoInspectionLastRunState(status: AutoInspectionStatus["last_status"]) {
  if (status === "succeeded") return { label: "执行成功", tone: "success" as const };
  if (status === "failed") return { label: "执行失败", tone: "danger" as const };
  if (status === "cancelled") return { label: "已取消", tone: "neutral" as const };
  return { label: "尚未执行", tone: "neutral" as const };
}

function InspectionSummaryCount(props: { label: string; value: number }) {
  return (
    <div className="min-w-0 px-3 py-3 sm:px-4">
      <div className="text-muted-foreground text-xs leading-5">{props.label}</div>
      <strong className="mt-0.5 block text-lg leading-6 font-semibold tabular-nums">
        {props.value}
      </strong>
    </div>
  );
}

function formatAutoInspectionCountdown(scheduledFor: string | null, clock: number): string {
  if (!scheduledFor) return "等待排期";
  const scheduled = new Date(scheduledFor).getTime();
  if (!Number.isFinite(scheduled)) return "等待排期";
  const remainingSeconds = Math.ceil((scheduled - clock) / 1000);
  if (remainingSeconds <= 0) return "即将检查";
  return `${formatDurationSeconds(remainingSeconds)}后检查`;
}

function autoInspectionQueueState(value: AutoInspectionStatus["queue"][number]) {
  if (value.state === "ready") return { label: "待执行", tone: "info" as const };
  if (value.state === "waiting") return { label: "等待下一次检查", tone: "neutral" as const };
  if (value.state === "blocked") return { label: "配置阻塞", tone: "danger" as const };
  return { label: "未启用", tone: "neutral" as const };
}

function AutoInspectionTaskIcon() {
  return <Activity size={15} aria-hidden="true" />;
}

function autoInspectionHeartbeatState(record: AutoInspectionStatus["heartbeat_history"][number]) {
  if (record.status === "running") return { label: "执行中", tone: "info" as const };
  if (record.status === "succeeded") return { label: "正常", tone: "success" as const };
  return { label: "失败", tone: "danger" as const };
}

type InspectionUpstreamSyncSummary = {
  upstreamTotal: number;
  upstreamSucceeded: number;
  upstreamFailed: number;
  accountTotal: number | null;
  accountRateSucceeded: number;
  accountRateFailed: number | null;
};

function inspectionResultCount(value: Record<string, unknown>, key: string): number | null {
  const count = value[key];
  if (typeof count !== "number" || !Number.isInteger(count) || count < 0) return null;
  return count;
}

function legacyInspectionAccountCounts(
  result: Record<string, unknown>,
  upstreams?: UpstreamSummary,
): { total: number; succeeded: number; failed: number } | null {
  if (!upstreams || !Array.isArray(result.hosts)) return null;
  const currentCounts = new Map(
    upstreams.hosts.map((host) => [host.host.trim().toLowerCase(), host.account_count]),
  );
  let total = 0;
  let succeeded = 0;
  let matched = 0;
  for (const rawHost of result.hosts) {
    if (rawHost === null || typeof rawHost !== "object" || Array.isArray(rawHost)) continue;
    const host = rawHost as Record<string, unknown>;
    const name = typeof host.host === "string" ? host.host.trim().toLowerCase() : "";
    const accountCount = currentCounts.get(name);
    if (accountCount === undefined) continue;
    matched++;
    total += accountCount;
    if (host.status === "succeeded") succeeded += accountCount;
  }
  if (matched === 0) return null;
  return { total, succeeded, failed: total - succeeded };
}

function inspectionUpstreamSyncSummary(
  task?: Task,
  upstreams?: UpstreamSummary,
): InspectionUpstreamSyncSummary | null {
  const raw = task?.result.upstream_sync;
  if (raw === null || typeof raw !== "object" || Array.isArray(raw)) return null;
  const result = raw as Record<string, unknown>;
  const upstreamTotal = inspectionResultCount(result, "total");
  const upstreamSucceeded = inspectionResultCount(result, "succeeded");
  const authFailed = inspectionResultCount(result, "auth_failed");
  const failed = inspectionResultCount(result, "failed");
  if (
    upstreamTotal === null ||
    upstreamSucceeded === null ||
    authFailed === null ||
    failed === null
  ) {
    return null;
  }
  const accountTotal = inspectionResultCount(result, "account_total");
  const accountRateFailed = inspectionResultCount(result, "account_rate_failed");
  const persistedAccountRateSucceeded = inspectionResultCount(result, "account_rate_succeeded");
  const legacyAccounts = legacyInspectionAccountCounts(result, upstreams);
  return {
    upstreamTotal,
    upstreamSucceeded,
    upstreamFailed: authFailed + failed,
    accountTotal: accountTotal ?? legacyAccounts?.total ?? null,
    accountRateSucceeded: persistedAccountRateSucceeded ?? legacyAccounts?.succeeded ?? 0,
    accountRateFailed: accountRateFailed ?? legacyAccounts?.failed ?? null,
  };
}

const autoInspectionOperationDescriptions: Record<string, string> = {
  upstream_sync: "同步上游分组目录和该上游全部账号共享的余额。",
  upstream_rate_sync: "同步上游分组目录和该上游全部账号共享的余额。",
  account_rate_sync: "将上游最新倍率同步到绑定账号，并按新倍率更新账号名称。",
  traffic_refresh: "读取真实请求记录并更新账号健康样本。",
  active_probe: "对到期账号执行主动探测并补充健康样本。",
  evidence_collection: "读取真实请求记录并补充到期账号的主动探测样本。",
  routing_calculation: "根据最新健康样本重新计算账号状态和调度目标。",
  inspection_calculation: "根据最新健康样本重新计算账号状态和调度目标。",
  routing_writeback: "将发生变化的调度目标应用到账号。",
  alert_evaluation: "检查当前异常并按通知策略发送告警。",
};

function inspectionTaskResultObject(
  task: Task | undefined,
  key: string,
): Record<string, unknown> | null {
  const value = task?.result[key];
  if (value === null || typeof value !== "object" || Array.isArray(value)) return null;
  return value as Record<string, unknown>;
}

type InspectionTaskQueueOperation = {
  operation: string;
  label: string;
  targetCount: number | null;
  cycle: string | null;
};

function inspectionTaskQueueOperations(task?: Task): InspectionTaskQueueOperation[] {
  const value = task?.result.planned_operations;
  if (!Array.isArray(value)) return [];
  const result: InspectionTaskQueueOperation[] = [];
  for (const item of value) {
    if (item === null || typeof item !== "object" || Array.isArray(item)) continue;
    const operation = (item as Record<string, unknown>).operation;
    if (typeof operation !== "string" || !operation.trim()) continue;
    const rawLabel = (item as Record<string, unknown>).label;
    const rawTargetCount = (item as Record<string, unknown>).target_count;
    const rawCycle = (item as Record<string, unknown>).cycle;
    result.push({
      operation,
      label:
        typeof rawLabel === "string" && rawLabel.trim()
          ? rawLabel
          : (autoInspectionOperationLabels[operation] ?? operation),
      targetCount:
        typeof rawTargetCount === "number" && Number.isInteger(rawTargetCount)
          ? rawTargetCount
          : null,
      cycle: typeof rawCycle === "string" && rawCycle.trim() ? rawCycle : null,
    });
  }
  return result;
}

function inspectionTaskOperationSet(task: Task | undefined, key: string): Set<string> {
  const value = task?.result[key];
  if (!Array.isArray(value)) return new Set();
  return new Set(
    value.filter((item): item is string => typeof item === "string" && item.trim().length > 0),
  );
}

type InspectionTaskQueueState = "running" | "completed" | "queued";

function inspectionTaskQueueState(
  operation: string,
  active: Set<string>,
  completed: Set<string>,
): InspectionTaskQueueState {
  if (active.has(operation)) return "running";
  if (completed.has(operation)) return "completed";
  return "queued";
}

function inspectionTaskQueueStateMeta(state: InspectionTaskQueueState): {
  label: string;
  tone: "info" | "success" | "neutral";
  iconClass: string;
} {
  if (state === "running") {
    return {
      label: "执行中",
      tone: "info",
      iconClass: "border-primary/30 bg-primary/10 text-primary",
    };
  }
  if (state === "completed") {
    return {
      label: "已完成",
      tone: "success",
      iconClass: "border-success/30 bg-success/10 text-success",
    };
  }
  return {
    label: "排队中",
    tone: "neutral",
    iconClass: "border-border bg-muted text-muted-foreground",
  };
}

function InspectionTaskQueueStateIcon(props: { state: InspectionTaskQueueState }) {
  if (props.state === "running") {
    return <RefreshCw className="animate-spin" size={13} aria-hidden="true" />;
  }
  if (props.state === "completed") {
    return <Check size={13} aria-hidden="true" />;
  }
  return <Pause size={13} aria-hidden="true" />;
}

function AutoInspectionLiveTaskQueue(props: { task?: Task; loading?: boolean }) {
  if (!props.task) {
    return props.loading ? (
      <TaskStartupState message="正在读取本轮任务队列" />
    ) : (
      <p className="text-muted-foreground text-sm">正在生成本轮任务，任务建立后会显示执行队列。</p>
    );
  }
  const operations = inspectionTaskQueueOperations(props.task);
  const active = inspectionTaskOperationSet(props.task, "active_operations");
  const completed = inspectionTaskOperationSet(props.task, "completed_operations");
  return (
    <div className="space-y-3">
      <div className="space-y-2" role="status" aria-live="polite">
        <div className="flex min-w-0 items-center gap-2 text-sm">
          <RefreshCw className="text-primary shrink-0 animate-spin" size={15} aria-hidden="true" />
          <span className="min-w-0 flex-1 truncate">{props.task.message}</span>
          <span className="text-muted-foreground shrink-0 font-mono text-xs tabular-nums">
            {props.task.progress}%
          </span>
        </div>
        <Progress value={props.task.progress} aria-label={`自动巡检进度 ${props.task.progress}%`} />
      </div>
      {operations.length ? (
        <ol className="divide-border/70 overflow-hidden rounded-lg border divide-y">
          {operations.map((operation, index) => {
            const state = inspectionTaskQueueState(operation.operation, active, completed);
            const stateMeta = inspectionTaskQueueStateMeta(state);
            return (
              <li
                key={operation.operation}
                data-slot="inspection-task-queue-operation"
                data-state={state}
                className="flex min-w-0 items-center gap-3 px-3 py-2.5"
              >
                <span
                  className={cn(
                    "flex size-7 shrink-0 items-center justify-center rounded-full border",
                    stateMeta.iconClass,
                  )}
                >
                  <InspectionTaskQueueStateIcon state={state} />
                </span>
                <div className="min-w-0 flex-1">
                  <div className="flex min-w-0 items-center gap-2">
                    <strong className="truncate text-sm font-medium">{operation.label}</strong>
                    <span className="text-muted-foreground shrink-0 text-xs">
                      第 {index + 1} 项
                    </span>
                  </div>
                  {operation.targetCount !== null || operation.cycle ? (
                    <p className="text-muted-foreground mt-0.5 truncate text-xs">
                      {operation.targetCount !== null ? `${operation.targetCount} 个账号` : null}
                      {operation.targetCount !== null && operation.cycle ? " · " : null}
                      {operation.cycle}
                    </p>
                  ) : null}
                </div>
                <StatusPill label={stateMeta.label} tone={stateMeta.tone} />
              </li>
            );
          })}
        </ol>
      ) : (
        <p className="text-muted-foreground text-sm">本轮任务正在准备执行计划。</p>
      )}
    </div>
  );
}

function inspectionOperationDetail(operation: string, task?: Task): string {
  if (operation === "account_rate_sync") {
    const accountRates = inspectionTaskResultObject(task, "account_rate_sync");
    if (accountRates) {
      const requested = inspectionResultCount(accountRates, "requested");
      const updated = inspectionResultCount(accountRates, "updated");
      const unchanged = inspectionResultCount(accountRates, "unchanged");
      const missing = inspectionResultCount(accountRates, "missing");
      const failed = inspectionResultCount(accountRates, "failed");
      if (
        requested !== null &&
        updated !== null &&
        unchanged !== null &&
        missing !== null &&
        failed !== null
      ) {
        return `检查 ${requested} 个绑定账号，更新 ${updated} 个、无需更新 ${unchanged} 个、缺失 ${missing} 个、失败 ${failed} 个`;
      }
    }
  }
  if (
    operation === "traffic_refresh" ||
    operation === "active_probe" ||
    operation === "evidence_collection"
  ) {
    const evidence = inspectionTaskResultObject(task, "evidence");
    if (evidence) {
      const accounts = inspectionResultCount(evidence, "monitored_accounts");
      const traffic = inspectionResultCount(evidence, "traffic_persisted");
      const probes = inspectionResultCount(evidence, "probes_persisted");
      if (accounts !== null && traffic !== null && probes !== null) {
        return `监控 ${accounts} 个账号，新增 ${traffic} 条流量样本、${probes} 条探测样本`;
      }
    }
  }
  if (operation === "routing_calculation" || operation === "inspection_calculation") {
    const routing = inspectionTaskResultObject(task, "routing");
    if (routing) {
      const accounts = inspectionResultCount(routing, "accounts");
      const groups = inspectionResultCount(routing, "groups");
      const fused = inspectionResultCount(routing, "newly_fused");
      const recovered = inspectionResultCount(routing, "recovered");
      const degraded = inspectionResultCount(routing, "degraded");
      if (
        accounts !== null &&
        groups !== null &&
        fused !== null &&
        recovered !== null &&
        degraded !== null
      ) {
        return `计算 ${accounts} 个账号、${groups} 个分组，新增熔断 ${fused} 个、恢复 ${recovered} 个、降级 ${degraded} 个`;
      }
    }
  }
  if (operation === "routing_writeback") {
    const writeback = inspectionTaskResultObject(task, "writeback");
    if (writeback) {
      const changed = inspectionResultCount(writeback, "changed");
      const succeeded = inspectionResultCount(writeback, "succeeded");
      const failed = inspectionResultCount(writeback, "failed");
      if (changed !== null && succeeded !== null && failed !== null) {
        return `变更 ${changed} 个账号，执行成功 ${succeeded} 个、失败 ${failed} 个`;
      }
    }
  }
  if (operation === "alert_evaluation") {
    const alert = inspectionTaskResultObject(task, "alert_evaluation");
    const findings = alert ? inspectionResultCount(alert, "findings") : null;
    if (findings !== null) return `发现 ${findings} 项异常`;
  }
  return autoInspectionOperationDescriptions[operation] ?? "完成本步骤的巡检处理。";
}

function formatAutoInspectionStepTime(
  checkedAt: string,
  elapsedSeconds: number,
  operationStartedAt?: string | null,
): string {
  const explicitStarted = operationStartedAt ? new Date(operationStartedAt).getTime() : Number.NaN;
  const started = Number.isFinite(explicitStarted)
    ? explicitStarted
    : new Date(checkedAt).getTime();
  if (!Number.isFinite(started)) return "--:--:--";
  const fallbackOffset = Number.isFinite(explicitStarted) ? 0 : elapsedSeconds * 1000;
  return new Date(started + fallbackOffset).toLocaleTimeString("zh-CN", {
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  });
}

function inspectionTimingsOverlap(
  current: { started_at?: string | null; duration_seconds: number | null },
  other: { started_at?: string | null; duration_seconds: number | null },
): boolean {
  if (!current.started_at || !other.started_at) return false;
  const currentStarted = new Date(current.started_at).getTime();
  const otherStarted = new Date(other.started_at).getTime();
  if (!Number.isFinite(currentStarted) || !Number.isFinite(otherStarted)) return false;
  const currentEnded = currentStarted + Math.max(current.duration_seconds ?? 0, 0) * 1000;
  const otherEnded = otherStarted + Math.max(other.duration_seconds ?? 0, 0) * 1000;
  return currentStarted < otherEnded && otherStarted < currentEnded;
}

function AutoInspectionOperationTimeline(props: {
  record: AutoInspectionStatus["heartbeat_history"][number];
  task?: Task;
  upstreamSync?: InspectionUpstreamSyncSummary | null;
  upstreamSyncLoading?: boolean;
}) {
  const operationTimings = props.record.operation_timings ?? [];
  const operations = props.record.operations ?? [];
  const timings = operationTimings.length
    ? operationTimings
    : operations.map((operation) => ({
        operation,
        duration_seconds: null,
        started_at: null,
      }));
  if (!timings.length) {
    return props.record.status === "running" ? "正在检查到期任务" : "未发现到期任务";
  }
  let elapsedSeconds = 0;
  return (
    <ol data-slot="operation-timing-list" className="py-0.5">
      {timings.map((timing, index) => {
        const label = autoInspectionOperationLabels[timing.operation] ?? timing.operation;
        const stepTime = formatAutoInspectionStepTime(
          props.record.checked_at,
          elapsedSeconds,
          timing.started_at,
        );
        const runsInParallel = timings.some(
          (other, otherIndex) => otherIndex !== index && inspectionTimingsOverlap(timing, other),
        );
        elapsedSeconds += timing.duration_seconds ?? 0;
        return (
          <li
            key={`${timing.operation}:${index}`}
            data-slot="heartbeat-step"
            className="grid grid-cols-[4.5rem_1.75rem_minmax(0,1fr)] gap-x-3 pb-3 last:pb-0"
          >
            <time
              data-slot="heartbeat-step-time"
              className="text-muted-foreground self-center text-right font-mono text-[11px] tabular-nums"
              dateTime={timing.started_at ?? props.record.checked_at}
            >
              {stepTime}
            </time>
            <div
              data-slot="heartbeat-step-marker"
              className="relative flex items-center justify-center"
            >
              {timings.length > 1 ? (
                <span
                  data-slot="heartbeat-timeline-connector"
                  className={cn(
                    "bg-border absolute left-1/2 w-px -translate-x-1/2",
                    index === 0 ? "top-1/2" : "-top-3",
                    index === timings.length - 1 ? "bottom-1/2" : "-bottom-3",
                  )}
                  aria-hidden="true"
                />
              ) : null}
              <span className="border-primary/30 bg-background text-primary relative z-10 flex size-7 items-center justify-center rounded-full border">
                <Check size={13} aria-hidden="true" />
              </span>
            </div>
            <div
              data-slot="heartbeat-step-card"
              className="border-border/70 bg-card min-w-0 rounded-lg border px-3 py-2.5"
            >
              <div className="flex flex-wrap items-center justify-between gap-2">
                <strong className="text-primary font-medium">{label}</strong>
                <span className="text-muted-foreground rounded-full border px-1.5 py-0.5 text-[11px]">
                  {runsInParallel ? "并行" : `第 ${index + 1} 步`}
                </span>
              </div>
              <p className="text-muted-foreground mt-1 text-xs leading-5">
                {inspectionOperationDetail(timing.operation, props.task)}
              </p>
              {timing.operation === autoInspectionUpstreamSyncOperation && props.upstreamSync ? (
                <div className="text-muted-foreground mt-1 grid gap-0.5 text-xs leading-5">
                  <span>同步内容：上游目录、共享余额</span>
                  <span>
                    上游：共 {props.upstreamSync.upstreamTotal} 个，成功{" "}
                    {props.upstreamSync.upstreamSucceeded} 个，失败{" "}
                    {props.upstreamSync.upstreamFailed} 个
                  </span>
                </div>
              ) : null}
              {timing.operation === autoInspectionUpstreamSyncOperation &&
              props.upstreamSyncLoading ? (
                <span className="text-muted-foreground mt-1 block text-xs">正在读取同步统计…</span>
              ) : null}
              <div className="text-muted-foreground mt-2 flex flex-wrap items-center gap-x-4 gap-y-1 border-t pt-2 text-xs">
                <span>
                  耗时：
                  <strong className="text-foreground font-mono font-medium tabular-nums">
                    {timing.duration_seconds === null
                      ? "未记录"
                      : formatDurationSeconds(timing.duration_seconds)}
                  </strong>
                </span>
              </div>
            </div>
          </li>
        );
      })}
    </ol>
  );
}

function autoInspectionOperationSummary(
  record: AutoInspectionStatus["heartbeat_history"][number],
): string {
  const operationSource = (record.operation_timings ?? []).length
    ? (record.operation_timings ?? []).map((timing) => timing.operation)
    : (record.operations ?? []);
  const operations = [...new Set(operationSource)].map(
    (operation) => autoInspectionOperationLabels[operation] ?? operation,
  );
  if (operations.length) return operations.join("、");
  if (record.status === "running") return "正在检查到期任务";
  return "本轮仅检查任务是否到期，未执行其他操作";
}

export function AutoInspectionHeartbeatDetails(props: {
  record: AutoInspectionStatus["heartbeat_history"][number];
  task?: Task;
  taskLoading?: boolean;
  upstreams?: UpstreamSummary;
}) {
  const state = autoInspectionHeartbeatState(props.record);
  const upstreamSync = inspectionUpstreamSyncSummary(props.task, props.upstreams);
  return (
    <div className="min-h-0 space-y-4 overflow-x-hidden overflow-y-auto pr-1">
      <section aria-labelledby="heartbeat-summary-title" className="grid gap-2">
        <h3 id="heartbeat-summary-title" className="text-sm font-medium">
          巡检概况
        </h3>
        <dl className="grid gap-3 rounded-lg border p-3 text-sm sm:grid-cols-3">
          <div>
            <dt className="text-muted-foreground text-xs">结果</dt>
            <dd className="mt-1">
              <StatusPill label={state.label} tone={state.tone} />
            </dd>
          </div>
          <div>
            <dt className="text-muted-foreground text-xs">检查时间</dt>
            <dd className="mt-1">{formatDate(props.record.checked_at, true)}</dd>
          </div>
          <div>
            <dt className="text-muted-foreground text-xs">总耗时</dt>
            <dd className="mt-1 font-mono text-xs tabular-nums">
              {props.record.status === "running"
                ? "正在执行"
                : formatAutoInspectionDuration(
                    props.record.checked_at,
                    props.record.completed_at,
                    0,
                  )}
            </dd>
          </div>
        </dl>
      </section>

      {props.record.status === "running" ? (
        <section aria-labelledby="heartbeat-live-queue-title" className="grid gap-2">
          <h3 id="heartbeat-live-queue-title" className="text-sm font-medium">
            任务执行队列
          </h3>
          <AutoInspectionLiveTaskQueue task={props.task} loading={props.taskLoading} />
        </section>
      ) : null}

      <section aria-labelledby="heartbeat-operations-title" className="grid gap-2">
        <h3 id="heartbeat-operations-title" className="text-sm font-medium">
          执行时间线
        </h3>
        <div className="min-w-0">
          <AutoInspectionOperationTimeline
            record={props.record}
            task={props.task}
            upstreamSync={upstreamSync}
            upstreamSyncLoading={props.taskLoading && upstreamSync === null}
          />
        </div>
      </section>

      {props.record.error ? (
        <section aria-labelledby="heartbeat-error-title" className="grid gap-2">
          <h3 id="heartbeat-error-title" className="text-sm font-medium">
            失败原因
          </h3>
          <div className="border-destructive/25 bg-destructive/5 text-destructive rounded-lg border p-3 text-sm leading-6 whitespace-pre-wrap break-words">
            {props.record.error}
          </div>
        </section>
      ) : null}
    </div>
  );
}

export function AutoInspectionQueueDetails(props: { item: AutoInspectionStatus["queue"][number] }) {
  const state = autoInspectionQueueState(props.item);
  const operations = props.item.operations ?? [];
  return (
    <div className="min-h-0 space-y-4 overflow-x-hidden overflow-y-auto pr-1">
      <section aria-labelledby="queue-detail-summary-title" className="grid gap-2">
        <h3 id="queue-detail-summary-title" className="text-sm font-medium">
          任务概况
        </h3>
        <dl className="grid gap-3 rounded-lg border p-3 text-sm sm:grid-cols-3">
          <div>
            <dt className="text-muted-foreground text-xs">状态</dt>
            <dd className="mt-1">
              <StatusPill label={state.label} tone={state.tone} />
            </dd>
          </div>
          <div>
            <dt className="text-muted-foreground text-xs">计划时间</dt>
            <dd className="mt-1">
              {props.item.scheduled_for ? formatDate(props.item.scheduled_for, true) : "等待排期"}
            </dd>
          </div>
          <div>
            <dt className="text-muted-foreground text-xs">目标范围</dt>
            <dd className="mt-1">
              {props.item.target_count !== null
                ? `${props.item.target_count} 个账号`
                : "按当前策略确定"}
            </dd>
          </div>
        </dl>
        <p className="text-muted-foreground text-xs leading-5">{props.item.detail}</p>
      </section>

      <section aria-labelledby="queue-detail-operations-title" className="grid gap-2">
        <h3 id="queue-detail-operations-title" className="text-sm font-medium">
          执行计划
        </h3>
        {operations.length ? (
          <ol className="py-0.5">
            {operations.map((operation, index) => {
              const operationDue = operation.due ?? props.item.state === "ready";
              return (
                <li
                  key={operation.operation}
                  data-slot="queue-detail-operation"
                  className="grid grid-cols-[1.75rem_minmax(0,1fr)] gap-x-3 pb-3 last:pb-0"
                >
                  <div className="relative flex justify-center">
                    {index < operations.length - 1 ? (
                      <span
                        className="bg-border absolute top-7 -bottom-3 left-1/2 w-px -translate-x-1/2"
                        aria-hidden="true"
                      />
                    ) : null}
                    <span
                      className={cn(
                        "relative z-10 mt-1 flex size-7 items-center justify-center rounded-full border",
                        operationDue
                          ? "border-primary/30 bg-background text-primary"
                          : "border-border bg-muted text-muted-foreground",
                      )}
                    >
                      {operationDue ? (
                        <Check size={13} aria-hidden="true" />
                      ) : (
                        <Pause size={13} aria-hidden="true" />
                      )}
                    </span>
                  </div>
                  <div className="border-border/70 bg-card min-w-0 rounded-lg border px-3 py-2.5">
                    <div className="flex flex-wrap items-center justify-between gap-2">
                      <strong className={operationDue ? "text-primary font-medium" : "font-medium"}>
                        {operation.label}
                      </strong>
                      <StatusPill
                        label={operationDue ? "本轮执行" : "本轮不执行"}
                        tone={operationDue ? "info" : "neutral"}
                      />
                    </div>
                    <p className="text-muted-foreground mt-1 text-xs leading-5">
                      {autoInspectionOperationDescriptions[operation.operation] ??
                        "按照当前巡检策略执行该项操作。"}
                    </p>
                    <div className="text-muted-foreground mt-2 grid gap-1 border-t pt-2 text-xs sm:grid-cols-2">
                      <span>
                        执行周期：
                        <strong className="text-foreground font-medium">
                          {operation.cycle || "继承调度策略"}
                        </strong>
                      </span>
                      <span>
                        目标：
                        <strong className="text-foreground font-medium">
                          {operation.target_count !== null
                            ? `${operation.target_count} 个账号`
                            : "按执行时数据确定"}
                        </strong>
                      </span>
                    </div>
                  </div>
                </li>
              );
            })}
          </ol>
        ) : (
          <div className="text-muted-foreground rounded-lg border p-3 text-sm">
            本轮没有可执行操作
          </div>
        )}
      </section>
    </div>
  );
}

function AutoInspectionCard() {
  const status = useQuery({
    queryKey: ["auto-inspection"],
    queryFn: api.autoInspection,
    refetchInterval: 15_000,
  });
  const [draft, setDraft] = useState<AutoInspectionConfig | null>(null);
  const [clearHistoryOpen, setClearHistoryOpen] = useState(false);
  const [selectedHeartbeat, setSelectedHeartbeat] = useState<
    AutoInspectionStatus["heartbeat_history"][number] | null
  >(null);
  const [selectedQueueItem, setSelectedQueueItem] = useState<
    AutoInspectionStatus["queue"][number] | null
  >(null);
  const heartbeatTask = useQuery({
    queryKey: ["auto-inspection-heartbeat-task", selectedHeartbeat?.task_id],
    queryFn: () => api.task(selectedHeartbeat!.task_id!),
    enabled: Boolean(selectedHeartbeat?.task_id),
    retry: false,
    refetchInterval: taskPollInterval,
  });
  const heartbeatUpstreams = useQuery({
    queryKey: ["upstreams"],
    queryFn: api.upstreams,
    enabled: selectedHeartbeat !== null,
    staleTime: 30_000,
  });
  const save = useMutation({
    mutationFn: api.updateAutoInspection,
    onSuccess: (value) => {
      setDraft(autoInspectionConfig(value));
      toast.success(value.enabled ? "自动巡检已开启" : "自动巡检已关闭");
      void status.refetch();
    },
    onError: (error) =>
      toast.error(error instanceof Error ? error.message : "自动巡检设置保存失败"),
  });
  const clearHistory = useMutation({
    mutationFn: api.clearAutoInspectionHistory,
    onSuccess: async (result) => {
      setClearHistoryOpen(false);
      await status.refetch();
      toast.success(`已清空 ${result.deleted} 条心跳记录`);
    },
    onError: (error) => toast.error(error instanceof Error ? error.message : "心跳记录清空失败"),
  });
  useEffect(() => {
    if (status.data) {
      setDraft((current) => current ?? autoInspectionConfig(status.data!));
    }
  }, [status.data]);
  useEffect(() => {
    if (!status.data) return;
    setSelectedHeartbeat((current) => {
      if (!current) return current;
      return (
        status.data?.heartbeat_history.find((record) => record.checked_at === current.checked_at) ??
        current
      );
    });
  }, [status.data]);
  const current = draft ?? (status.data ? autoInspectionConfig(status.data) : null);
  const hasRunningHeartbeat = status.data?.running === true;
  const hasLiveSchedule = status.data?.enabled === true && status.data.next_run_at !== null;
  const [durationClock, setDurationClock] = useState(() => Date.now());
  useEffect(() => {
    if (!hasRunningHeartbeat && !hasLiveSchedule) return;
    setDurationClock(Date.now());
    const timer = window.setInterval(() => setDurationClock(Date.now()), 1_000);
    return () => window.clearInterval(timer);
  }, [hasLiveSchedule, hasRunningHeartbeat]);
  const intervalValid =
    current !== null &&
    Number.isInteger(current.interval_seconds) &&
    current.interval_seconds >= 15 &&
    current.interval_seconds <= 86400;
  return (
    <PageLayout>
      <PageHeading
        eyebrow="INSPECTION / AUTOMATION"
        title="自动巡检"
        description="查看下一次执行任务和心跳记录；主动探测、倍率同步与告警检测分别继承各自策略。"
        action={
          <PageActions>
            <Button
              aria-label="保存自动巡检"
              onClick={() => current && save.mutate(current)}
              disabled={!current || !intervalValid || save.isPending}
            >
              <Save size={16} />
              <span className="hidden sm:inline">
                {save.isPending ? "保存中…" : "保存自动巡检"}
              </span>
            </Button>
          </PageActions>
        }
      />
      <div className="w-full space-y-4" data-testid="auto-inspection-layout">
        <Card size="sm">
          <CardHeader>
            <CardTitle>巡检服务</CardTitle>
            <CardDescription>
              这里只控制后台服务与心跳；各任务执行周期统一在调度策略中配置
            </CardDescription>
          </CardHeader>
          <CardContent>
            {status.error && (
              <QueryError error={status.error} fallback="自动巡检状态读取失败" embedded />
            )}
            {status.isLoading && !current ? <Skeleton className="h-36 w-full" /> : null}
            {current ? (
              <div className="grid gap-3 xl:grid-cols-2" data-testid="auto-inspection-settings">
                <PolicySwitchRow
                  label="启用自动巡检"
                  description="开启后由后台持续检查到期任务，不依赖浏览器登录状态。"
                  checked={current.enabled}
                  disabled={save.isPending}
                  onCheckedChange={(enabled) => setDraft({ ...current, enabled })}
                />
                <SettingsControlRow
                  title="调度心跳"
                  description="每次心跳只检查任务是否到期，不会直接探测全部账号。"
                >
                  <div className="flex items-center gap-2">
                    <Input
                      className="w-24"
                      type="number"
                      min={15}
                      max={86400}
                      value={current.interval_seconds}
                      aria-label="调度心跳周期"
                      onChange={(event) =>
                        setDraft({
                          ...current,
                          interval_seconds: Number(event.target.value),
                        })
                      }
                    />
                    <span className="text-muted-foreground text-sm">秒</span>
                  </div>
                </SettingsControlRow>
                {!intervalValid ? (
                  <p className="text-destructive text-xs xl:col-span-2">
                    调度心跳必须为 15 到 86400 秒
                  </p>
                ) : null}
              </div>
            ) : null}
          </CardContent>
        </Card>

        <Card size="sm" data-testid="last-inspection-summary">
          <CardHeader className="gap-2 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-center">
            <div className="min-w-0">
              <CardTitle>上一轮概要</CardTitle>
              <CardDescription>
                {status.data?.last_run_at
                  ? `执行时间：${formatDate(status.data.last_run_at, true)}`
                  : "自动巡检完成一轮后会在这里显示汇总"}
              </CardDescription>
            </div>
            {status.data ? (
              <div className="flex flex-wrap items-center gap-2 sm:justify-end">
                <StatusPill
                  label={autoInspectionLastRunState(status.data.last_status).label}
                  tone={autoInspectionLastRunState(status.data.last_status).tone}
                />
                <span className="text-muted-foreground text-xs">
                  耗时：
                  <strong className="text-foreground font-medium">
                    {status.data.last_run_at
                      ? formatInspectionRunDuration(status.data.last_run_duration_ms)
                      : "—"}
                  </strong>
                </span>
              </div>
            ) : null}
          </CardHeader>
          {status.isLoading ? <Skeleton className="m-3 h-20 w-auto" /> : null}
          {!status.isLoading && status.data?.last_run_at ? (
            <CardContent className="p-0 group-data-[size=sm]/card:p-0">
              <div className="divide-border/70 grid grid-cols-2 divide-x divide-y sm:grid-cols-4 xl:grid-cols-8">
                <InspectionSummaryCount
                  label="受管账号"
                  value={status.data.last_summary.channels}
                />
                <InspectionSummaryCount label="主动探测" value={status.data.last_summary.probed} />
                <InspectionSummaryCount label="新增样本" value={status.data.last_summary.samples} />
                <InspectionSummaryCount label="新增熔断" value={status.data.last_summary.fused} />
                <InspectionSummaryCount
                  label="恢复回池"
                  value={status.data.last_summary.recovered}
                />
                <InspectionSummaryCount label="自动执行" value={status.data.last_summary.applied} />
                <InspectionSummaryCount
                  label="自动处置"
                  value={status.data.last_summary.cleaned_up}
                />
                <InspectionSummaryCount label="当前告警" value={status.data.last_summary.alerts} />
              </div>
            </CardContent>
          ) : null}
        </Card>

        <Card size="sm">
          <CardHeader>
            <CardTitle>任务队列</CardTitle>
            <CardDescription>操作到期后组合为一项巡检任务，并明确展示本轮包含内容</CardDescription>
          </CardHeader>
          <CardContent className="p-0 group-data-[size=sm]/card:p-0">
            {status.isLoading ? <LoadingRows columns={1} rows={4} /> : null}
            {!status.isLoading && !status.data?.queue.length ? (
              <EmptyRow text="暂无调度计划，自动巡检状态更新后会显示任务计划" />
            ) : null}
            <div data-slot="queue-list" className="divide-border/70 divide-y">
              {status.data?.queue.map((item) => {
                const state = autoInspectionQueueState(item);
                const operations = item.operations ?? [];
                let dueIndex = 0;
                return (
                  <section key={item.task_type} data-slot="queue-round">
                    <div className="grid gap-3 px-3 py-3 sm:grid-cols-[minmax(12rem,1fr)_auto_auto_auto] sm:items-center sm:px-4">
                      <div className="flex min-w-0 items-center gap-2.5">
                        <span className="bg-muted text-muted-foreground flex size-8 shrink-0 items-center justify-center rounded-md">
                          <AutoInspectionTaskIcon />
                        </span>
                        <div className="min-w-0">
                          <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
                            <strong className="font-medium">{item.label}</strong>
                            {item.target_count !== null ? (
                              <span className="text-muted-foreground text-xs">
                                {item.target_count} 个目标
                              </span>
                            ) : null}
                          </div>
                        </div>
                      </div>
                      <StatusPill label={state.label} tone={state.tone} />
                      <div className="sm:min-w-40 sm:text-right">
                        {item.scheduled_for ? (
                          <>
                            <div className="text-sm font-medium">
                              {formatAutoInspectionCountdown(item.scheduled_for, durationClock)}
                            </div>
                            <div className="text-muted-foreground mt-0.5 font-mono text-xs">
                              {formatDate(item.scheduled_for, true)}
                            </div>
                          </>
                        ) : (
                          <span className="text-muted-foreground text-sm">等待排期</span>
                        )}
                      </div>
                      {operations.length ? (
                        <TableActionButton
                          label="查看任务详情"
                          onClick={() => setSelectedQueueItem(item)}
                          className="justify-self-end"
                        >
                          <Eye />
                        </TableActionButton>
                      ) : (
                        <span className="text-muted-foreground justify-self-end text-sm">—</span>
                      )}
                    </div>

                    {operations.length ? (
                      <div data-slot="queue-operations" className="border-border/70 border-t">
                        <div className="text-muted-foreground hidden min-h-9 grid-cols-[2.5rem_minmax(10rem,1fr)_minmax(13rem,1.2fr)_7rem] items-center gap-3 px-4 text-xs font-medium sm:grid">
                          <span className="text-center">顺序</span>
                          <span>操作</span>
                          <span>执行周期</span>
                          <span className="text-right">本轮安排</span>
                        </div>
                        <ol className="divide-border/60 divide-y">
                          {operations.map((operation) => {
                            const operationDue = operation.due ?? item.state === "ready";
                            if (operationDue) dueIndex += 1;
                            return (
                              <li
                                key={operation.operation}
                                data-slot="queue-operation"
                                className="grid min-h-12 grid-cols-[2rem_minmax(0,1fr)_auto] items-center gap-x-3 gap-y-1 px-3 py-2.5 sm:grid-cols-[2.5rem_minmax(10rem,1fr)_minmax(13rem,1.2fr)_7rem] sm:px-4"
                              >
                                <span
                                  data-sequence={operationDue ? dueIndex : undefined}
                                  className={cn(
                                    "flex size-6 items-center justify-center justify-self-center rounded-full font-mono text-xs",
                                    operationDue
                                      ? "bg-primary/10 text-primary font-medium"
                                      : "bg-muted text-muted-foreground",
                                  )}
                                >
                                  {operationDue ? dueIndex : "—"}
                                </span>
                                <div className="min-w-0">
                                  <span className="block text-sm font-medium">
                                    {operation.label}
                                  </span>
                                  {operation.target_count !== null ? (
                                    <span className="text-muted-foreground mt-0.5 block text-xs">
                                      {operation.target_count} 个账号
                                    </span>
                                  ) : null}
                                </div>
                                <span className="text-muted-foreground col-span-2 col-start-2 text-xs leading-5 sm:col-span-1 sm:col-start-auto">
                                  {operation.cycle || "继承调度策略"}
                                </span>
                                <span
                                  className={cn(
                                    "row-start-1 text-right text-xs font-medium sm:row-auto",
                                    operationDue ? "text-primary" : "text-muted-foreground",
                                  )}
                                >
                                  {operationDue ? "本轮执行" : "本轮不执行"}
                                </span>
                              </li>
                            );
                          })}
                        </ol>
                      </div>
                    ) : (
                      <div className="border-border/70 text-muted-foreground border-t px-4 py-4 text-sm">
                        {item.state === "disabled"
                          ? "启用自动巡检后才会生成执行计划"
                          : "当前没有可执行操作"}
                      </div>
                    )}
                  </section>
                );
              })}
            </div>
          </CardContent>
        </Card>

        <Card size="sm">
          <CardHeader className="flex items-start justify-between gap-3">
            <div className="min-w-0">
              <CardTitle>心跳记录</CardTitle>
              <CardDescription className="mt-1">
                保留最近 20 次调度检查，包括没有任务到期的心跳
              </CardDescription>
            </div>
            <Button
              variant="outline"
              size="sm"
              disabled={
                clearHistory.isPending ||
                status.data?.running === true ||
                !status.data?.heartbeat_history.length
              }
              onClick={() => setClearHistoryOpen(true)}
            >
              <Trash2 size={15} />
              清空记录
            </Button>
          </CardHeader>
          <CardContent className="p-0 group-data-[size=sm]/card:p-0">
            <Table className="min-w-[900px]" containerClassName="min-h-0 overflow-auto">
              <TableHeader>
                <TableRow>
                  <TableHead className="w-[18%]">检查时间</TableHead>
                  <TableHead className="w-[10%]">结果</TableHead>
                  <TableHead className="w-[10%]">总耗时</TableHead>
                  <TableHead className="w-[32%]">执行概况</TableHead>
                  <TableHead className="w-[24%]">错误</TableHead>
                  <TableHead className="w-[6%] text-right">操作</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {status.isLoading && <TableLoadingRows columns={6} />}
                {!status.isLoading && !status.data?.heartbeat_history.length ? (
                  <TableMessageRow columns={6}>
                    <EmptyRow text="暂无心跳记录，启用自动巡检后会记录每次调度检查" />
                  </TableMessageRow>
                ) : null}
                {status.data?.heartbeat_history.map((record) => (
                  <TableRow key={`${record.checked_at}:${record.task_id ?? "skipped"}`}>
                    <TableCell>{formatDate(record.checked_at, true)}</TableCell>
                    <TableCell>
                      <StatusPill {...autoInspectionHeartbeatState(record)} />
                    </TableCell>
                    <TableCell className="font-mono text-xs tabular-nums">
                      <span
                        data-live-duration={record.status === "running" ? true : undefined}
                        suppressHydrationWarning={record.status === "running"}
                      >
                        {formatAutoInspectionDuration(
                          record.checked_at,
                          record.completed_at,
                          durationClock,
                        )}
                      </span>
                    </TableCell>
                    <TableCell tooltipContent={autoInspectionOperationSummary(record)}>
                      {autoInspectionOperationSummary(record)}
                    </TableCell>
                    <TableCell
                      className={record.error ? "text-destructive" : undefined}
                      tooltipContent={record.error ?? "—"}
                    >
                      {record.error ?? "—"}
                    </TableCell>
                    <TableCell className="text-right" overflowTooltip={false}>
                      {record.error || record.task_id || (record.operation_timings ?? []).length ? (
                        <TableActionButton
                          label="查看心跳详情"
                          onClick={() => setSelectedHeartbeat(record)}
                        >
                          <Eye />
                        </TableActionButton>
                      ) : (
                        "—"
                      )}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </CardContent>
        </Card>
      </div>
      <Dialog
        open={clearHistoryOpen}
        onOpenChange={(open) => {
          if (!clearHistory.isPending) setClearHistoryOpen(open);
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>清空心跳记录</DialogTitle>
            <DialogDescription>
              将删除全部自动巡检心跳历史，不会修改巡检开关、调度周期或各任务的到期时间。
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button
              variant="outline"
              disabled={clearHistory.isPending}
              onClick={() => setClearHistoryOpen(false)}
            >
              取消
            </Button>
            <Button
              variant="destructive"
              disabled={clearHistory.isPending}
              onClick={() => clearHistory.mutate()}
            >
              <Trash2 size={15} />
              {clearHistory.isPending ? "清空中…" : "确认清空"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      <Dialog
        open={selectedQueueItem !== null}
        onOpenChange={(open) => {
          if (!open) setSelectedQueueItem(null);
        }}
      >
        <DialogContent
          width="wide"
          height="tall"
          className="grid grid-rows-[auto_minmax(0,1fr)] overflow-hidden"
        >
          <DialogHeader>
            <DialogTitle>巡检任务详情</DialogTitle>
            <DialogDescription>
              {selectedQueueItem
                ? `${selectedQueueItem.label} · ${autoInspectionQueueState(selectedQueueItem).label}`
                : "查看下一轮巡检计划"}
            </DialogDescription>
          </DialogHeader>
          <DialogBody className="overflow-hidden pr-0">
            {selectedQueueItem ? <AutoInspectionQueueDetails item={selectedQueueItem} /> : null}
          </DialogBody>
        </DialogContent>
      </Dialog>
      <Dialog
        open={selectedHeartbeat !== null}
        onOpenChange={(open) => {
          if (!open) setSelectedHeartbeat(null);
        }}
      >
        <DialogContent
          width="wide"
          height="tall"
          className="grid grid-rows-[auto_minmax(0,1fr)] overflow-hidden"
        >
          <DialogHeader>
            <DialogTitle>巡检心跳详情</DialogTitle>
            <DialogDescription>
              {selectedHeartbeat
                ? `${formatDate(selectedHeartbeat.checked_at, true)} · ${autoInspectionHeartbeatState(selectedHeartbeat).label}`
                : "查看本轮巡检执行信息"}
            </DialogDescription>
          </DialogHeader>
          <DialogBody className="overflow-hidden pr-0">
            {selectedHeartbeat ? (
              <AutoInspectionHeartbeatDetails
                record={selectedHeartbeat}
                task={heartbeatTask.data}
                taskLoading={heartbeatTask.isLoading || heartbeatUpstreams.isLoading}
                upstreams={heartbeatUpstreams.data}
              />
            ) : null}
          </DialogBody>
        </DialogContent>
      </Dialog>
    </PageLayout>
  );
}

export function AutoInspectionPage() {
  return <AutoInspectionCard />;
}

const runtimeModeOptions: ReadonlyArray<{
  value: RuntimeMode;
  description: string;
}> = [
  {
    value: "监控模式",
    description:
      "只读取上游与真实流量数据并执行巡检告警，不主动探测、不保存调度结果、不写入 Sub2API",
  },
  {
    value: "完全模式",
    description: "采集上游与流量数据，计算并保存调度结果，再按策略应用到 Sub2API",
  },
];

function PolicyPageLoading() {
  return (
    <div className="flex flex-col gap-4" data-testid="policy-loading" aria-label="正在加载调度策略">
      <Card size="sm">
        <CardHeader>
          <Skeleton className="h-5 w-24" />
          <Skeleton className="h-4 w-full max-w-md" />
        </CardHeader>
        <CardContent>
          <Skeleton className="h-14 w-full" />
        </CardContent>
      </Card>
      <Card size="sm">
        <CardHeader>
          <Skeleton className="h-5 w-32" />
          <Skeleton className="h-4 w-full max-w-lg" />
        </CardHeader>
        <CardContent className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          <Skeleton className="h-16 w-full" />
          <Skeleton className="h-16 w-full" />
          <Skeleton className="h-16 w-full" />
        </CardContent>
      </Card>
      <Card size="sm">
        <CardHeader>
          <Skeleton className="h-5 w-28" />
          <Skeleton className="h-4 w-full max-w-sm" />
        </CardHeader>
        <CardContent className="grid gap-4 sm:grid-cols-2">
          <Skeleton className="h-16 w-full" />
          <Skeleton className="h-16 w-full" />
        </CardContent>
      </Card>
    </div>
  );
}

export function PolicyPage() {
  const policy = useQuery({
    queryKey: ["policy"],
    queryFn: api.policy,
    refetchInterval: 30_000,
    staleTime: 30_000,
  });
  const config = useQuery({ queryKey: ["config"], queryFn: api.config });
  const queryClient = useQueryClient();
  const [draft, setDraft] = useState<PolicyDraft | null>(null);
  const [category, setCategory] = useState<"operations" | "rules" | "scope">("operations");
  const [dangerousSaveOpen, setDangerousSaveOpen] = useState(false);
  const [restoreControlOpen, setRestoreControlOpen] = useState(false);
  const save = useMutation({
    mutationFn: (payload: PolicyUpdate) => api.updatePolicy(payload),
    onSuccess: (value) => {
      queryClient.setQueryData(["policy"], value);
      setDraft(policyDraft(value));
      void Promise.all([
        queryClient.invalidateQueries({ queryKey: ["config"] }),
        queryClient.invalidateQueries({ queryKey: ["groups"] }),
        queryClient.invalidateQueries({ queryKey: ["logs"] }),
        queryClient.invalidateQueries({ queryKey: ["overview"] }),
      ]);
      toast.success("策略已保存，将用于下一轮调度");
    },
    onError: (error) => {
      const message = error instanceof Error ? error.message : "策略保存失败";
      toast.error(message);
    },
  });
  const restoreControl = useMutation({
    mutationFn: api.restorePolicyControl,
    onSuccess: (result) => {
      setRestoreControlOpen(false);
      void Promise.all([
        queryClient.invalidateQueries({ queryKey: ["accounts"] }),
        queryClient.invalidateQueries({ queryKey: ["groups"] }),
        queryClient.invalidateQueries({ queryKey: ["logs"] }),
        queryClient.invalidateQueries({ queryKey: ["overview"] }),
      ]);
      if (result.failed) {
        toast.error(`交还完成：成功 ${result.restored}，失败 ${result.failed}`);
      } else {
        toast.success(`已交还控制权，恢复 ${result.restored} 个账号`);
      }
    },
    onError: (error) => toast.error(error instanceof Error ? error.message : "交还控制权失败"),
  });
  const updateProbes = useMutation({
    mutationFn: (enabled: boolean) => api.setProbesEnabled(enabled),
    onSuccess: (value) => {
      queryClient.setQueryData(["config"], value);
      setDraft((current) =>
        current
          ? withPolicyAdvancedValue(current, "probe", "enabled", value.probes_enabled)
          : current,
      );
      void queryClient.invalidateQueries({ queryKey: ["policy"] });
      toast.success(value.probes_enabled ? "主动探测已启用" : "主动探测已关闭");
    },
    onError: (error) => toast.error(error instanceof Error ? error.message : "主动探测设置失败"),
  });
  const data = policy.data;
  useEffect(() => {
    if (data) {
      setDraft((current) => current ?? policyDraft(data));
    }
  }, [data]);
  const current = policy.error ? null : (draft ?? (data ? policyDraft(data) : null));
  const payload = current ? policyPayload(current) : null;
  const relationshipError = current ? policyRelationshipError(current) : null;
  const cleanup = payload?.advanced_policy?.cleanup;
  const cleanupConfig =
    cleanup !== null && typeof cleanup === "object" && !Array.isArray(cleanup)
      ? (cleanup as Record<string, unknown>)
      : null;
  const destructiveCleanup = cleanupConfig?.enabled === true && cleanupConfig.action === "delete";
  const activeRuntimeMode = current?.mode;
  const runtimeModeDescription =
    runtimeModeOptions.find((option) => option.value === activeRuntimeMode)?.description ??
    "运行配置不可用";
  function submitPolicy() {
    if (!payload) return;
    if (destructiveCleanup) {
      setDangerousSaveOpen(true);
      return;
    }
    save.mutate(payload);
  }
  async function refreshPolicy() {
    const result = await policy.refetch();
    if (result.data) {
      setDraft(policyDraft(result.data));
    }
  }
  return (
    <PageLayout>
      <PageHeading
        eyebrow="SCHEDULING / POLICY"
        title="调度策略"
        description="设置全局默认、健康判定、熔断回池和自动执行范围；分组覆盖在分组管理中配置。"
        action={
          <PageActions>
            <Button variant="outline" onClick={() => void refreshPolicy()}>
              <RefreshCw size={16} />
              刷新策略
            </Button>
            <Button disabled={!payload || save.isPending} onClick={submitPolicy}>
              <ShieldCheck size={16} />
              {save.isPending ? "保存中…" : "保存策略"}
            </Button>
          </PageActions>
        }
      />
      <div className="w-full space-y-4">
        {data?.configuration_errors?.length ? (
          <div className="border-warning/40 bg-warning/10 text-warning rounded-lg border px-3 py-2 text-sm">
            策略配置存在无效值：{data.configuration_errors.join("、")}
            。已停止使用这些字段的默认值，请修正后保存。
          </div>
        ) : null}
        {current && !payload && !data?.configuration_errors?.length ? (
          <div className="border-warning/40 bg-warning/10 text-warning rounded-lg border px-3 py-2 text-sm">
            {relationshipError ?? "策略参数存在空值或无效数字"}
            ，修正后才能保存；不会自动填充默认值。
          </div>
        ) : null}
        {policy.error ? <QueryError error={policy.error} fallback="调度策略读取失败" /> : null}
        <div className="bg-muted flex w-fit gap-1 rounded-lg p-1">
          {(
            [
              ["operations", "调度与执行"],
              ["rules", "健康判定"],
              ["scope", "守护范围"],
            ] as const
          ).map(([value, label]) => (
            <Button
              key={value}
              size="sm"
              variant={category === value ? "secondary" : "ghost"}
              aria-pressed={category === value}
              onClick={() => setCategory(value)}
            >
              {label}
            </Button>
          ))}
        </div>
        {policy.isLoading && !current ? <PolicyPageLoading /> : null}
        {category === "operations" && current ? (
          <div className="flex flex-col gap-4" data-testid="policy-operations-layout">
            <Card size="sm">
              <CardHeader>
                <CardTitle>运行控制</CardTitle>
                <CardDescription>
                  选择监控告警、仅保存调度结果或自动应用到 Sub2API；随“保存策略”一起生效
                </CardDescription>
              </CardHeader>
              <CardContent>
                <SettingsControlRow title="执行模式" description={runtimeModeDescription}>
                  <div className="flex flex-wrap items-center justify-end gap-2">
                    {data?.mode !== current.mode ? (
                      <StatusPill label="待保存" tone="warning" />
                    ) : null}
                    <div
                      className="bg-muted flex flex-wrap justify-end gap-1 rounded-lg p-1"
                      role="group"
                      aria-label="执行模式"
                      data-testid="policy-runtime-modes"
                    >
                      {runtimeModeOptions.map((option) => (
                        <Tooltip key={option.value}>
                          <TooltipTrigger render={<span className="inline-flex" />}>
                            <Button
                              size="sm"
                              variant={activeRuntimeMode === option.value ? "secondary" : "ghost"}
                              disabled={save.isPending}
                              aria-pressed={activeRuntimeMode === option.value}
                              aria-label={`${option.value}：${option.description}`}
                              onClick={() => setDraft({ ...current, mode: option.value })}
                            >
                              {option.value}
                            </Button>
                          </TooltipTrigger>
                          <TooltipContent>{option.description}</TooltipContent>
                        </Tooltip>
                      ))}
                    </div>
                  </div>
                </SettingsControlRow>
              </CardContent>
            </Card>
            {current && (
              <Card size="sm">
                <CardHeader>
                  <CardTitle>全局默认策略</CardTitle>
                  <CardDescription>
                    分组没有单独设置时使用；分组级选择在分组管理中配置。
                    {schedulingWeightFormula}
                  </CardDescription>
                </CardHeader>
                <CardContent className="grid gap-x-5 gap-y-4 sm:grid-cols-2 lg:grid-cols-3">
                  <FormField label="全局默认策略">
                    <Select
                      value={current.global_strategy}
                      onValueChange={(value) =>
                        value && setDraft({ ...current, global_strategy: value })
                      }
                    >
                      <SelectTrigger>
                        <SelectValue>{displayStrategy(current.global_strategy)}</SelectValue>
                      </SelectTrigger>
                      <SelectContent>
                        {schedulingStrategyOptions.map((option) => (
                          <SelectItem key={option.value} value={option.value}>
                            {option.label}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                    <span className="text-muted-foreground text-xs leading-4 font-normal">
                      {schedulingStrategyDescription(current.global_strategy)}
                    </span>
                  </FormField>
                  <FormField label="倍率缺失回退">
                    <Select
                      value={current.missing_rate_fallback}
                      onValueChange={(value) =>
                        value && setDraft({ ...current, missing_rate_fallback: value })
                      }
                    >
                      <SelectTrigger>
                        <SelectValue>{displayFallback(current.missing_rate_fallback)}</SelectValue>
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="current_cost_wall">回退当前成本墙</SelectItem>
                        <SelectItem value="fail_closed">严格关闭</SelectItem>
                        <SelectItem value="fail_open">允许继续</SelectItem>
                      </SelectContent>
                    </Select>
                  </FormField>
                  <PolicyNumberField
                    label="每组总权重预算"
                    description="由同一分组内参与调度的账号按策略共享"
                    min={1}
                    max={1000000}
                    value={policyAdvancedValue(current, "weights", "budget")}
                    onChange={(value) =>
                      setDraft(withPolicyAdvancedValue(current, "weights", "budget", value))
                    }
                  />
                  <PolicyNumberField
                    label="人工优先位范围"
                    description="保留优先级 1 至 N；自动调度从 N+1 开始"
                    min={1}
                    max={1000}
                    value={policyAdvancedValue(current, "manual_priority", "reserved_max")}
                    onChange={(value) =>
                      setDraft(
                        withPolicyAdvancedValue(current, "manual_priority", "reserved_max", value),
                      )
                    }
                  />
                  <PolicyNumberField
                    label="权重健康闸门"
                    unit="分"
                    min={0}
                    max={100}
                    step="any"
                    value={policyAdvancedValue(current, "weights", "gate_floor")}
                    onChange={(value) =>
                      setDraft(withPolicyAdvancedValue(current, "weights", "gate_floor", value))
                    }
                  />
                  <PolicyNumberField
                    label="均衡中价格占比"
                    min={0}
                    max={1}
                    value={policyAdvancedValue(current, "weights", "balanced_price_ratio")}
                    step="0.05"
                    onChange={(value) =>
                      setDraft(
                        withPolicyAdvancedValue(current, "weights", "balanced_price_ratio", value),
                      )
                    }
                  />
                </CardContent>
              </Card>
            )}
            {current ? (
              <PolicyConfigCard
                title="巡检任务周期"
                description="心跳只检查任务是否到期；上游数据会同时刷新倍率、余额和鉴权状态。"
              >
                <PolicyNumberField
                  label="上游数据拉取间隔"
                  unit="秒"
                  min={30}
                  max={86400}
                  value={policyAdvancedValue(current, "upstream_multiplier", "interval_seconds")}
                  onChange={(value) =>
                    setDraft(
                      withPolicyAdvancedValue(
                        current,
                        "upstream_multiplier",
                        "interval_seconds",
                        value,
                      ),
                    )
                  }
                />
                <PolicyNumberField
                  label="请求记录拉取间隔"
                  unit="秒"
                  min={1}
                  max={86400}
                  value={policyAdvancedValue(current, "traffic", "refresh_seconds")}
                  onChange={(value) =>
                    setDraft(withPolicyAdvancedValue(current, "traffic", "refresh_seconds", value))
                  }
                />
              </PolicyConfigCard>
            ) : null}
            {current ? (
              <PolicyOperationsEditor
                value={current}
                onChange={setDraft}
                probesEnabled={config.data?.probes_enabled}
                probesPending={updateProbes.isPending || config.isLoading}
                onProbesEnabledChange={(enabled) => updateProbes.mutate(enabled)}
              />
            ) : null}
          </div>
        ) : null}
        {category === "rules" && current ? (
          <div className="grid items-start gap-4 xl:grid-cols-2">
            <PolicyRulesEditor value={current} onChange={setDraft} />
          </div>
        ) : null}
        {category === "scope" && current ? (
          <PolicyScopeLayout
            value={current}
            onChange={setDraft}
            groups={data?.group_strategies ?? []}
            onRestoreControl={() => setRestoreControlOpen(true)}
            restorePending={restoreControl.isPending}
          />
        ) : null}
      </div>
      <Dialog open={dangerousSaveOpen} onOpenChange={setDangerousSaveOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>确认启用自动删除</DialogTitle>
            <DialogDescription>
              认证失效达到条件后将先摘除流量，再从 Sub2API 删除账号。删除后无法由 Console 重建。
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDangerousSaveOpen(false)}>
              取消
            </Button>
            <Button
              variant="destructive"
              disabled={save.isPending}
              onClick={() => {
                if (!payload) return;
                setDangerousSaveOpen(false);
                save.mutate(payload);
              }}
            >
              确认保存
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      <Dialog open={restoreControlOpen} onOpenChange={setRestoreControlOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>交还调度控制权</DialogTitle>
            <DialogDescription>
              将所有被 Console
              改动过的账号恢复为接管前的优先级、负载因子、并发与调度状态。失败项会保留基线，可再次重试。
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setRestoreControlOpen(false)}>
              取消
            </Button>
            <Button
              variant="destructive"
              disabled={restoreControl.isPending}
              onClick={() => restoreControl.mutate()}
            >
              {restoreControl.isPending ? "正在恢复…" : "恢复并交还"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </PageLayout>
  );
}

type PolicyEditorProps = {
  value: PolicyDraft;
  onChange: (value: PolicyDraft) => void;
};

type PolicyOperationsEditorProps = PolicyEditorProps & {
  probesEnabled?: boolean;
  probesPending: boolean;
  onProbesEnabledChange: (enabled: boolean) => void;
};

function policyAdvancedValue(value: PolicyDraft, section: string, path: string): unknown {
  let current: unknown = value.advanced_policy[section];
  for (const part of path.split(".")) {
    if (!current || typeof current !== "object" || Array.isArray(current)) return undefined;
    current = (current as Record<string, unknown>)[part];
  }
  return current;
}

function withPolicyAdvancedValue(
  value: PolicyDraft,
  section: string,
  path: string,
  nextValue: unknown,
): PolicyDraft {
  const source = value.advanced_policy[section];
  const sectionValue =
    source && typeof source === "object" && !Array.isArray(source)
      ? { ...(source as Record<string, unknown>) }
      : {};
  let target = sectionValue;
  const parts = path.split(".");
  parts.slice(0, -1).forEach((part) => {
    const child = target[part];
    const next =
      child && typeof child === "object" && !Array.isArray(child)
        ? { ...(child as Record<string, unknown>) }
        : {};
    target[part] = next;
    target = next;
  });
  target[parts.at(-1) ?? path] = nextValue;
  return {
    ...value,
    advanced_policy: { ...value.advanced_policy, [section]: sectionValue },
  };
}

export function withManagedGroupScope(
  value: PolicyDraft,
  mode: "all" | "selected",
  groupIDs: string[],
): PolicyDraft {
  let next = withPolicyAdvancedValue(value, "scope", "managed_group_mode", mode);
  next = withPolicyAdvancedValue(next, "scope", "managed_group_ids", groupIDs);
  return next;
}

function advancedNumber(value: unknown): string | number {
  return typeof value === "number" || typeof value === "string" ? value : "";
}

function advancedList(value: unknown): string {
  return Array.isArray(value) ? value.map(String).join(", ") : "";
}

function PolicyNumberField(props: {
  label: string;
  description?: string;
  value: unknown;
  onChange: (value: number | null) => void;
  unit?: string;
  step?: string;
  min?: number;
  max?: number;
  disabled?: boolean;
}) {
  return (
    <FormField label={props.unit ? `${props.label}（${props.unit}）` : props.label}>
      <Input
        type="number"
        step={props.step ?? "1"}
        min={props.min}
        max={props.max}
        disabled={props.disabled}
        value={advancedNumber(props.value)}
        onChange={(event) => {
          const raw = event.target.value.trim();
          const parsed = Number(raw);
          props.onChange(raw && Number.isFinite(parsed) ? parsed : null);
        }}
      />
      {props.description ? (
        <span className="text-muted-foreground text-xs leading-4 font-normal">
          {props.description}
        </span>
      ) : null}
    </FormField>
  );
}

function PolicySwitchRow(props: {
  label: string;
  description: string;
  checked: boolean;
  disabled?: boolean;
  onCheckedChange: (value: boolean) => void;
}) {
  const switchId = React.useId();
  return (
    <SettingsControlRow
      title={props.label}
      description={props.description}
      controlId={switchId}
      controlDisabled={props.disabled}
    >
      <Switch
        id={switchId}
        checked={props.checked}
        disabled={props.disabled}
        aria-label={props.label}
        onCheckedChange={props.onCheckedChange}
      />
    </SettingsControlRow>
  );
}

function PolicyConfigCard(props: {
  title: string;
  description: string;
  children: React.ReactNode;
  columns?: 2 | 3;
  wide?: boolean;
  switchAction?: {
    checked: boolean;
    label: string;
    text?: string;
    disabled?: boolean;
    onCheckedChange: (value: boolean) => void;
  };
}) {
  const switchId = React.useId();
  const heading = (
    <>
      <CardTitle>{props.title}</CardTitle>
      <CardDescription>{props.description}</CardDescription>
    </>
  );
  return (
    <Card size="sm" className={props.wide ? "xl:col-span-2" : undefined}>
      <CardHeader className="flex items-start justify-between gap-4">
        {props.switchAction ? (
          <label
            className={cn(
              "min-w-0",
              props.switchAction.disabled ? "cursor-not-allowed" : "cursor-pointer",
            )}
            htmlFor={switchId}
          >
            {heading}
          </label>
        ) : (
          <div className="min-w-0">{heading}</div>
        )}
        {props.switchAction ? (
          <div className="flex shrink-0 items-center gap-2">
            <Switch
              id={switchId}
              checked={props.switchAction.checked}
              disabled={props.switchAction.disabled}
              aria-label={props.switchAction.label}
              onCheckedChange={props.switchAction.onCheckedChange}
            />
            {props.switchAction.text ? (
              <label
                className={cn(
                  "text-muted-foreground text-sm font-medium whitespace-nowrap",
                  props.switchAction.disabled ? "cursor-not-allowed" : "cursor-pointer",
                )}
                htmlFor={switchId}
              >
                {props.switchAction.text}
              </label>
            ) : null}
          </div>
        ) : null}
      </CardHeader>
      <CardContent
        className={cn(
          "grid gap-x-4 gap-y-3 sm:grid-cols-2",
          props.columns === 3 && "lg:grid-cols-3",
        )}
      >
        {props.children}
      </CardContent>
    </Card>
  );
}

function PolicyOperationsEditor(props: PolicyOperationsEditorProps) {
  const set = (section: string, path: string, value: unknown) =>
    props.onChange(withPolicyAdvancedValue(props.value, section, path, value));
  const retryEnabled = policyAdvancedValue(props.value, "probe", "retry_enabled") === true;
  const retrySource = String(policyAdvancedValue(props.value, "probe", "retry_source") ?? "fixed");
  return (
    <>
      <PolicyConfigCard
        title="自动执行范围"
        description="关闭某一项后仍计算期望值，但不自动应用到 Sub2API。"
        wide
      >
        <div className="col-span-full grid gap-2 sm:grid-cols-2" data-testid="policy-auto-apply">
          {Object.entries(autoApplyLabels).map(([key, label]) => (
            <SettingsControlRow
              key={key}
              title={label}
              description={`控制是否自动执行${label}变更`}
              controlId={`policy-auto-apply-${key}`}
            >
              <Switch
                id={`policy-auto-apply-${key}`}
                checked={props.value.auto_apply?.[key] === true}
                aria-label={`${label}自动执行`}
                onCheckedChange={(enabled) =>
                  props.onChange({
                    ...props.value,
                    auto_apply: {
                      ...(props.value.auto_apply ?? {}),
                      [key]: enabled,
                    },
                  })
                }
              />
            </SettingsControlRow>
          ))}
          <PolicyNumberField
            label="调度写入并发"
            unit="个账号"
            min={1}
            max={16}
            value={policyAdvancedValue(props.value, "writeback", "concurrency")}
            onChange={(value) => set("writeback", "concurrency", value)}
          />
          <SettingsControlRow
            title="调度写后确认"
            description="开启后只复核自动调度实际修改的字段。"
            controlId="policy-writeback-verification"
          >
            <Switch
              id="policy-writeback-verification"
              checked={policyAdvancedValue(props.value, "writeback", "verification") === true}
              aria-label="调度写后确认"
              onCheckedChange={(value) => set("writeback", "verification", value)}
            />
          </SettingsControlRow>
        </div>
      </PolicyConfigCard>
      <PolicyConfigCard
        title="熔断"
        description="致命错误立即熔断；错误率与延迟触发软熔断，且受保底池与每轮切换上限约束。"
        columns={3}
        wide
        switchAction={{
          checked: policyAdvancedValue(props.value, "breaker", "enabled") !== false,
          label: "启用熔断",
          onCheckedChange: (value) => set("breaker", "enabled", value),
        }}
      >
        <PolicyNumberField
          label="错误率窗口"
          unit="次请求"
          min={1}
          value={policyAdvancedValue(props.value, "breaker", "http_window")}
          onChange={(value) => set("breaker", "http_window", value)}
        />
        <PolicyNumberField
          label="窗口内失败次数"
          unit="次"
          min={1}
          value={policyAdvancedValue(props.value, "breaker", "http_failures")}
          onChange={(value) => set("breaker", "http_failures", value)}
        />
        <PolicyNumberField
          label="且健康分低于"
          unit="分"
          min={0}
          max={100}
          value={policyAdvancedValue(props.value, "breaker", "http_score_below")}
          onChange={(value) => set("breaker", "http_score_below", value)}
        />
        <PolicyNumberField
          label="延迟窗口"
          unit="次请求"
          min={1}
          value={policyAdvancedValue(props.value, "breaker", "latency_window")}
          onChange={(value) => set("breaker", "latency_window", value)}
        />
        <PolicyNumberField
          label="窗口内慢响应次数"
          unit="次"
          min={1}
          value={policyAdvancedValue(props.value, "breaker", "latency_occurrences")}
          onChange={(value) => set("breaker", "latency_occurrences", value)}
        />
        <PolicyNumberField
          label="慢响应首字界限"
          unit="毫秒"
          min={1}
          value={policyAdvancedValue(props.value, "breaker", "latency_ttfb_ms")}
          onChange={(value) => set("breaker", "latency_ttfb_ms", value)}
        />
        <PolicyNumberField
          label="每轮最多熔断"
          unit="个"
          min={1}
          value={policyAdvancedValue(props.value, "breaker", "max_switch_per_round")}
          onChange={(value) => set("breaker", "max_switch_per_round", value)}
        />
        <PolicyNumberField
          label="熔断冷却"
          unit="秒"
          min={0}
          value={policyAdvancedValue(props.value, "breaker", "fused_cooldown_seconds")}
          onChange={(value) => set("breaker", "fused_cooldown_seconds", value)}
        />
        <FormField label="见到即熔断的错误码">
          <Input
            value={advancedList(
              policyAdvancedValue(props.value, "breaker", "instant_status_codes"),
            )}
            placeholder="例如 402, 429"
            onChange={(event) =>
              set(
                "breaker",
                "instant_status_codes",
                splitConfigList(event.target.value).map(Number),
              )
            }
          />
        </FormField>
        <div className="col-span-full grid gap-2 lg:grid-cols-3">
          <PolicySwitchRow
            label="凭据失效立即熔断"
            description="认证失败无需累计；仍受保底池约束。"
            checked={policyAdvancedValue(props.value, "breaker", "hard_fatal") !== false}
            onCheckedChange={(value) => set("breaker", "hard_fatal", value)}
          />
          <PolicySwitchRow
            label="网关错误只降级不熔断"
            description="临时网关异常只压低权重和优先级。"
            checked={policyAdvancedValue(props.value, "breaker", "http_degrade_only") === true}
            onCheckedChange={(value) => set("breaker", "http_degrade_only", value)}
          />
          <PolicySwitchRow
            label="延迟超标只降级不熔断"
            description="慢账号仍留在池内，但减少流量。"
            checked={policyAdvancedValue(props.value, "breaker", "latency_degrade_only") === true}
            onCheckedChange={(value) => set("breaker", "latency_degrade_only", value)}
          />
        </div>
      </PolicyConfigCard>
      <PolicyConfigCard
        title="保底与降级"
        description="保底池避免分组断供；低分账号可只降级而不停止调度。"
        columns={3}
        wide
      >
        <PolicyNumberField
          label="保底可用账号数"
          unit="个"
          min={0}
          value={policyAdvancedValue(props.value, "breaker", "min_pool_size")}
          onChange={(value) => set("breaker", "min_pool_size", value)}
        />
        <PolicyNumberField
          label="计入可用池的最低分"
          unit="分"
          min={0}
          max={100}
          value={policyAdvancedValue(props.value, "breaker", "min_pool_score")}
          onChange={(value) => set("breaker", "min_pool_score", value)}
        />
        <PolicyNumberField
          label="降级线"
          unit="分"
          min={0}
          max={100}
          value={policyAdvancedValue(props.value, "degrade", "score_threshold")}
          onChange={(value) => set("degrade", "score_threshold", value)}
        />
        <PolicyNumberField
          label="降级优先级步进"
          min={1}
          value={policyAdvancedValue(props.value, "degrade", "priority_step")}
          onChange={(value) => set("degrade", "priority_step", value)}
        />
        <PolicyNumberField
          label="降级负载乘数"
          min={0.000001}
          max={1}
          step="any"
          value={policyAdvancedValue(props.value, "degrade", "load_factor_ratio")}
          onChange={(value) => set("degrade", "load_factor_ratio", value)}
        />
        <PolicyNumberField
          label="最低负载因子"
          min={1}
          value={policyAdvancedValue(props.value, "degrade", "min_load_factor")}
          onChange={(value) => set("degrade", "min_load_factor", value)}
        />
        <div className="col-span-full">
          <PolicySwitchRow
            label="启用降级"
            description="低分账号压低权重但不停止调度。"
            checked={policyAdvancedValue(props.value, "degrade", "enabled") !== false}
            onCheckedChange={(value) => set("degrade", "enabled", value)}
          />
        </div>
      </PolicyConfigCard>
      <PolicyConfigCard
        title="健康回池"
        description="熔断账号按低频探测恢复，连续达标后重新进入调度。"
        columns={3}
        wide
        switchAction={{
          checked: policyAdvancedValue(props.value, "recovery", "enabled") !== false,
          label: "启用健康回池",
          onCheckedChange: (value) => set("recovery", "enabled", value),
        }}
      >
        <PolicyNumberField
          label="恢复探测间隔"
          unit="秒"
          min={1}
          value={policyAdvancedValue(props.value, "recovery", "probe_interval_seconds")}
          onChange={(value) => set("recovery", "probe_interval_seconds", value)}
        />
        <PolicyNumberField
          label="回池目标分"
          unit="分"
          min={0}
          max={100}
          value={policyAdvancedValue(props.value, "recovery", "target_score")}
          onChange={(value) => set("recovery", "target_score", value)}
        />
        <PolicyNumberField
          label="连续成功次数"
          unit="次"
          min={1}
          value={policyAdvancedValue(props.value, "recovery", "success_count")}
          onChange={(value) => set("recovery", "success_count", value)}
        />
        <PolicyNumberField
          label="健康持续时长"
          unit="秒"
          min={0}
          value={policyAdvancedValue(props.value, "recovery", "hold_seconds")}
          onChange={(value) => set("recovery", "hold_seconds", value)}
        />
      </PolicyConfigCard>
      <PolicyConfigCard
        title="负载因子调权"
        description="权重变化小于阈值时不执行，应用到 Sub2API 后进入冷却期，避免路由反复震荡。"
        columns={3}
        wide
        switchAction={{
          checked: policyAdvancedValue(props.value, "weights", "enabled") !== false,
          label: "启用负载因子调权",
          onCheckedChange: (value) => set("weights", "enabled", value),
        }}
      >
        <FormField label="变化阈值">
          <Input
            type="number"
            min={0.000001}
            max={1}
            step="any"
            value={props.value.change_threshold}
            onChange={(event) =>
              props.onChange({
                ...props.value,
                change_threshold: event.target.value,
              })
            }
          />
        </FormField>
        <FormField label="调权冷却（秒）">
          <Input
            type="number"
            min={0}
            max={86400}
            value={props.value.cooldown_seconds ?? ""}
            onChange={(event) =>
              props.onChange({
                ...props.value,
                cooldown_seconds: policyNumberInput(event.target.value),
              })
            }
          />
        </FormField>
        <PolicyNumberField
          label="负载因子下限"
          min={1}
          value={policyAdvancedValue(props.value, "weights", "min_load_factor")}
          onChange={(value) => set("weights", "min_load_factor", value)}
        />
        <PolicyNumberField
          label="负载因子上限"
          min={1}
          value={policyAdvancedValue(props.value, "weights", "max_load_factor")}
          onChange={(value) => set("weights", "max_load_factor", value)}
        />
        <PolicyNumberField
          label="价格权重强度"
          min={0.000001}
          max={100}
          step="any"
          value={policyAdvancedValue(props.value, "weights", "price_exp")}
          onChange={(value) => set("weights", "price_exp", value)}
        />
        <PolicyNumberField
          label="速度权重强度"
          min={0.000001}
          max={100}
          step="any"
          value={policyAdvancedValue(props.value, "weights", "speed_exp")}
          onChange={(value) => set("weights", "speed_exp", value)}
        />
      </PolicyConfigCard>
      <PolicyConfigCard
        title="认证失效自动处置"
        description="账号反复出现认证失效时自动暂停、停用或删除；默认关闭。"
        columns={3}
        wide
        switchAction={{
          checked: policyAdvancedValue(props.value, "cleanup", "enabled") === true,
          label: "启用认证失效自动处置",
          onCheckedChange: (value) => set("cleanup", "enabled", value),
        }}
      >
        <FormField label="处置动作">
          <Select
            value={String(policyAdvancedValue(props.value, "cleanup", "action") ?? "pause")}
            onValueChange={(value) => value && set("cleanup", "action", value)}
          >
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="none">仅熔断，不额外处置</SelectItem>
              <SelectItem value="pause">暂停调度</SelectItem>
              <SelectItem value="disable">停用账号</SelectItem>
              <SelectItem value="delete">删除账号</SelectItem>
            </SelectContent>
          </Select>
        </FormField>
        <PolicyNumberField
          label="判定窗口"
          unit="次样本"
          min={1}
          value={policyAdvancedValue(props.value, "cleanup", "window")}
          onChange={(value) => set("cleanup", "window", value)}
        />
        <PolicyNumberField
          label="窗口内失效次数"
          unit="次"
          min={1}
          value={policyAdvancedValue(props.value, "cleanup", "occurrences")}
          onChange={(value) => set("cleanup", "occurrences", value)}
        />
        <PolicyNumberField
          label="最短观察时长"
          unit="分钟"
          min={0}
          value={policyAdvancedValue(props.value, "cleanup", "min_fused_minutes")}
          onChange={(value) => set("cleanup", "min_fused_minutes", value)}
        />
        <PolicyNumberField
          label="每轮最多处置"
          unit="个"
          min={1}
          value={policyAdvancedValue(props.value, "cleanup", "max_per_round")}
          onChange={(value) => set("cleanup", "max_per_round", value)}
        />
        <FormField label="触发处置的错误码">
          <Input
            value={advancedList(
              policyAdvancedValue(props.value, "cleanup", "trigger_status_codes"),
            )}
            placeholder="例如 401, 403"
            onChange={(event) =>
              set(
                "cleanup",
                "trigger_status_codes",
                splitConfigList(event.target.value).map(Number),
              )
            }
          />
        </FormField>
        <div className="col-span-full grid gap-2 sm:grid-cols-2">
          <PolicySwitchRow
            label="保留分组内最后一个账号"
            description="避免自动处置清空整个分组。"
            checked={policyAdvancedValue(props.value, "cleanup", "keep_last_in_group") !== false}
            onCheckedChange={(value) => set("cleanup", "keep_last_in_group", value)}
          />
          <PolicySwitchRow
            label="仅处置凭据失效"
            description="余额不足与额度耗尽不会触发。"
            checked={policyAdvancedValue(props.value, "cleanup", "only_auth_errors") !== false}
            onCheckedChange={(value) => set("cleanup", "only_auth_errors", value)}
          />
        </div>
      </PolicyConfigCard>
      <PolicyConfigCard
        title="智能扩容"
        description="负载率达到阈值时小步提高并发，健康状态变差时按步长缩容。"
        columns={3}
        wide
        switchAction={{
          checked: policyAdvancedValue(props.value, "scaling", "enabled") === true,
          label: "启用智能扩容",
          onCheckedChange: (value) => set("scaling", "enabled", value),
        }}
      >
        <PolicyNumberField
          label="全局并发上限"
          min={1}
          max={10000000}
          value={policyAdvancedValue(props.value, "scaling", "global_max_concurrency")}
          onChange={(value) => set("scaling", "global_max_concurrency", value)}
        />
        <PolicyNumberField
          label="单账号并发下限"
          min={1}
          max={1000000}
          value={policyAdvancedValue(props.value, "scaling", "min_per_account")}
          onChange={(value) => set("scaling", "min_per_account", value)}
        />
        <PolicyNumberField
          label="单账号并发上限"
          min={1}
          max={1000000}
          value={policyAdvancedValue(props.value, "scaling", "max_per_account")}
          onChange={(value) => set("scaling", "max_per_account", value)}
        />
        <PolicyNumberField
          label="扩容触发负载率"
          min={0.000001}
          max={1}
          step="any"
          value={policyAdvancedValue(props.value, "scaling", "scale_up_ratio")}
          onChange={(value) => set("scaling", "scale_up_ratio", value)}
        />
        <PolicyNumberField
          label="扩容步长"
          min={1}
          max={1000000}
          value={policyAdvancedValue(props.value, "scaling", "step_up")}
          onChange={(value) => set("scaling", "step_up", value)}
        />
        <PolicyNumberField
          label="缩容步长"
          min={1}
          max={1000000}
          value={policyAdvancedValue(props.value, "scaling", "step_down")}
          onChange={(value) => set("scaling", "step_down", value)}
        />
        <PolicyNumberField
          label="扩缩容冷却"
          unit="秒"
          min={0}
          max={86400}
          value={policyAdvancedValue(props.value, "scaling", "cooldown_seconds")}
          onChange={(value) => set("scaling", "cooldown_seconds", value)}
        />
      </PolicyConfigCard>
      <PolicyConfigCard
        title="采样（真实流量 / 主动探测）"
        description="真实流量优先；没有新鲜样本时使用主动探测。"
        columns={3}
        wide
      >
        <FormField label="默认探测模型">
          <Input
            value={props.value.probe_model ?? ""}
            onChange={(event) =>
              props.onChange({
                ...props.value,
                probe_model: event.target.value,
              })
            }
            placeholder="例如 gpt-5.1-codex"
          />
        </FormField>
        <FormField label="探测间隔（秒）">
          <Input
            type="number"
            min={30}
            max={86400}
            value={props.value.probe_interval_seconds ?? ""}
            onChange={(event) =>
              props.onChange({
                ...props.value,
                probe_interval_seconds: policyNumberInput(event.target.value),
              })
            }
          />
        </FormField>
        <FormField label="流量回溯（分钟）">
          <Input
            type="number"
            min={1}
            max={10080}
            value={props.value.traffic_lookback_minutes ?? ""}
            onChange={(event) =>
              props.onChange({
                ...props.value,
                traffic_lookback_minutes: policyNumberInput(event.target.value),
              })
            }
          />
        </FormField>
        <FormField label="每账号样本上限">
          <Input
            type="number"
            min={1}
            max={200}
            value={props.value.max_samples_per_account ?? ""}
            onChange={(event) =>
              props.onChange({
                ...props.value,
                max_samples_per_account: policyNumberInput(event.target.value),
              })
            }
          />
        </FormField>
        <PolicyNumberField
          label="探测超时"
          unit="秒"
          min={1}
          max={86400}
          value={policyAdvancedValue(props.value, "probe", "timeout_seconds")}
          onChange={(value) => set("probe", "timeout_seconds", value)}
        />
        <PolicyNumberField
          label="探测并发"
          min={1}
          max={32}
          value={policyAdvancedValue(props.value, "probe", "concurrency")}
          onChange={(value) => set("probe", "concurrency", value)}
        />
        <PolicyNumberField
          label="真实样本新鲜期"
          unit="秒"
          min={1}
          max={86400}
          value={policyAdvancedValue(props.value, "probe", "traffic_fresh_seconds")}
          onChange={(value) => set("probe", "traffic_fresh_seconds", value)}
        />
        <FormField label="探测提示词">
          <Input
            value={String(policyAdvancedValue(props.value, "probe", "prompt") ?? "")}
            onChange={(event) => set("probe", "prompt", event.target.value)}
          />
        </FormField>
        <div className="col-span-full space-y-3 border-y py-3" data-testid="policy-probe-retry">
          <PolicySwitchRow
            label="失败重试"
            description="主动探测失败后，按选定规则在同一账号上再次请求。"
            checked={retryEnabled}
            onCheckedChange={(value) => set("probe", "retry_enabled", value)}
          />
          {retryEnabled ? (
            <div className="space-y-3 pl-0 sm:pl-3">
              <div
                className="bg-muted flex w-fit gap-1 rounded-lg p-1"
                role="group"
                aria-label="探测失败重试模式"
              >
                <Button
                  type="button"
                  size="sm"
                  variant={retrySource === "fixed" ? "secondary" : "ghost"}
                  aria-pressed={retrySource === "fixed"}
                  onClick={() => set("probe", "retry_source", "fixed")}
                >
                  固定规则
                </Button>
                <Button
                  type="button"
                  size="sm"
                  variant={retrySource === "sub2api_pool" ? "secondary" : "ghost"}
                  aria-pressed={retrySource === "sub2api_pool"}
                  onClick={() => set("probe", "retry_source", "sub2api_pool")}
                >
                  跟随账号池
                </Button>
              </div>
              {retrySource === "sub2api_pool" ? (
                <p className="text-muted-foreground text-sm leading-5">
                  读取每个 Sub2API 账号的池模式、重试次数和状态码；未启用池模式的账号不重试。
                </p>
              ) : (
                <div className="grid gap-x-4 gap-y-3 sm:grid-cols-2">
                  <PolicyNumberField
                    label="失败重试次数"
                    min={0}
                    max={10}
                    value={policyAdvancedValue(props.value, "probe", "retry_count")}
                    onChange={(value) => set("probe", "retry_count", value)}
                  />
                  <FormField label="触发重试状态码">
                    <Input
                      value={advancedList(
                        policyAdvancedValue(props.value, "probe", "retry_status_codes"),
                      )}
                      placeholder="429, 500, 502, 503, 504"
                      onChange={(event) =>
                        set(
                          "probe",
                          "retry_status_codes",
                          splitConfigList(event.target.value).map(Number),
                        )
                      }
                    />
                  </FormField>
                </div>
              )}
            </div>
          ) : null}
        </div>
        <div
          className="col-span-full grid gap-2 lg:grid-cols-3"
          data-testid="policy-sampling-switches"
        >
          <PolicySwitchRow
            label="接入真实流量样本"
            description="开启后真实流量优先，无新鲜流量的账号回退主动探测；关闭后只使用主动探测样本。"
            checked={props.value.traffic_enabled}
            onCheckedChange={(enabled) =>
              props.onChange({
                ...props.value,
                traffic_enabled: enabled,
              })
            }
          />
          <PolicySwitchRow
            label="启用主动探测"
            description="为没有新鲜真实流量的账号补充健康样本。"
            checked={props.probesEnabled === true}
            disabled={props.probesPending}
            onCheckedChange={props.onProbesEnabledChange}
          />
          <PolicySwitchRow
            label="有新鲜流量时跳过探测"
            description="减少对上游的额外请求。"
            checked={policyAdvancedValue(props.value, "probe", "skip_when_traffic_fresh") !== false}
            onCheckedChange={(value) => set("probe", "skip_when_traffic_fresh", value)}
          />
        </div>
      </PolicyConfigCard>
    </>
  );
}

export function PolicyRulesEditor(props: PolicyEditorProps) {
  const set = (section: string, path: string, value: unknown) =>
    props.onChange(withPolicyAdvancedValue(props.value, section, path, value));
  const scoreFields = [
    ["perfect", "完美健康"],
    ["slow_ttfb", "响应慢"],
    ["upstream_unknown", "上游未知异常"],
    ["gateway_error", "网关错误"],
    ["quota_exhausted", "限流 / 额度耗尽"],
    ["probe_fail", "探测失败"],
    ["fatal", "凭据失效"],
  ] as const;
  const shortRatioValue = policyAdvancedValue(props.value, "scoring", "short_ratio");
  const shortRatio =
    typeof shortRatioValue === "number" ? shortRatioValue : Number(shortRatioValue);
  const longRatio = Number.isFinite(shortRatio) ? Math.max(0, 1 - shortRatio).toFixed(2) : "-";
  return (
    <>
      <PolicyConfigCard
        title="健康分公式"
        description="短期分按最新样本加权，最终分由短期分与长期分合成。"
        columns={3}
      >
        <PolicyNumberField
          label="短期窗口"
          unit="次"
          min={1}
          max={10000}
          value={policyAdvancedValue(props.value, "scoring", "short_window")}
          onChange={(value) => set("scoring", "short_window", value)}
        />
        <PolicyNumberField
          label="长期窗口"
          unit="次"
          min={1}
          max={100000}
          value={policyAdvancedValue(props.value, "scoring", "long_window")}
          onChange={(value) => set("scoring", "long_window", value)}
        />
        <PolicyNumberField
          label="最新一次权重"
          min={0.000001}
          max={1}
          value={policyAdvancedValue(props.value, "scoring", "latest_weight")}
          step="any"
          onChange={(value) => set("scoring", "latest_weight", value)}
        />
        <PolicyNumberField
          label="短期分占比"
          min={0.000001}
          max={1}
          value={shortRatioValue}
          step="any"
          onChange={(value) => set("scoring", "short_ratio", value)}
        />
        <PolicyNumberField
          label="响应慢阈值"
          unit="毫秒"
          min={1}
          max={3600000}
          value={policyAdvancedValue(props.value, "scoring", "slow_ttfb_ms")}
          onChange={(value) => set("scoring", "slow_ttfb_ms", value)}
        />
        <div className="bg-muted/50 col-span-full rounded-lg px-3 py-2.5 text-sm">
          <span className="font-medium">当前公式：</span>
          最终分 = 短期分 × {Number.isFinite(shortRatio) ? shortRatio.toFixed(2) : "-"} + 长期分 ×{" "}
          {longRatio}
          <span className="text-muted-foreground ml-2">
            （短期最近{" "}
            {advancedNumber(policyAdvancedValue(props.value, "scoring", "short_window")) ||
              "-"}{" "}
            次，最新一次权重{" "}
            {Number(policyAdvancedValue(props.value, "scoring", "latest_weight")) * 100 || "-"}
            %；长期最近{" "}
            {advancedNumber(policyAdvancedValue(props.value, "scoring", "long_window")) || "-"}{" "}
            次均值）
          </span>
        </div>
      </PolicyConfigCard>
      <PolicyConfigCard
        title="事件分值表"
        description="每类结果对应一个健康分，凭据失效固定为一票否决。"
        columns={3}
      >
        {scoreFields.map(([key, label]) => (
          <PolicyNumberField
            key={key}
            label={label}
            unit="分"
            min={key === "quota_exhausted" ? 1 : 0}
            max={100}
            step="any"
            disabled={key === "fatal"}
            value={policyAdvancedValue(props.value, "scoring", `event_scores.${key}`)}
            onChange={(value) => set("scoring", `event_scores.${key}`, key === "fatal" ? 0 : value)}
          />
        ))}
      </PolicyConfigCard>
      <PolicyConfigCard
        title="错误分类"
        description="401 / 403 和鉴权关键字视为凭据失效；余额不足、额度耗尽和限流始终按可恢复问题处理。"
        wide
      >
        <div className="sm:col-span-2 lg:col-span-4">
          <FormField label="致命错误关键字（每行一个）">
            <Textarea
              className="min-h-28"
              value={advancedList(
                policyAdvancedValue(props.value, "classify", "fatal_patterns"),
              ).replaceAll(", ", "\n")}
              onChange={(event) =>
                set("classify", "fatal_patterns", splitConfigList(event.target.value))
              }
            />
          </FormField>
        </div>
        <div className="sm:col-span-2">
          <FormField label="网关错误状态码">
            <Input
              value={advancedList(
                policyAdvancedValue(props.value, "classify", "gateway_status_codes"),
              )}
              onChange={(event) =>
                set(
                  "classify",
                  "gateway_status_codes",
                  splitConfigList(event.target.value).map(Number),
                )
              }
            />
          </FormField>
        </div>
      </PolicyConfigCard>
    </>
  );
}

type PolicyScopeEditorProps = PolicyEditorProps & {
  groups: import("./api").PolicySnapshot["group_strategies"];
  onRestoreControl: () => void;
  restorePending: boolean;
};

export function PolicyScopeLayout(props: PolicyScopeEditorProps) {
  return (
    <div className="flex flex-col gap-4" data-testid="policy-scope-layout">
      <PolicyScopeEditor
        value={props.value}
        onChange={props.onChange}
        groups={props.groups}
        onRestoreControl={props.onRestoreControl}
        restorePending={props.restorePending}
      />
    </div>
  );
}

export function PolicyScopeEditor(props: PolicyScopeEditorProps) {
  const set = (section: string, path: string, value: unknown) =>
    props.onChange(withPolicyAdvancedValue(props.value, section, path, value));
  const configuredMode = policyAdvancedValue(props.value, "scope", "managed_group_mode");
  const configuredIDs = policyAdvancedValue(props.value, "scope", "managed_group_ids");
  const managedIDs = Array.isArray(configuredIDs) ? configuredIDs.map(String) : [];
  const selectedMode = configuredMode === "selected";
  const selectedIDs = new Set(managedIDs);
  const stableGroupIDs = props.groups.flatMap((group) => (group.id === null ? [] : [group.id]));
  function updateManagedGroups(mode: "all" | "selected", ids: string[]) {
    props.onChange(withManagedGroupScope(props.value, mode, ids));
  }
  return (
    <>
      <PolicyConfigCard
        title="账号托管"
        description="人工优先级账号始终由人工控制；其他账号默认由调度引擎统一管理。"
      >
        <div className="col-span-full">
          <PolicySwitchRow
            label="托管所有账号"
            description="开启后，现有和新添加账号的调度开关都由 Console 决定；关闭后，检测到 Sub2API 人工修改时保留人工值并停止托管该账号。"
            checked={policyAdvancedValue(props.value, "scope", "manage_all_accounts") !== false}
            onCheckedChange={(checked) => set("scope", "manage_all_accounts", checked)}
          />
        </div>
      </PolicyConfigCard>
      <PolicyConfigCard
        title="参与守护的分组"
        description="未参与的分组不会被探测、熔断或调权。"
        columns={3}
        switchAction={{
          checked: !selectedMode,
          label: "全部分组参与守护",
          text: "全部分组",
          onCheckedChange: (allGroups) =>
            updateManagedGroups(allGroups ? "all" : "selected", allGroups ? [] : stableGroupIDs),
        }}
      >
        {!selectedMode ? (
          <p className="text-muted-foreground col-span-full text-sm leading-6">
            当前所有分组都参与守护。关闭上方开关可只勾选部分分组。
          </p>
        ) : props.groups.length ? (
          <div
            className="col-span-full grid gap-2 sm:grid-cols-2 xl:grid-cols-3"
            data-testid="managed-group-options"
          >
            {props.groups.map((group) => {
              const disabled = group.id === null;
              const checked = group.id !== null && selectedIDs.has(group.id);
              const detail = [group.id ? `#${group.id}` : "缺少稳定 ID", ...group.platforms]
                .filter(Boolean)
                .join(" · ");
              return (
                <label
                  key={group.id ?? group.name}
                  className={cn(
                    "border-border/70 bg-background hover:bg-muted/40 flex min-h-14 items-center gap-3 rounded-lg border px-3 py-2.5 transition-colors",
                    disabled ? "cursor-not-allowed opacity-60" : "cursor-pointer",
                  )}
                >
                  <Checkbox
                    checked={checked}
                    disabled={disabled}
                    aria-label={`选择分组 ${group.name}`}
                    onCheckedChange={(nextChecked) => {
                      if (group.id === null) return;
                      const next = new Set(selectedIDs);
                      if (nextChecked) next.add(group.id);
                      else next.delete(group.id);
                      updateManagedGroups(
                        "selected",
                        stableGroupIDs.filter((id) => next.has(id)),
                      );
                    }}
                  />
                  <span className="min-w-0">
                    <span className="block truncate text-sm font-medium">{group.name}</span>
                    <span className="text-muted-foreground block truncate text-xs">{detail}</span>
                  </span>
                </label>
              );
            })}
          </div>
        ) : (
          <p className="text-muted-foreground col-span-full text-sm">暂无可选择的分组。</p>
        )}
      </PolicyConfigCard>
      <PolicyConfigCard
        title="账号类型与平台"
        description="留空表示不限；填写后只有匹配的账号类型或平台会被守护。"
      >
        <div className="sm:col-span-2">
          <FormField label="账号类型">
            <Input
              value={advancedList(policyAdvancedValue(props.value, "scope", "account_types"))}
              onChange={(event) =>
                set("scope", "account_types", splitConfigList(event.target.value))
              }
              placeholder="例如 apikey, oauth"
            />
          </FormField>
        </div>
        <div className="sm:col-span-2">
          <FormField label="平台">
            <Input
              value={advancedList(policyAdvancedValue(props.value, "scope", "platforms"))}
              onChange={(event) => set("scope", "platforms", splitConfigList(event.target.value))}
              placeholder="留空表示全部平台"
            />
          </FormField>
        </div>
      </PolicyConfigCard>
      <PolicyConfigCard
        title="排除的分组"
        description="被排除的分组不探测、不熔断、不调权，现有配置保持不动。"
      >
        <div className="sm:col-span-2 lg:col-span-4">
          <FormField label="稳定分组 ID">
            <Input
              value={props.value.excluded_group_ids?.join(", ") ?? ""}
              onChange={(event) =>
                props.onChange({
                  ...props.value,
                  excluded_group_ids: splitConfigList(event.target.value),
                })
              }
              placeholder="留空表示没有排除"
            />
          </FormField>
        </div>
      </PolicyConfigCard>
      <PolicyConfigCard
        title="暂停与排除的渠道"
        description="暂停账号不接流量但继续计分；排除账号完全不参与，并恢复接管前配置。"
      >
        <div className="sm:col-span-2">
          <FormField label="暂停调度的稳定账号 ID">
            <Input
              value={advancedList(policyAdvancedValue(props.value, "scope", "paused_account_ids"))}
              onChange={(event) =>
                set("scope", "paused_account_ids", splitConfigList(event.target.value))
              }
              placeholder="例如 12, 34"
            />
          </FormField>
        </div>
        <div className="sm:col-span-2">
          <FormField label="排除的稳定账号 ID">
            <Input
              value={advancedList(
                policyAdvancedValue(props.value, "scope", "excluded_account_ids"),
              )}
              onChange={(event) =>
                set("scope", "excluded_account_ids", splitConfigList(event.target.value))
              }
              placeholder="例如 12, 34"
            />
          </FormField>
        </div>
      </PolicyConfigCard>
      <Card size="sm" className="border-destructive/40 xl:col-span-2">
        <CardHeader>
          <CardTitle className="text-destructive">交还控制权</CardTitle>
          <CardDescription>
            恢复所有被 Console 改动过的账号在接管前的优先级、负载因子、并发与调度状态。
          </CardDescription>
        </CardHeader>
        <CardContent>
          <Button
            variant="destructive"
            disabled={props.restorePending}
            onClick={props.onRestoreControl}
          >
            <RefreshCw size={16} />
            {props.restorePending ? "正在恢复…" : "恢复全部账号原始配置"}
          </Button>
        </CardContent>
      </Card>
    </>
  );
}

function PanelHeading(props: { title: string; subtitle?: string; action?: React.ReactNode }) {
  return (
    <div className="console-panel-heading bg-card text-card-foreground relative z-10 box-border flex min-h-14 shrink-0 items-center justify-between gap-3 border-b border-border/70 px-4 py-3">
      <div
        className={cn("min-w-0 flex-1", props.subtitle ? "py-0.5" : "flex min-h-5 items-center")}
      >
        <h2 className="truncate text-sm font-semibold leading-5">{props.title}</h2>
        {props.subtitle && (
          <p className="text-muted-foreground mt-1 truncate text-xs leading-4">{props.subtitle}</p>
        )}
      </div>
      {props.action && (
        <div className="flex shrink-0 items-center gap-2 leading-none">{props.action}</div>
      )}
    </div>
  );
}
function StatusPill(props: {
  label: string;
  tone: "success" | "warning" | "danger" | "info" | "neutral";
}) {
  return <StatusBadge label={displayLabel(props.label)} variant={props.tone} />;
}
function effectiveProjectMode(value: string | undefined) {
  if (value === "监控模式" || value === "monitoring") return "监控模式";
  if (value === "完全模式" || value === "full") return "完全模式";
  if (value === "未初始化") return "未初始化";
  return "配置错误";
}
function RunRow(props: {
  status: string;
  title: string;
  detail: string;
  state?: string;
  icon?: React.ReactNode;
  action?: React.ReactNode;
}) {
  const failed = props.status === "failed" || props.status === "error" || props.status === "fused";
  return (
    <div className="console-list-row box-border flex min-h-16 items-center gap-3 border-b border-border/70 px-4 py-3 last:border-b-0">
      <span
        className={cn(
          "flex size-7 shrink-0 items-center justify-center rounded-md",
          failed
            ? "bg-destructive/10 text-destructive"
            : props.status === "succeeded" || props.status === "healthy"
              ? "bg-success/10 text-success"
              : "bg-muted text-muted-foreground",
        )}
      >
        {props.icon ?? <Activity size={15} />}
      </span>
      <div className="min-w-0 flex-1">
        <strong className="block truncate text-sm font-medium leading-5">
          {displayText(props.title)}
        </strong>
        <span className="text-muted-foreground mt-1 block truncate text-xs leading-4">
          {displayText(props.detail)}
        </span>
      </div>
      {props.state !== undefined && props.state !== null && (
        <Badge
          variant={failed ? "destructive" : props.status === "succeeded" ? "secondary" : "outline"}
        >
          {displayLabel(props.state)}
        </Badge>
      )}
      {props.action}
    </div>
  );
}
function TableLoadingRows(props: { columns: number; rows?: number }) {
  return (
    <>
      {Array.from({ length: props.rows ?? 4 }, (_, row) => (
        <TableRow key={`table-loading:${row}`} aria-label="正在加载数据">
          {Array.from({ length: props.columns }, (_, column) => (
            <TableCell key={column}>
              <Skeleton className={cn("h-4", column === 0 ? "w-3/5" : "w-4/5")} />
            </TableCell>
          ))}
        </TableRow>
      ))}
    </>
  );
}
function TableMessageRow(props: { columns: number; children: React.ReactNode }) {
  return (
    <TableRow className="hover:bg-transparent">
      <TableCell colSpan={props.columns} className="h-auto p-0 whitespace-normal">
        {props.children}
      </TableCell>
    </TableRow>
  );
}
function LoadingRows(props: { columns: number; rows?: number; rowClass?: string }) {
  return (
    <div aria-label="正在加载数据">
      {Array.from({ length: props.rows ?? 3 }, (_, row) => (
        <div
          className={cn(
            "console-list-row box-border min-h-16 items-center gap-3 border-b border-border/70 px-4 py-3 last:border-b-0",
            props.rowClass ?? "flex",
          )}
          key={row}
        >
          {Array.from({ length: props.columns }, (_, column) => (
            <Skeleton className={cn("h-4", column === 0 ? "w-3/5" : "w-4/5")} key={column} />
          ))}
        </div>
      ))}
    </div>
  );
}
function EmptyRow(props: { text: string; detail?: string }) {
  return (
    <div className="text-muted-foreground flex items-center gap-2 px-4 py-5 text-sm">
      <CircleHelp size={16} className="shrink-0" />
      <span>
        <strong className="text-foreground block font-medium">{props.text}</strong>
        {props.detail && <small className="mt-1 block">{props.detail}</small>}
      </span>
    </div>
  );
}
function QueryError(props: {
  error: unknown;
  fallback: string;
  embedded?: boolean;
  className?: string;
}) {
  return <QueryErrorToast error={props.error} fallback={props.fallback} />;
}
function formatDate(value: string | null | undefined, includeSeconds = false) {
  if (!value) return "未知时间";
  const numeric = /^\d{10,13}$/.test(value) ? Number(value) : null;
  const date = new Date(numeric === null ? value : value.length === 10 ? numeric * 1000 : numeric);
  return Number.isNaN(date.getTime())
    ? value
    : date.toLocaleString("zh-CN", {
        month: "2-digit",
        day: "2-digit",
        hour: "2-digit",
        minute: "2-digit",
        second: includeSeconds ? "2-digit" : undefined,
      });
}
function formatBalance(value: string) {
  const parsed = Number(value);
  if (!Number.isFinite(parsed)) return value;
  return new Intl.NumberFormat("en-US", {
    style: "currency",
    currency: "USD",
    minimumFractionDigits: 2,
    maximumFractionDigits: 4,
  }).format(parsed);
}

export default App;
