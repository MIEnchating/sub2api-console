import type { AccountStatus, GroupStatus } from "@/api";
import { effectiveAccountState } from "@/features/accounts/lib/account-state";
import { groupPlatformSummary } from "@/features/accounts/lib/account-labels";
import { accountConcurrencyLimitedHelp } from "@/features/accounts/constants";

export type HealthTone = "healthy" | "warning" | "critical";

export type GroupHealth = {
  group: GroupStatus;
  accounts: AccountStatus[];
  liveCount: number;
  totalCount: number;
  livePercent: number;
  healthScore: number | null;
  scoredCount: number;
  needsAttention: number;
  rateLimitedCount: number;
  fusedCount: number;
  averageMultiplier: number | null;
  platformSummary: string;
  statusLabel: string;
  tone: HealthTone;
};

export type OverviewMetrics = {
  managedAccounts: number;
  healthyAccounts: number;
  degradedAccounts: number;
  costBlockedAccounts: number;
  concurrencyLimitedAccounts: number;
  fusedAccounts: number;
  survivorAccounts: number;
  pausedAccounts: number;
  disabledAccounts: number;
  unknownAccounts: number;
  averageHealthScore: number | null;
  assignedConcurrency: number;
  accountsWithConcurrency: number;
  riskGroups: number;
  criticalGroups: number;
};

export type AttentionState =
  | "apply_pending"
  | "fused"
  | "cost_blocked"
  | "concurrency_limited"
  | "survivor"
  | "degraded"
  | "paused"
  | "disabled";

export type AttentionAccount = {
  account: AccountStatus;
  state: AttentionState;
  reason: string;
};

const HEALTH_SCORES: Record<string, number> = {
  healthy: 100,
  active: 100,
  available: 100,
  succeeded: 100,
  passed: 100,
  pass: 100,
  normal: 100,
  degraded: 60,
  cost_blocked: 0,
  survivor: 30,
  fused: 0,
  paused: 0,
  disabled: 0,
  unavailable: 0,
  failed: 0,
};

const HEALTHY_STATES = new Set([
  "healthy",
  "active",
  "available",
  "succeeded",
  "passed",
  "pass",
  "normal",
]);

function accountHealthScore(account: AccountStatus): number | null {
  return accountHealthScoreForState(account, effectiveAccountState(account));
}

function accountHealthScoreForState(
  account: AccountStatus,
  state: ReturnType<typeof effectiveAccountState>,
): number | null {
  if (account.health_score !== null && Number.isFinite(account.health_score)) {
    return account.health_score;
  }
  return HEALTH_SCORES[state] ?? null;
}

function accountBelongsToGroup(account: AccountStatus, group: GroupStatus) {
  const groupName = group.name.trim().toLocaleLowerCase();
  return account.groups.some((name) => name.trim().toLocaleLowerCase() === groupName);
}

function accountBelongsToAnyGroup(account: AccountStatus, groups: GroupStatus[]): boolean {
  return groups.some((group) => accountBelongsToGroup(account, group));
}

/** 一次建立成员索引，指标与卡片共用分组健康结果。 */
export function buildOverviewHealth(
  accounts: AccountStatus[],
  groups: GroupStatus[],
): {
  metrics: OverviewMetrics;
  healthRows: GroupHealth[];
  attention: AttentionAccount[];
} {
  const visibleGroups = visibleOverviewGroups(groups);
  const members = new Map<string, AccountStatus[]>();
  for (const group of visibleGroups) members.set(group.name.trim().toLocaleLowerCase(), []);
  const managed: AccountStatus[] = [];
  for (const account of accounts) {
    let included = false;
    const names = new Set(account.groups.map((name) => name.trim().toLocaleLowerCase()));
    for (const name of names) {
      const groupMembers = members.get(name);
      if (!groupMembers) continue;
      groupMembers.push(account);
      included = true;
    }
    if (included) managed.push(account);
  }
  const healthRows = visibleGroups.map((group) =>
    groupHealthFromMembers(group, members.get(group.name.trim().toLocaleLowerCase()) ?? []),
  );
  return {
    metrics: metricsFromGroupHealth(managed, healthRows),
    healthRows,
    attention: attentionFromManagedAccounts(managed),
  };
}

export function visibleOverviewGroups(groups: GroupStatus[]): GroupStatus[] {
  return groups.filter((group) => group.participation_status !== "out_of_scope");
}

function overviewAccounts(accounts: AccountStatus[], groups: GroupStatus[]): AccountStatus[] {
  return accounts.filter((account) => accountBelongsToAnyGroup(account, groups));
}

function attentionState(account: AccountStatus): AttentionState | null {
  if (account.apply_pending) return "apply_pending";
  const state = effectiveAccountState(account);
  if (state === "fused") return "fused";
  if (state === "cost_blocked") return "cost_blocked";
  if (state === "concurrency_limited") return "concurrency_limited";
  if (state === "paused") return "paused";
  if (state === "disabled") return "disabled";
  if (state === "survivor") return "survivor";
  if (state === "degraded") return "degraded";
  return null;
}

function attentionReason(account: AccountStatus, state: AttentionState): string {
  const recentFailure = account.recent_results.find(
    (sample) => sample.failure_reason,
  )?.failure_reason;
  if (account.apply_error) return account.apply_error;
  if (account.decision_reason) return account.decision_reason;
  if (state === "concurrency_limited") return accountConcurrencyLimitedHelp;
  if (recentFailure) return recentFailure;

  const fallbacks: Record<AttentionState, string> = {
    apply_pending: "调度结果等待自动执行",
    fused: "渠道已熔断，等待恢复检测",
    cost_blocked: "渠道倍率超过当前成本墙",
    concurrency_limited: accountConcurrencyLimitedHelp,
    survivor: "渠道处于保底运行状态",
    degraded: "近期健康表现下降",
    paused: "渠道已被人工暂停，需要手动恢复",
    disabled: "渠道已停用，不参与调度",
  };
  return fallbacks[state];
}

export function buildAttentionAccounts(
  accounts: AccountStatus[],
  groups: GroupStatus[],
): AttentionAccount[] {
  return attentionFromManagedAccounts(overviewAccounts(accounts, visibleOverviewGroups(groups)));
}

function attentionFromManagedAccounts(accounts: AccountStatus[]): AttentionAccount[] {
  const priorities: Record<AttentionState, number> = {
    apply_pending: 0,
    fused: 1,
    cost_blocked: 2,
    concurrency_limited: 2,
    disabled: 3,
    paused: 4,
    survivor: 5,
    degraded: 6,
  };

  return accounts
    .map((account): AttentionAccount | null => {
      const state = attentionState(account);
      if (!state) return null;
      return { account, state, reason: attentionReason(account, state) };
    })
    .filter((item): item is AttentionAccount => item !== null)
    .sort((left, right) => {
      const priorityDifference = priorities[left.state] - priorities[right.state];
      if (priorityDifference !== 0) return priorityDifference;
      const scoreDifference =
        (accountHealthScore(left.account) ?? 101) - (accountHealthScore(right.account) ?? 101);
      if (scoreDifference !== 0) return scoreDifference;
      return (
        left.account.name.localeCompare(right.account.name, "zh-CN") ||
        left.account.id.localeCompare(right.account.id)
      );
    });
}

export function buildGroupHealth(group: GroupStatus, accounts: AccountStatus[]): GroupHealth {
  const groupAccounts = accounts.filter((account) => accountBelongsToGroup(account, group));
  return groupHealthFromMembers(group, groupAccounts);
}

function groupHealthFromMembers(group: GroupStatus, groupAccounts: AccountStatus[]): GroupHealth {
  const totalCount = Math.max(group.account_count, groupAccounts.length);
  const liveCount = Math.max(0, Math.min(group.scheduling_open, totalCount));
  const livePercent = totalCount === 0 ? 0 : Math.round((liveCount / totalCount) * 100);
  let scoreTotal = 0;
  let scoreCount = 0;
  let multiplierTotal = 0;
  let multiplierCount = 0;
  let fusedCount = 0;
  for (const account of groupAccounts) {
    const state = effectiveAccountState(account);
    const score = accountHealthScoreForState(account, state);
    if (score !== null) {
      scoreTotal += score;
      scoreCount += 1;
    }
    if (state === "fused") fusedCount += 1;
    if (account.multiplier !== null) {
      const multiplier = Number(account.multiplier);
      if (Number.isFinite(multiplier)) {
        multiplierTotal += multiplier;
        multiplierCount += 1;
      }
    }
  }
  let healthScore: number | null;
  if (group.average_health_score !== undefined) {
    healthScore = group.average_health_score;
  } else if (scoreCount === 0) {
    healthScore = null;
  } else {
    healthScore = Math.round((scoreTotal / scoreCount) * 10) / 10;
  }
  const scoredCount = group.scored_accounts ?? scoreCount;
  const rateLimitedCount = group.rate_limited_accounts ?? 0;
  const fallbackAttention = Math.min(
    totalCount,
    Math.max(0, totalCount - liveCount, group.scheduling_closed + group.scheduling_unknown),
  );
  const needsAttention = group.needs_attention ?? fallbackAttention;

  let tone: HealthTone = "healthy";
  let statusLabel = "运行健康";
  if (group.participation_status === "configuration_error") {
    tone = "critical";
    statusLabel = "配置异常";
  } else if (totalCount === 0 || livePercent <= 25) {
    tone = "critical";
    statusLabel = totalCount === 0 ? "暂无账号" : "仅剩保底";
  } else if (
    group.participation_status !== "participating" ||
    needsAttention > 0 ||
    rateLimitedCount > 0 ||
    livePercent < 80
  ) {
    tone = "warning";
    if (group.participation_status === "out_of_scope") {
      statusLabel = "未参与调度";
    } else if (needsAttention === 0 && rateLimitedCount > 0) {
      statusLabel = "限流中";
    } else {
      statusLabel = "部分异常";
    }
  }

  return {
    group,
    accounts: groupAccounts,
    liveCount,
    totalCount,
    livePercent,
    healthScore,
    scoredCount,
    needsAttention,
    rateLimitedCount,
    fusedCount,
    averageMultiplier:
      multiplierCount === 0 ? null : Math.round((multiplierTotal / multiplierCount) * 100) / 100,
    platformSummary: groupPlatformSummary(group),
    statusLabel,
    tone,
  };
}

export function buildOverviewMetrics(
  accounts: AccountStatus[],
  groups: GroupStatus[],
): OverviewMetrics {
  return metricsFromGroupHealth(
    accounts,
    groups.map((group) => buildGroupHealth(group, accounts)),
  );
}

function metricsFromGroupHealth(
  accounts: AccountStatus[],
  groupHealth: GroupHealth[],
): OverviewMetrics {
  let healthyAccounts = 0;
  let degradedAccounts = 0;
  let costBlockedAccounts = 0;
  let concurrencyLimitedAccounts = 0;
  let fusedAccounts = 0;
  let survivorAccounts = 0;
  let pausedAccounts = 0;
  let disabledAccounts = 0;
  let unknownAccounts = 0;
  let scoreTotal = 0;
  let scoreCount = 0;
  let assignedConcurrency = 0;
  let accountsWithConcurrency = 0;
  for (const account of accounts) {
    const state = effectiveAccountState(account);
    if (HEALTHY_STATES.has(state)) healthyAccounts += 1;
    if (state === "degraded") degradedAccounts += 1;
    if (state === "cost_blocked") costBlockedAccounts += 1;
    if (state === "concurrency_limited") concurrencyLimitedAccounts += 1;
    if (state === "fused") fusedAccounts += 1;
    if (state === "survivor") survivorAccounts += 1;
    if (state === "paused") pausedAccounts += 1;
    if (state === "disabled") disabledAccounts += 1;
    if (state === "unknown") unknownAccounts += 1;
    const score = accountHealthScoreForState(account, state);
    if (score !== null) {
      scoreTotal += score;
      scoreCount += 1;
    }
    if (
      account.concurrency !== null &&
      Number.isFinite(account.concurrency) &&
      account.concurrency >= 0
    ) {
      assignedConcurrency += account.concurrency;
      accountsWithConcurrency += 1;
    }
  }

  return {
    managedAccounts: accounts.length,
    healthyAccounts,
    degradedAccounts,
    costBlockedAccounts,
    concurrencyLimitedAccounts,
    fusedAccounts,
    survivorAccounts,
    pausedAccounts,
    disabledAccounts,
    unknownAccounts,
    averageHealthScore: scoreCount === 0 ? null : Math.round((scoreTotal / scoreCount) * 10) / 10,
    assignedConcurrency,
    accountsWithConcurrency,
    riskGroups: groupHealth.filter((group) => group.tone !== "healthy").length,
    criticalGroups: groupHealth.filter((group) => group.tone === "critical").length,
  };
}

export function strategyLabel(value: string): string {
  const labels: Record<string, string> = {
    balanced: "均衡调度",
    price: "价格优先",
    price_first: "价格优先",
    cost_first: "价格优先",
    speed: "速度优先",
    latency_first: "速度优先",
    speed_first: "速度优先",
    reliability: "稳定优先",
    reliability_first: "稳定优先",
    stable: "稳定优先",
    stability: "稳定优先",
    stability_first: "稳定优先",
  };
  return Object.hasOwn(labels, value) ? labels[value] : value;
}
