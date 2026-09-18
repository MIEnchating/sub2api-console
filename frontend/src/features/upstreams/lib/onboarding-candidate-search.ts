import type { OnboardingCandidate } from "@/api";

export function searchOnboardingCandidates<
  Candidate extends Pick<OnboardingCandidate, "group_id" | "group_name" | "description">,
>(candidates: readonly Candidate[], search: string): Candidate[] {
  const query = search.trim().toLocaleLowerCase();
  if (!query) return [...candidates];
  return candidates.filter((candidate) =>
    [candidate.group_id, candidate.group_name, candidate.description].some((value) =>
      value?.toLocaleLowerCase().includes(query),
    ),
  );
}
