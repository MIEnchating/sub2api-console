import type { OnboardingCandidate } from "../api";

export type OnboardingEntryKind = "full" | "host" | "group";

export type OnboardingBaseUrlFields = {
  baseUrlProtocol: "http" | "https";
  baseUrl: string;
};

export type OnboardingUpstreamTarget = {
  host: string;
  name: string;
  upstream_type: string;
};

type OnboardingLocalGroup = {
  platform?: string | null;
  platforms?: string[];
};

const compositeAccountPlatforms = new Set([
  "anthropic",
  "openai",
  "gemini",
  "antigravity",
  "grok",
  "kimi",
  "zhipu",
  "deepseek",
  "opencode",
]);

function normalizeOnboardingPlatform(value: string | null | undefined): string {
  const platform = value?.trim().toLocaleLowerCase() ?? "";
  if (["sub2api", "newapi", "oneapi"].includes(platform)) return "openai";
  if (["glm", "zhipuai"].includes(platform)) return "zhipu";
  if (platform === "claude") return "anthropic";
  if (platform === "google") return "gemini";
  if (platform === "moonshot") return "kimi";
  return platform;
}

function accountPlatformCanJoinGroup(accountPlatform: string, groupPlatform: string): boolean {
  if (accountPlatform === groupPlatform) return true;
  return groupPlatform === "composite" && compositeAccountPlatforms.has(accountPlatform);
}

export function compatibleOnboardingLocalGroups<T extends OnboardingLocalGroup>(
  candidate: Pick<OnboardingCandidate, "platform">,
  groups: T[],
): T[] {
  const upstreamPlatform = normalizeOnboardingPlatform(candidate.platform);
  return groups.filter((group) => {
    const platforms = [group.platform, ...(group.platforms ?? [])]
      .map(normalizeOnboardingPlatform)
      .filter(Boolean);
    if (!upstreamPlatform) return platforms.length > 0;
    return platforms.some((platform) => accountPlatformCanJoinGroup(upstreamPlatform, platform));
  });
}

export function isCompositeOnboardingPlatform(value: string | null | undefined): boolean {
  return normalizeOnboardingPlatform(value) === "composite";
}

export function adjacentOnboardingUpstreams(
  upstreams: OnboardingUpstreamTarget[],
  currentHost: string | null,
): { previous: OnboardingUpstreamTarget | null; next: OnboardingUpstreamTarget | null } {
  const normalizedCurrent = currentHost?.trim().toLocaleLowerCase();
  if (!normalizedCurrent) return { previous: null, next: null };
  const currentIndex = upstreams.findIndex(
    (upstream) => upstream.host.trim().toLocaleLowerCase() === normalizedCurrent,
  );
  if (currentIndex < 0) return { previous: null, next: null };
  return {
    previous: currentIndex > 0 ? upstreams[currentIndex - 1] : null,
    next: currentIndex + 1 < upstreams.length ? upstreams[currentIndex + 1] : null,
  };
}

export function normalizeOnboardingHost(value: string): string {
  const trimmed = value.trim();
  if (!trimmed || trimmed.includes("://") || /[/\\?#]/.test(trimmed)) return "";
  try {
    const parsed = new URL(`https://${trimmed}`);
    if (!parsed.hostname || parsed.username || parsed.password) return "";
    return parsed.host.toLowerCase();
  } catch {
    return "";
  }
}

export function upstreamHostFromBaseUrl(value: string): string {
  try {
    const parsed = new URL(value.trim());
    if (!["http:", "https:"].includes(parsed.protocol) || !parsed.hostname) return "";
    if (parsed.username || parsed.password) return "";
    return parsed.host.toLowerCase();
  } catch {
    return "";
  }
}

export function composeOnboardingBaseUrl(value: OnboardingBaseUrlFields): string {
  return `${value.baseUrlProtocol}://${value.baseUrl.trim().replace(/^\/+/, "")}`;
}

export function parseOnboardingBaseUrl(value: string): OnboardingBaseUrlFields {
  const trimmed = value.trim();
  return {
    baseUrlProtocol: trimmed.toLowerCase().startsWith("http://") ? "http" : "https",
    baseUrl: trimmed.replace(/^https?:\/\//i, ""),
  };
}

export function normalizeOnboardingBaseUrlInput(
  value: string,
  selectedProtocol: OnboardingBaseUrlFields["baseUrlProtocol"],
): OnboardingBaseUrlFields {
  const protocolMatch = value.match(/^\s*(https?):\/\//i);
  if (!protocolMatch) {
    return { baseUrlProtocol: selectedProtocol, baseUrl: value };
  }
  const parsedProtocol = protocolMatch[1].toLowerCase();
  return {
    baseUrlProtocol: parsedProtocol === "http" ? "http" : "https",
    baseUrl: value.slice(protocolMatch[0].length),
  };
}

export function onboardingRequestHost(
  upstream: Pick<{ host: string; base_url: string }, "host" | "base_url"> | null,
  draftHost: string,
): string {
  return normalizeOnboardingHost(upstream?.host ?? draftHost);
}

export function onboardingUpstreamRequest(
  baseUrl: string,
): { host: string; baseUrl: string } | null {
  const normalizedBaseUrl = baseUrl.trim();
  const host = upstreamHostFromBaseUrl(normalizedBaseUrl);
  if (!host) return null;
  return { host, baseUrl: normalizedBaseUrl };
}

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
  candidate: Pick<OnboardingCandidate, "can_create_key"> &
    Partial<Pick<OnboardingCandidate, "bound" | "bound_accounts">>,
): boolean {
  return candidate.can_create_key && !candidateHasExistingBinding(candidate);
}

export function candidateHasExistingBinding(
  candidate: Partial<Pick<OnboardingCandidate, "bound" | "bound_accounts">>,
): boolean {
  return (candidate.bound_accounts ?? []).some(
    (account) => account.account_exists && account.binding_status !== "missing",
  );
}

export function candidateBoundLocalGroups(
  candidate: Partial<Pick<OnboardingCandidate, "bound_accounts">>,
): string[] {
  return [
    ...new Set(
      (candidate.bound_accounts ?? [])
        .filter((account) => account.account_exists && account.binding_status !== "missing")
        .flatMap((account) => {
          const groups = account.local_groups?.map((group) => group.name.trim()).filter(Boolean);
          return groups?.length ? groups : [account.local_group.trim()];
        })
        .filter(Boolean),
    ),
  ].sort((left, right) => left.localeCompare(right));
}

export function candidateBoundLocalGroupIDs(
  candidate: Partial<Pick<OnboardingCandidate, "bound_accounts">>,
): string[] {
  return [
    ...new Set(
      (candidate.bound_accounts ?? [])
        .filter((account) => account.account_exists && account.binding_status !== "missing")
        .flatMap((account) => account.local_groups?.map((group) => group.id.trim()) ?? [])
        .filter(Boolean),
    ),
  ].sort((left, right) => left.localeCompare(right));
}

export function candidateBoundAccountIDs(
  candidate: Partial<Pick<OnboardingCandidate, "bound_accounts">>,
): string[] {
  return [
    ...new Set(
      (candidate.bound_accounts ?? [])
        .filter((account) => account.account_exists && account.binding_status !== "missing")
        .map((account) => account.account_id.trim())
        .filter(Boolean),
    ),
  ].sort((left, right) => left.localeCompare(right));
}

export function candidateBoundBaseURLs(
  candidate: Partial<Pick<OnboardingCandidate, "bound_accounts">>,
): string[] {
  return [
    ...new Set(
      (candidate.bound_accounts ?? [])
        .filter((account) => account.account_exists && account.binding_status !== "missing")
        .map((account) => account.base_url?.trim().replace(/\/+$/, "") ?? "")
        .filter(Boolean),
    ),
  ].sort((left, right) => left.localeCompare(right));
}

export function candidateUsesAccountBaseURL(
  candidate: Partial<Pick<OnboardingCandidate, "bound_accounts">>,
  baseURL: string,
): boolean {
  const current = candidateBoundBaseURLs(candidate);
  const desired = baseURL.trim().replace(/\/+$/, "");
  return current.length === 1 && current[0] === desired;
}

export function candidateHasOnboardingChange(
  candidate: Partial<Pick<OnboardingCandidate, "bound" | "bound_accounts">>,
  selectedLocalGroupIDs: string[],
  accountBaseURL: string,
  accountBaseURLEdited: boolean,
): boolean {
  if (!candidateHasExistingBinding(candidate)) return selectedLocalGroupIDs.length > 0;
  return (
    !sameOnboardingGroupSelection(selectedLocalGroupIDs, candidateBoundLocalGroupIDs(candidate)) ||
    (accountBaseURLEdited && !candidateUsesAccountBaseURL(candidate, accountBaseURL))
  );
}

export function localGroupMultiplierLabel(
  groups: Array<{ rate_multiplier?: string | null }>,
): string {
  if (groups.length === 0) return "—";
  return [...new Set(groups.map((group) => group.rate_multiplier?.trim() || "未设置"))].join("、");
}

export function sameOnboardingGroupSelection(left: string[], right: string[]): boolean {
  return [...new Set(left)].sort().join(",") === [...new Set(right)].sort().join(",");
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
