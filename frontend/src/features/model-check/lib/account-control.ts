import type { AccountControlAction, AccountStatus } from "@/api";
import { accountPoolState } from "@/features/accounts/lib/account-pool";
import { accountConcurrencyLimitedHelp } from "@/features/accounts/constants";

export type DetectionControlAction = Extract<AccountControlAction, "fuse" | "recover" | "resume">;
export const detectionControlLabels: Record<DetectionControlAction, string> = {
  fuse: "手动熔断",
  recover: "解除熔断",
  resume: "恢复调度",
};

export function detectionControlState(account: AccountStatus): {
  recovery: DetectionControlAction;
  fuseReason: string;
  recoveryReason: string;
} {
  const state = accountPoolState({ ...account, health: account.health ?? "" }).value;
  let protectedReason = "";
  if (!/^[1-9]\d*$/.test(account.id)) protectedReason = "账号 ID 无效，请同步账号";
  else if (account.manual_priority != null) protectedReason = "请先取消手动控制";
  else if (state === "excluded") protectedReason = "请先在账号管理恢复管控";
  const fused = state === "fused";
  const recovery = fused || state === "cost_blocked" ? "recover" : "resume";
  let fuseReason = protectedReason;
  let recoveryReason = protectedReason;
  if (!fuseReason && (fused || state === "paused")) fuseReason = "账号已停止调度";
  if (!recoveryReason) {
    if (state === "concurrency_limited") recoveryReason = accountConcurrencyLimitedHelp;
    else if (!fused && state !== "paused" && account.schedulable !== false)
      recoveryReason = "账号当前无需恢复调度";
  }
  return { recovery, fuseReason, recoveryReason };
}

export function detectionControlBlocked(
  account: AccountStatus,
  action: DetectionControlAction,
): string {
  const state = detectionControlState(account);
  if (action === "fuse") return state.fuseReason;
  if (action !== state.recovery) return "账号状态已变化，请取消后重新选择操作";
  return state.recoveryReason;
}
