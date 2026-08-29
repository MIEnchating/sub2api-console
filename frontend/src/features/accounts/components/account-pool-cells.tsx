import type { AccountStatus } from "@/api";
import { AccountHealthScore } from "@/components/account-health-score";
import { AccountRecentResults } from "@/components/account-recent-results";
import { StatusBadge } from "@/components/status-badge";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { TableOverflowTooltip } from "@/components/ui/table-overflow-tooltip";
import { accountPoolState } from "@/features/accounts/lib/account-pool";
import { accountIdentityMeta } from "@/features/accounts/lib/account-labels";
import { cn } from "@/lib/utils";

const secondsFormatter = new Intl.NumberFormat("zh-CN", {
  maximumFractionDigits: 0,
});

function latencySeconds(value: number | null): string {
  if (value === null || !Number.isFinite(value)) return "—";
  return `${secondsFormatter.format(value / 1000)}s`;
}

export function AccountHealthCell(props: { account: AccountStatus }) {
  return (
    <AccountHealthScore
      score={props.account.health_score}
      shortScore={props.account.short_score}
      longScore={props.account.long_score}
      sampleCount={props.account.sample_count}
    />
  );
}

export function AccountRecentResultsCell(props: { account: AccountStatus }) {
  return (
    <AccountRecentResults
      results={props.account.recent_results}
      sampleCount={props.account.sample_count}
      showCount
    />
  );
}

export function AccountLatencyCell(props: { account: AccountStatus }) {
  return (
    <div className="grid gap-1 tabular-nums">
      <span className="font-medium">P95 {latencySeconds(props.account.ttfb_p95_ms)}</span>
      <span className="text-xs text-muted-foreground">
        P50 {latencySeconds(props.account.ttfb_p50_ms)}
      </span>
    </div>
  );
}

export function AccountRoutingParametersCell(props: { account: AccountStatus }) {
  const account = props.account;
  const targetChanged =
    (account.target_priority != null && account.target_priority !== account.priority) ||
    (account.target_load_factor != null && account.target_load_factor !== account.load_factor) ||
    (account.target_concurrency != null && account.target_concurrency !== account.concurrency);
  return (
    <div className="grid gap-1 tabular-nums">
      {account.manual_priority != null ? (
        <span className="text-primary font-semibold">人工优先位 #{account.manual_priority}</span>
      ) : (
        <span className="font-medium">当前优先级 {account.priority ?? "—"}</span>
      )}
      <span className="text-muted-foreground text-xs">
        负载 {account.load_factor ?? "—"} · 并发 {account.concurrency ?? "—"}
      </span>
      {targetChanged ? (
        <div className="border-primary/40 mt-1 grid gap-1 border-l-2 pl-2">
          <span className="text-primary text-xs font-medium">
            目标优先级 {account.target_priority ?? account.priority ?? "—"}
          </span>
          <span className="text-muted-foreground text-xs">
            负载 {account.target_load_factor ?? account.load_factor ?? "—"} · 并发{" "}
            {account.target_concurrency ?? account.concurrency ?? "—"}
          </span>
        </div>
      ) : null}
    </div>
  );
}

export function AccountIdentityMeta(props: { account: AccountStatus; className?: string }) {
  return (
    <span className={cn("text-muted-foreground truncate text-xs", props.className)}>
      {accountIdentityMeta(props.account)}
    </span>
  );
}

export function AccountIdentityCell(props: { account: AccountStatus }) {
  const groups = props.account.groups.length ? props.account.groups.join("、") : "未分组";
  return (
    <div className="w-60 max-w-[15rem]">
      <Tooltip>
        <TooltipTrigger render={<strong className="block truncate font-semibold" />}>
          {props.account.name}
        </TooltipTrigger>
        <TooltipContent className="max-w-sm">{props.account.name}</TooltipContent>
      </Tooltip>
      <AccountIdentityMeta account={props.account} className="mt-0.5 block" />
      <Tooltip>
        <TooltipTrigger render={<p className="mt-0.5 truncate text-xs text-muted-foreground" />}>
          {props.account.upstream_host ?? "Host 未记录"}
        </TooltipTrigger>
        <TooltipContent className="max-w-sm">
          {props.account.upstream_host ?? "Host 未记录"}
        </TooltipContent>
      </Tooltip>
      <Tooltip>
        <TooltipTrigger render={<p className="mt-0.5 truncate text-xs text-muted-foreground" />}>
          分组：{groups}
        </TooltipTrigger>
        <TooltipContent className="max-w-sm">{groups}</TooltipContent>
      </Tooltip>
    </div>
  );
}

function accountStateReasonLabel(state: ReturnType<typeof accountPoolState>["value"]): string {
  const labels: Partial<Record<ReturnType<typeof accountPoolState>["value"], string>> = {
    degraded: "降级原因",
    cost_blocked: "拦截原因",
    fused: "熔断原因",
    survivor: "保底原因",
    paused: "暂停原因",
    disabled: "停用原因",
    excluded: "排除原因",
    unknown: "状态说明",
  };
  return labels[state] ?? "";
}

function accountStateReason(
  account: AccountStatus,
  state: ReturnType<typeof accountPoolState>["value"],
): string | null {
  if (state === "paused") return account.paused_reason?.trim() || null;
  if (state === "disabled") return account.upstream_block_reason?.trim() || null;
  if (account.decision_state !== state) return null;
  return account.decision_reason?.trim() || null;
}

function accountLatestError(account: AccountStatus): string | null {
  if (account.last_error?.trim()) return account.last_error.trim();
  for (const result of account.recent_results) {
    if (result.failure_reason?.trim()) return result.failure_reason.trim();
  }
  return null;
}

function accountSchedulingStopReason(
  account: AccountStatus,
  state: ReturnType<typeof accountPoolState>["value"],
): { label: "停止原因" | "停止原因未记录"; reason: string } | null {
  if (state === "paused" || state === "disabled" || state === "excluded") return null;
  if (!account.upstream_block && account.schedulable !== false) return null;
  if (
    ["fused", "cost_blocked"].includes(account.decision_state ?? "") &&
    account.decision_reason?.trim()
  ) {
    return { label: "停止原因", reason: account.decision_reason.trim() };
  }
  if (account.upstream_block === "unschedulable") {
    return { label: "停止原因未记录", reason: "Sub2API 调度开关已关闭" };
  }
  if (account.upstream_block_reason?.trim()) {
    return { label: "停止原因", reason: account.upstream_block_reason.trim() };
  }
  if (account.schedulable === false) {
    return { label: "停止原因未记录", reason: "Sub2API 调度开关已关闭" };
  }
  return null;
}

function AccountStateDetail(props: { children: string; tone?: "default" | "danger" }) {
  return (
    <TableOverflowTooltip
      content={props.children}
      className={cn(
        "max-w-48 text-xs",
        props.tone === "danger" ? "text-destructive" : "text-muted-foreground",
      )}
    >
      {props.children}
    </TableOverflowTooltip>
  );
}

export function AccountStateCell(props: { account: AccountStatus }) {
  const state = accountPoolState(props.account);
  const reason = accountStateReason(props.account, state.value);
  const reasonLabel = accountStateReasonLabel(state.value);
  const stateReason = reason && reasonLabel ? `${reasonLabel}：${reason}` : null;
  const latestError = accountLatestError(props.account);
  const stopReason = accountSchedulingStopReason(props.account, state.value);
  const errorMessage = latestError ? `最近错误：${latestError}` : null;
  const stopMessage = stopReason ? `${stopReason.label}：${stopReason.reason}` : null;
  const desiredState = props.account.desired_health
    ? accountPoolState({
        ...props.account,
        health: props.account.desired_health,
        apply_pending: false,
      })
    : null;
  const pendingMessage = desiredState
    ? `当前状态：${state.label}。引擎期望：${desiredState.label}。${props.account.apply_error ?? "尚未应用到 Sub2API"}。`
    : null;
  const badge = (
    <StatusBadge
      label={state.label}
      variant={state.tone}
      aria-label={pendingMessage ?? undefined}
    />
  );
  return (
    <div className="grid gap-1">
      {props.account.apply_pending && desiredState ? (
        <Tooltip>
          <TooltipTrigger render={badge} />
          <TooltipContent className="max-w-sm">{pendingMessage}</TooltipContent>
        </Tooltip>
      ) : (
        badge
      )}
      {stateReason ? <AccountStateDetail>{stateReason}</AccountStateDetail> : null}
      {errorMessage && errorMessage !== stateReason ? (
        <AccountStateDetail>{errorMessage}</AccountStateDetail>
      ) : null}
      {stopMessage ? <AccountStateDetail tone="danger">{stopMessage}</AccountStateDetail> : null}
    </div>
  );
}
