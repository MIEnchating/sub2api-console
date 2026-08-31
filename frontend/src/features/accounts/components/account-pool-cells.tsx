import type { AccountStatus } from "@/api";
import { CircleHelp } from "lucide-react";
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
    <div className="grid gap-1 tabular-nums" aria-label="综合延迟">
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
        <>
          <span className="text-primary font-semibold">人工优先位 #{account.manual_priority}</span>
          <span className="text-muted-foreground text-xs">
            {account.manual_sync_balance_multiplier ? "仅同步余额与倍率" : "完全人工控制"}
          </span>
        </>
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

export function AccountBaseURLCell(props: { account: AccountStatus; expanded?: boolean }) {
  const account = props.account;
  const presentation = accountBaseURLPresentation(account);
  const reason = account.base_url_check_reason?.trim() || "没有 Base URL 校验结果";
  return (
    <div className={cn("grid gap-1", props.expanded ? "max-w-none" : "max-w-48")}>
      <StatusBadge label={presentation.label} variant={presentation.variant} title={reason} />
      <TableOverflowTooltip
        content={
          account.base_url ?? (account.base_url_checked_at ? "详情未提供 Base URL" : "等待校验")
        }
        className={cn("text-xs", props.expanded ? "max-w-none" : "max-w-48")}
      >
        {account.base_url ?? (account.base_url_checked_at ? "详情未提供 Base URL" : "等待校验")}
      </TableOverflowTooltip>
      {account.base_url_source === "platform_default" ? (
        <span className="text-muted-foreground text-xs">来源：Sub2API 平台默认地址</span>
      ) : null}
      {account.upstream_base_url ? (
        <TableOverflowTooltip
          content={`上游访问地址：${account.upstream_base_url}`}
          className={cn(
            "text-muted-foreground text-xs",
            props.expanded ? "max-w-none" : "max-w-48",
          )}
        >
          上游访问地址：{account.upstream_base_url}
        </TableOverflowTooltip>
      ) : null}
    </div>
  );
}

export function accountBaseURLPresentation(account: AccountStatus) {
  return {
    matched: { label: "同一地址", variant: "success" as const },
    different_allowed: { label: "地址不同（允许）", variant: "info" as const },
    official_mismatch: { label: "配置异常", variant: "danger" as const },
    invalid: { label: "地址不可读", variant: "warning" as const },
    unchecked: { label: "尚未校验", variant: "neutral" as const },
    unknown: {
      label: account.base_url ? "缺少上游信息" : "缺少账号 Base URL",
      variant: "neutral" as const,
    },
  }[account.base_url_check ?? "unknown"];
}

export function AccountKeyStatusCell(props: { account: AccountStatus }) {
  const raw = props.account.key_status?.trim().toLowerCase() ?? "";
  const presentation = (() => {
    if (["active", "enabled", "available", "ok", "1"].includes(raw)) {
      return { label: "正常", variant: "success" as const };
    }
    if (["inactive", "disabled", "2"].includes(raw)) {
      return { label: "已停用", variant: "warning" as const };
    }
    if (raw === "suspected") {
      return { label: "待复核", variant: "warning" as const };
    }
    if (["key_missing", "missing", "deleted"].includes(raw)) {
      return { label: "Key 已删除", variant: "danger" as const };
    }
    if (raw === "group_missing") {
      return { label: "分组已删除", variant: "danger" as const };
    }
    if (raw === "key_and_group_missing") {
      return { label: "Key、分组已删除", variant: "danger" as const };
    }
    if (raw === "group_inactive") {
      return { label: "分组已停用", variant: "warning" as const };
    }
    if (["expired", "3"].includes(raw)) {
      return { label: "已过期", variant: "danger" as const };
    }
    if (["exhausted", "4"].includes(raw)) {
      return { label: "额度耗尽", variant: "warning" as const };
    }
    if (raw === "mixed") {
      return { label: "状态不一致", variant: "warning" as const };
    }
    if (raw === "unbound") {
      return { label: "未绑定 Key", variant: "neutral" as const };
    }
    if (raw === "unknown" || !raw) {
      return { label: "状态未知", variant: "neutral" as const };
    }
    return { label: "其他状态", variant: "neutral" as const };
  })();
  const detail =
    props.account.key_status_reason?.trim() ||
    (raw ? `上游 Key 原始状态：${raw}` : "尚未从上游同步 Key 状态");
  return <StatusBadge label={presentation.label} variant={presentation.variant} title={detail} />;
}

export function AccountSub2APIStatusCell(props: { account: AccountStatus }) {
  const raw = props.account.sub2api_status?.trim().toLowerCase() ?? "";
  const error = props.account.sub2api_error?.trim() ?? "";
  const presentation = (() => {
    if (raw === "error") return { label: "错误", variant: "danger" as const };
    if (raw === "active" && props.account.schedulable === false) {
      return { label: "暂停", variant: "neutral" as const };
    }
    if (raw === "active") return { label: "正常", variant: "success" as const };
    if (["disabled", "inactive"].includes(raw)) {
      return { label: "停用", variant: "neutral" as const };
    }
    if (raw === "expired") return { label: "已过期", variant: "danger" as const };
    if (!raw) return { label: "未同步", variant: "neutral" as const };
    return { label: props.account.sub2api_status!.trim(), variant: "warning" as const };
  })();
  return (
    <div className="flex min-w-0 items-center gap-1">
      <StatusBadge
        label={presentation.label}
        variant={presentation.variant}
        title={raw ? `Sub2API 原始状态：${raw}` : "管理快照尚未返回账号状态"}
      />
      {error ? (
        <Tooltip>
          <TooltipTrigger
            render={
              <button
                type="button"
                className="text-destructive hover:text-destructive/80 inline-flex size-5 shrink-0 items-center justify-center"
                aria-label="查看 Sub2API 账号报错"
              />
            }
          >
            <CircleHelp className="size-3.5" aria-hidden="true" />
          </TooltipTrigger>
          <TooltipContent className="max-w-sm whitespace-pre-wrap break-words">
            {error}
          </TooltipContent>
        </Tooltip>
      ) : null}
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
  const sub2apiError = account.sub2api_error?.trim();
  if (account.last_error?.trim() && account.last_error.trim() !== sub2apiError) {
    return account.last_error.trim();
  }
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
  const stopMessage =
    stopReason && stopReason.reason !== reason ? `${stopReason.label}：${stopReason.reason}` : null;
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
