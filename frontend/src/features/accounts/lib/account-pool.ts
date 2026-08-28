import type { AccountStatus } from "@/api";
import type { StatusVariant } from "@/components/status-badge";
import { effectiveAccountState } from "@/features/accounts/lib/account-state";

export type AccountPoolFilter =
  | "all"
  | "healthy"
  | "degraded"
  | "cost_blocked"
  | "survivor"
  | "fused"
  | "paused"
  | "disabled"
  | "excluded"
  | "unknown";

type AccountPoolState = Exclude<AccountPoolFilter, "all">;

export type AccountPoolStateMeta = {
  value: AccountPoolState;
  label: string;
  tone: StatusVariant;
};

export const accountPoolFilters: Array<{
  value: AccountPoolFilter;
  label: string;
}> = [
  { value: "all", label: "全部" },
  { value: "healthy", label: "健康" },
  { value: "degraded", label: "降级" },
  { value: "cost_blocked", label: "成本墙拦截" },
  { value: "fused", label: "已熔断" },
  { value: "survivor", label: "保底强留" },
  { value: "paused", label: "已暂停" },
  { value: "disabled", label: "已停用" },
  { value: "excluded", label: "已排除" },
  { value: "unknown", label: "待探测" },
];

const stateMeta: Record<AccountPoolState, AccountPoolStateMeta> = {
  healthy: { value: "healthy", label: "健康", tone: "success" },
  degraded: { value: "degraded", label: "降级", tone: "warning" },
  cost_blocked: { value: "cost_blocked", label: "成本墙拦截", tone: "warning" },
  survivor: { value: "survivor", label: "保底强留", tone: "purple" },
  fused: { value: "fused", label: "已熔断", tone: "danger" },
  paused: { value: "paused", label: "已暂停", tone: "warning" },
  disabled: { value: "disabled", label: "已停用", tone: "neutral" },
  excluded: { value: "excluded", label: "已排除", tone: "neutral" },
  unknown: { value: "unknown", label: "待探测", tone: "neutral" },
};

export function accountPoolState(account: AccountStatus): AccountPoolStateMeta {
  return stateMeta[effectiveAccountState(account)];
}

export function accountPoolCounts(accounts: AccountStatus[]): Record<AccountPoolFilter, number> {
  const counts: Record<AccountPoolFilter, number> = {
    all: accounts.length,
    healthy: 0,
    degraded: 0,
    cost_blocked: 0,
    survivor: 0,
    fused: 0,
    paused: 0,
    disabled: 0,
    excluded: 0,
    unknown: 0,
  };
  for (const account of accounts) {
    counts[accountPoolState(account).value] += 1;
  }
  return counts;
}

export function accountMatchesPoolFilter(
  account: AccountStatus,
  filter: AccountPoolFilter,
): boolean {
  return filter === "all" || accountPoolState(account).value === filter;
}
