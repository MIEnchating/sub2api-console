import type { OnboardingRequest } from "@/api";

export function expandOnboardingCreationRequests(request: OnboardingRequest): OnboardingRequest[] {
  const localGroupIDs = request.local_group_ids ?? [];
  if ((request.account_ids?.length ?? 0) > 0 || localGroupIDs.length <= 1) {
    return [request];
  }
  return localGroupIDs.map((localGroupID) => ({
    ...request,
    local_group_id: localGroupID,
    local_group_ids: [localGroupID],
  }));
}
