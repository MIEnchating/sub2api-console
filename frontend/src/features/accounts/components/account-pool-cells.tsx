import type { AccountStatus } from "@/api";
import type { ReactElement } from "react";
import { AccountHealthScore } from "@/components/account-health-score";
import { AccountRecentResults } from "@/components/account-recent-results";
import { StatusBadge } from "@/components/status-badge";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { TableOverflowTooltip } from "@/components/ui/table-overflow-tooltip";
import {
  accountPoolState,
  accountSchedulingSwitchLabel,
} from "@/features/accounts/lib/account-pool";
import { AccountRecoveryStatus } from "./account-recovery-status";
import { accountIdentityMeta } from "@/features/accounts/lib/account-labels";
import { cn } from "@/lib/utils";
import { formatHealthScore as healthScoreValue } from "@/lib/health-score";
import { accountConcurrencyLimitedHelp, accountConcurrencyLimitedReason } from "../constants";
import {
  accountLoadFactorLabel,
  effectiveAccountLoadFactor,
  effectiveTargetLoadFactor,
} from "../lib/account-routing-values";

export { AccountLatencyCell } from "./account-latency-cell";

function shortSampleCount(account: AccountStatus): number | string {
  if (account.sample_count === 0) return 0;
  return account.short_sample_count ?? "未记录";
}

function healthScoreAriaLabel(account: AccountStatus): string {
  return [
    "查看健康评分详情",
    `综合健康分 ${healthScoreValue(account.health_score)}`,
    `短期评分 ${healthScoreValue(account.short_score)}`,
    `长期评分 ${healthScoreValue(account.long_score)}`,
    `短期样本数 ${shortSampleCount(account)}`,
    `长期样本数 ${account.long_sample_count ?? account.sample_count}`,
    `连续失败 ${account.failure_streak ?? "—"}`,
    `连续恢复 ${account.recovery_pass_streak ?? "—"}`,
  ].join("，");
}

export function AccountHealthCell(props: { account: AccountStatus }) {
  const account = props.account;
  const state = accountPoolState(account);
  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <div
            className="focus-visible:ring-ring inline-flex cursor-pointer rounded-sm outline-none focus-visible:ring-2 focus-visible:ring-offset-2"
            tabIndex={0}
            aria-label={healthScoreAriaLabel(account)}
          />
        }
      >
        <AccountHealthScore
          score={account.health_score}
          shortScore={account.short_score}
          longScore={account.long_score}
          sampleCount={account.sample_count}
        />
      </TooltipTrigger>
      <TooltipContent
        role="tooltip"
        aria-label="健康评分详情"
        className="grid w-80 max-w-[calc(100vw-2rem)] gap-3 p-3 text-xs"
      >
        <div className="flex items-start justify-between gap-3">
          <div className="grid gap-0.5">
            <strong className="text-sm">健康评分详情</strong>
            <span className="text-muted-foreground">
              {account.health_evaluated_at ? "当前证据评分" : "本轮调度采用的健康评估"}
            </span>
          </div>
          <StatusBadge label={state.label} variant={state.tone} />
        </div>

        {account.health_evaluated_at ? (
          <dl className="grid gap-1.5">
            <div className="grid grid-cols-[5rem_minmax(0,1fr)] gap-3">
              <dt className="text-muted-foreground">评估时间</dt>
              <dd className="text-right break-words tabular-nums">
                <time dateTime={account.health_evaluated_at}>
                  {new Date(account.health_evaluated_at).toLocaleString("zh-CN")}
                </time>
              </dd>
            </div>
            <div className="grid grid-cols-[5rem_minmax(0,1fr)] gap-3">
              <dt className="text-muted-foreground">最新证据时间</dt>
              <dd className="text-right break-words tabular-nums">
                {account.health_evidence_at ? (
                  <time dateTime={account.health_evidence_at}>
                    {new Date(account.health_evidence_at).toLocaleString("zh-CN")}
                  </time>
                ) : (
                  "暂无有效证据"
                )}
              </dd>
            </div>
          </dl>
        ) : null}

        <dl className="grid gap-1.5">
          <div className="flex items-center justify-between gap-4">
            <dt className="text-muted-foreground">综合健康分</dt>
            <dd className="font-semibold tabular-nums">{healthScoreValue(account.health_score)}</dd>
          </div>
          <div className="flex items-center justify-between gap-4">
            <dt className="text-muted-foreground">短期评分</dt>
            <dd className="font-medium tabular-nums">{healthScoreValue(account.short_score)}</dd>
          </div>
          <div className="flex items-center justify-between gap-4">
            <dt className="text-muted-foreground">长期评分</dt>
            <dd className="font-medium tabular-nums">{healthScoreValue(account.long_score)}</dd>
          </div>
        </dl>

        <div className="border-border grid gap-2 border-t pt-2.5">
          <strong>{account.health_evaluated_at ? "评分依据" : "本轮依据"}</strong>
          <dl className="grid grid-cols-2 gap-3">
            <div className="grid gap-0.5">
              <dt className="text-muted-foreground">短期样本数</dt>
              <dd className="font-semibold tabular-nums">{shortSampleCount(account)}</dd>
            </div>
            <div className="grid gap-0.5">
              <dt className="text-muted-foreground">长期样本数</dt>
              <dd className="font-semibold tabular-nums">
                {account.long_sample_count ?? account.sample_count}
              </dd>
            </div>
            <div className="grid gap-0.5">
              <dt className="text-muted-foreground">连续失败</dt>
              <dd className="font-semibold tabular-nums">{account.failure_streak ?? "—"}</dd>
            </div>
            <div className="grid gap-0.5">
              <dt className="text-muted-foreground">连续恢复</dt>
              <dd className="font-semibold tabular-nums">{account.recovery_pass_streak ?? "—"}</dd>
            </div>
          </dl>
          <p className="text-muted-foreground">
            实际参与评分的有效样本数；短期取长期样本中最新的一部分，不重复相加。
          </p>
          {account.health_evaluated_at ? (
            <p className="text-muted-foreground">
              调度状态与连续失败、连续恢复次数沿用最近一次调度评估。
            </p>
          ) : null}
        </div>
      </TooltipContent>
    </Tooltip>
  );
}

export function AccountRecentResultsCell(props: { account: AccountStatus }) {
  return <AccountRecentResults results={props.account.recent_results} />;
}

export function AccountRoutingParametersCell(props: { account: AccountStatus }) {
  const account = props.account;
  const loadFactor = accountLoadFactorLabel(
    effectiveAccountLoadFactor(account),
    account.load_factor,
    account.concurrency,
  );
  const targetLoadFactor = accountLoadFactorLabel(
    effectiveTargetLoadFactor(account),
    account.target_load_factor ?? account.load_factor,
    account.target_concurrency ?? account.concurrency,
  );
  const targetChanged =
    (account.target_priority != null && account.target_priority !== account.priority) ||
    (account.target_load_factor != null && account.target_load_factor !== account.load_factor) ||
    (account.target_concurrency != null && account.target_concurrency !== account.concurrency);
  return (
    <div className="grid gap-1 tabular-nums">
      {account.manual_priority != null ? (
        <>
          <span className="text-primary font-semibold">手动控制 #{account.manual_priority}</span>
          <span className="text-muted-foreground text-xs">
            当前优先级 {account.priority ?? "—"}
          </span>
          <span className="text-muted-foreground text-xs">
            {account.schedulable ? "参与调度" : "停止调度"} ·{" "}
            {account.manual_sync_balance_multiplier ? "同步上游余额" : "不同步上游余额"}
          </span>
        </>
      ) : (
        <span className="font-medium">当前优先级 {account.priority ?? "—"}</span>
      )}
      <span className="text-muted-foreground text-xs">
        负载 {loadFactor} · 并发 {account.concurrency ?? "—"}
      </span>
      {targetChanged ? (
        <div className="border-primary/40 mt-1 grid gap-1 border-l-2 pl-2">
          <span className="text-primary text-xs font-medium">
            目标优先级 {account.target_priority ?? account.priority ?? "—"}
          </span>
          <span className="text-muted-foreground text-xs">
            负载 {targetLoadFactor} · 并发{" "}
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
    <div className="grid min-w-0 gap-0.5">
      <div
        data-slot="account-identity-heading"
        className="flex min-w-0 flex-nowrap items-center gap-2"
      >
        <Tooltip>
          <TooltipTrigger
            render={
              <strong
                tabIndex={0}
                className="block min-w-0 flex-1 truncate rounded-sm font-semibold outline-none focus-visible:ring-2 focus-visible:ring-ring"
              />
            }
          >
            {props.account.name}
          </TooltipTrigger>
          <TooltipContent role="tooltip" className="max-w-sm">
            {props.account.name}
          </TooltipContent>
        </Tooltip>
      </div>
      <div
        data-slot="account-identity-meta"
        className="flex min-w-0 flex-nowrap items-center gap-2 text-xs text-muted-foreground"
      >
        <AccountIdentityMeta account={props.account} className="block min-w-0 flex-1" />
        <Tooltip>
          <TooltipTrigger
            render={
              <p
                tabIndex={0}
                className="min-w-0 max-w-[35%] shrink-0 truncate rounded-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
              />
            }
          >
            {props.account.upstream_host ?? "Host 未记录"}
          </TooltipTrigger>
          <TooltipContent role="tooltip" className="max-w-sm">
            {props.account.upstream_host ?? "Host 未记录"}
          </TooltipContent>
        </Tooltip>
      </div>
      <Tooltip>
        <TooltipTrigger
          render={
            <p
              tabIndex={0}
              className="w-full min-w-0 truncate rounded-sm text-xs text-muted-foreground outline-none focus-visible:ring-2 focus-visible:ring-ring"
            />
          }
        >
          分组：{groups}
        </TooltipTrigger>
        <TooltipContent role="tooltip" className="max-w-sm">
          分组：{groups}
        </TooltipContent>
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

function accountStateReasonLabel(state: ReturnType<typeof accountPoolState>["value"]): string {
  const labels: Partial<Record<ReturnType<typeof accountPoolState>["value"], string>> = {
    degraded: "降级原因",
    cost_blocked: "拦截原因",
    concurrency_limited: "等待原因",
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
  if (state === "concurrency_limited") {
    if (account.decision_state === state && account.decision_reason?.trim()) {
      return account.decision_reason.trim();
    }
    return accountConcurrencyLimitedReason;
  }
  if (account.decision_state !== state) return null;
  return account.decision_reason?.trim() || null;
}

function accountCurrentError(account: AccountStatus): string | null {
  if (account.sub2api_status?.trim().toLowerCase() !== "error") return null;
  return account.sub2api_error?.trim() || "Sub2API 未返回错误原因，请同步账号查看最新状态";
}

function accountSchedulingStopReason(
  account: AccountStatus,
  state: ReturnType<typeof accountPoolState>["value"],
): { label: "停止原因" | "停止原因未记录"; reason: string } | null {
  if (["paused", "disabled", "excluded", "concurrency_limited"].includes(state)) return null;
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

function AccountStateDetail(props: {
  children: string;
  tone?: "default" | "warning" | "danger";
  expanded?: boolean;
}) {
  let toneClass = "text-muted-foreground";
  if (props.tone === "danger") toneClass = "text-destructive";
  if (props.tone === "warning") toneClass = "text-warning";
  if (props.expanded)
    return (
      <p
        className={cn(
          "min-w-0 whitespace-pre-wrap break-words text-sm [overflow-wrap:anywhere]",
          toneClass,
        )}
      >
        {props.children}
      </p>
    );
  return (
    <TableOverflowTooltip content={props.children} className={cn("text-xs", toneClass)}>
      {props.children}
    </TableOverflowTooltip>
  );
}

export function AccountStateCell(props: {
  account: AccountStatus;
  expanded?: boolean;
  compact?: boolean;
}): ReactElement {
  const state = accountPoolState(props.account);
  const reason = accountStateReason(props.account, state.value);
  const evidencePending =
    (state.value === "healthy" || state.value === "degraded") && props.account.evidence_pending;
  let reasonLabel = accountStateReasonLabel(state.value);
  if (evidencePending) reasonLabel = "观察原因";
  if (props.account.recovery) reasonLabel = "当前判定";
  const stateReason = reason && reasonLabel ? `${reasonLabel}：${reason}` : null;
  const currentError = accountCurrentError(props.account);
  const stopReason = accountSchedulingStopReason(props.account, state.value);
  const errorMessage = currentError ? `最近错误：${currentError}` : null;
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
      aria-label={props.account.apply_pending ? (pendingMessage ?? undefined) : undefined}
    />
  );
  if (props.compact && !props.expanded) {
    const pending = props.account.apply_pending ? pendingMessage : null;
    const summary = pending || errorMessage || stopMessage || stateReason;
    let tone: "default" | "warning" | "danger" = "default";
    if (pending) tone = "warning";
    else if (errorMessage || stopMessage) tone = "danger";
    else if (state.value === "degraded" || evidencePending) tone = "warning";
    return (
      <div className="grid min-w-0 gap-1.5">
        <div className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1">
          {badge}
          <span
            className="min-w-0 whitespace-normal text-xs text-muted-foreground"
            aria-label={accountSchedulingSwitchLabel(props.account.schedulable)}
          >
            {accountSchedulingSwitchLabel(props.account.schedulable)}
          </span>
        </div>
        {summary ? <AccountStateDetail tone={tone}>{summary}</AccountStateDetail> : null}
        {state.value === "concurrency_limited" ? (
          <AccountStateDetail tone="warning">{accountConcurrencyLimitedHelp}</AccountStateDetail>
        ) : null}
        {props.account.recovery ? (
          <AccountRecoveryStatus recovery={props.account.recovery} />
        ) : null}
      </div>
    );
  }
  return (
    <div className="grid min-w-0 gap-1">
      {props.expanded ? (
        <p className="text-sm">
          健康评估：
          {props.account.health_score == null ? "暂无有效评分" : `${props.account.health_score} 分`}
          ，有效样本 {props.account.sample_count} 次
        </p>
      ) : null}
      {props.account.apply_pending && desiredState ? (
        <Tooltip>
          <TooltipTrigger render={badge} />
          <TooltipContent className="max-w-sm">{pendingMessage}</TooltipContent>
        </Tooltip>
      ) : (
        badge
      )}
      <AccountStateDetail expanded={props.expanded}>
        {accountSchedulingSwitchLabel(props.account.schedulable)}
      </AccountStateDetail>
      {props.account.apply_pending && pendingMessage ? (
        <AccountStateDetail expanded={props.expanded} tone="warning">
          {pendingMessage}
        </AccountStateDetail>
      ) : null}
      {stateReason && (!props.account.recovery || props.expanded || evidencePending) ? (
        <AccountStateDetail
          expanded={props.expanded}
          tone={state.value === "degraded" || evidencePending ? "warning" : "default"}
        >
          {stateReason}
        </AccountStateDetail>
      ) : null}
      {state.value === "concurrency_limited" ? (
        <AccountStateDetail expanded={props.expanded} tone="warning">
          {accountConcurrencyLimitedHelp}
        </AccountStateDetail>
      ) : null}
      {props.account.recovery ? (
        <AccountRecoveryStatus recovery={props.account.recovery} expanded={props.expanded} />
      ) : null}
      {errorMessage && errorMessage !== stateReason ? (
        <AccountStateDetail expanded={props.expanded} tone="danger">
          {errorMessage}
        </AccountStateDetail>
      ) : null}
      {stopMessage ? (
        <AccountStateDetail expanded={props.expanded} tone="danger">
          {stopMessage}
        </AccountStateDetail>
      ) : null}
    </div>
  );
}
