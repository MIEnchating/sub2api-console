import type { AccountStatus, GroupStatus } from "@/api";
import { effectiveAccountState } from "@/features/accounts/lib/account-state";
import { groupPlatformSummary } from "@/features/accounts/lib/account-labels";

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

function accountHealthScore(account: AccountStatus): number | null {
  if (account.health_score !== null && Number.isFinite(account.health_score)) {
    return account.health_score;
  }
  const state = effectiveAccountState(account);
  return HEALTH_SCORES[state] ?? null;
}

function accountBelongsToGroup(account: AccountStatus, group: GroupStatus) {
  const groupName = group.name.trim().toLocaleLowerCase();
  return account.groups.some((name) => name.trim().toLocaleLowerCase() === groupName);
}

function accountBelongsToAnyGroup(account: AccountStatus, groups: GroupStatus[]): boolean {
  return groups.some((group) => accountBelongsToGroup(account, group));
}

export function visibleOverviewGroups(groups: GroupStatus[]): GroupStatus[] {
  return groups.filter((group) => group.participation_status !== "out_of_scope");
}

export function overviewAccounts(
  accounts: AccountStatus[],
  groups: GroupStatus[],
): AccountStatus[] {
  return accounts.filter((account) => accountBelongsToAnyGroup(account, groups));
}

function attentionState(account: AccountStatus): AttentionState | null {
  if (account.apply_pending) return "apply_pending";
  const state = effectiveAccountState(account);
  if (state === "fused") return "fused";
  if (state === "cost_blocked") return "cost_blocked";
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
  if (recentFailure) return recentFailure;

  const fallbacks: Record<AttentionState, string> = {
    apply_pending: "调度结果等待自动执行",
    fused: "渠道已熔断，等待恢复检测",
    cost_blocked: "渠道倍率超过当前成本墙",
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
  const priorities: Record<AttentionState, number> = {
    apply_pending: 0,
    fused: 1,
    cost_blocked: 2,
    disabled: 3,
    paused: 4,
    survivor: 5,
    degraded: 6,
  };

  return overviewAccounts(accounts, visibleOverviewGroups(groups))
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

function roundedAverage(values: number[]): number | null {
  if (values.length === 0) return null;
  return Math.round((values.reduce((total, value) => total + value, 0) / values.length) * 10) / 10;
}

function averageMultiplier(accounts: AccountStatus[]): number | null {
  const values = accounts
    .map((account) => (account.multiplier === null ? Number.NaN : Number(account.multiplier)))
    .filter((value) => Number.isFinite(value));
  if (values.length === 0) return null;
  return Math.round((values.reduce((sum, value) => sum + value, 0) / values.length) * 100) / 100;
}

export function buildGroupHealth(group: GroupStatus, accounts: AccountStatus[]): GroupHealth {
  const groupAccounts = accounts.filter((account) => accountBelongsToGroup(account, group));
  const totalCount = Math.max(group.account_count, groupAccounts.length);
  const liveCount = Math.max(0, Math.min(group.scheduling_open, totalCount));
  const livePercent = totalCount === 0 ? 0 : Math.round((liveCount / totalCount) * 100);
  const scores = groupAccounts
    .map(accountHealthScore)
    .filter((value): value is number => value !== null);
  const healthScore =
    group.average_health_score !== undefined ? group.average_health_score : roundedAverage(scores);
  const scoredCount = group.scored_accounts ?? scores.length;
  const rateLimitedCount = group.rate_limited_accounts ?? 0;
  const fusedCount = groupAccounts.filter((account) => {
    return effectiveAccountState(account) === "fused";
  }).length;
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
    averageMultiplier: averageMultiplier(groupAccounts),
    platformSummary: groupPlatformSummary(group),
    statusLabel,
    tone,
  };
}

export function buildOverviewMetrics(
  accounts: AccountStatus[],
  groups: GroupStatus[],
): OverviewMetrics {
  const states = accounts.map(effectiveAccountState);
  const scores = accounts
    .map(accountHealthScore)
    .filter((value): value is number => value !== null);
  const groupHealth = groups.map((group) => buildGroupHealth(group, accounts));
  const concurrency = accounts
    .map((account) => account.concurrency)
    .filter((value): value is number => value !== null && Number.isFinite(value) && value >= 0);

  return {
    managedAccounts: accounts.length,
    healthyAccounts: states.filter((state) =>
      ["healthy", "active", "available", "succeeded", "passed", "pass", "normal"].includes(state),
    ).length,
    degradedAccounts: states.filter((state) => state === "degraded").length,
    costBlockedAccounts: states.filter((state) => state === "cost_blocked").length,
    fusedAccounts: states.filter((state) => state === "fused").length,
    survivorAccounts: states.filter((state) => state === "survivor").length,
    pausedAccounts: states.filter((state) => state === "paused").length,
    disabledAccounts: states.filter((state) => state === "disabled").length,
    unknownAccounts: states.filter((state) => state === "unknown").length,
    averageHealthScore: roundedAverage(scores),
    assignedConcurrency: concurrency.reduce((sum, value) => sum + value, 0),
    accountsWithConcurrency: concurrency.length,
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
  return labels[value] ?? value;
}
