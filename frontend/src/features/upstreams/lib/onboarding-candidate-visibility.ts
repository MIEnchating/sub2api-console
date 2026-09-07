import type { OnboardingCandidate } from "@/api";

const enabledUpstreamGroupStatuses = new Set(["active", "enabled", "available", "ok", "1"]);

export const defaultOnlyShowEnabledOnboardingGroups = true;

function onboardingCandidateIsEnabled(candidate: Pick<OnboardingCandidate, "status">): boolean {
  const status = candidate.status?.trim().toLocaleLowerCase() ?? "";
  return enabledUpstreamGroupStatuses.has(status);
}

export function filterOnboardingCandidates<Candidate extends Pick<OnboardingCandidate, "status">>(
  candidates: readonly Candidate[],
  onlyShowEnabled = defaultOnlyShowEnabledOnboardingGroups,
): Candidate[] {
  if (!onlyShowEnabled) return [...candidates];
  return candidates.filter(onboardingCandidateIsEnabled);
}
