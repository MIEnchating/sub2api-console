import type { AccountStatus } from "@/api";

export type CanonicalAccountState =
  | "healthy"
  | "degraded"
  | "fused"
  | "cost_blocked"
  | "survivor"
  | "paused"
  | "disabled"
  | "excluded"
  | "unknown";

const aliases: Record<string, CanonicalAccountState> = {
  healthy: "healthy",
  active: "healthy",
  available: "healthy",
  normal: "healthy",
  passed: "healthy",
  pass: "healthy",
  succeeded: "healthy",
  success: "healthy",
  通过: "healthy",
  正常: "healthy",
  健康: "healthy",
  degraded: "degraded",
  observing: "degraded",
  降级: "degraded",
  观察中: "degraded",
  fused: "fused",
  fuse_pending: "fused",
  hard_open: "fused",
  soft_open: "fused",
  熔断: "fused",
  已熔断: "fused",
  cost_blocked: "cost_blocked",
  "cost-wall-blocked": "cost_blocked",
  成本墙拦截: "cost_blocked",
  已被成本墙拦截: "cost_blocked",
  survivor: "survivor",
  paused: "paused",
  暂停: "paused",
  已暂停: "paused",
  disabled: "disabled",
  inactive: "disabled",
  停用: "disabled",
  已停用: "disabled",
  excluded: "excluded",
  out_of_scope: "excluded",
  排除: "excluded",
  已排除: "excluded",
};

function normalizeAccountState(value: string | null | undefined): CanonicalAccountState {
  return aliases[(value ?? "").trim().toLowerCase()] ?? "unknown";
}

export function effectiveAccountState(account: AccountStatus): CanonicalAccountState {
  if (account.paused === true) return "paused";
  if (account.health.trim()) return normalizeAccountState(account.health);
  if (account.routing_state?.trim()) return normalizeAccountState(account.routing_state);
  return normalizeAccountState(account.health_status);
}
