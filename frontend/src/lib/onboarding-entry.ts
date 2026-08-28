import type { OnboardingCandidate } from "../api";

export type OnboardingEntryKind = "full" | "host" | "group";

export function onboardingEntryKind(
  host: string | undefined,
  groupId: string | undefined,
): OnboardingEntryKind {
  if (host?.trim() && groupId?.trim()) return "group";
  if (host?.trim()) return "host";
  return "full";
}

export function onboardingEntryDescription(kind: OnboardingEntryKind): string {
  if (kind === "group") {
    return "上游及上游分组已确定，选择本地分组后添加账号。";
  }
  if (kind === "host") {
    return "上游已确定，选择上游分组和本地分组后添加账号。";
  }
  return "先添加并验证上游，再同步上游信息并选择分组添加账号。";
}

export function onboardingSelectionTitle(kind: OnboardingEntryKind): string {
  if (kind === "group") return "选择本地分组并添加账号";
  if (kind === "host") return "选择分组并添加账号";
  return "第二步：选择分组并添加账号";
}

export function candidateCanCreateKey(
  candidate: Pick<OnboardingCandidate, "can_create_key">,
): boolean {
  return candidate.can_create_key;
}

export function rechargeRatioLabel(rechargeRate: string | null): string {
  if (!rechargeRate?.trim()) return "未配置";
  return `1:${rechargeRate.trim()}`;
}

export function canSubmitOnboarding(
  pending: boolean,
  upstreamGroupId: string | null,
  localGroupId: string,
): boolean {
  return !pending && Boolean(upstreamGroupId?.trim()) && Boolean(localGroupId.trim());
}

export function candidateCreationUnavailableReason(
  candidate: Pick<OnboardingCandidate, "unavailable_reason">,
): string {
  if (candidate.unavailable_reason) return candidate.unavailable_reason;
  return "当前无法创建 Key";
}

export function localGroupSelectionLabel(
  groups: Array<{ id: string | null; name: string }>,
  value: string,
): string {
  const group = groups.find((item) => item.id === value);
  if (!group) return "所选分组不可用";
  return group.name;
}

export function onboardingCandidateStats(
  candidates: Array<Pick<OnboardingCandidate, "bound_accounts" | "can_create_key">>,
): { selectable: number; bound: number } {
  return {
    selectable: candidates.filter(candidateCanCreateKey).length,
    bound: candidates.filter((candidate) =>
      candidate.bound_accounts.some(
        (account) => account.account_exists && account.binding_status !== "missing",
      ),
    ).length,
  };
}
