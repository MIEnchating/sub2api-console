import { describe, expect, it } from "vitest";

import type { OnboardingRequest } from "@/api";
import { expandOnboardingCreationRequests } from "../onboarding-requests";

describe("expandOnboardingCreationRequests", () => {
  it("creates one single-group request for each selected group when adding accounts", () => {
    const request: OnboardingRequest = {
      host: "upstream.test",
      upstream_type: "sub2api",
      upstream_group_id: "pro",
      local_group_ids: [3, 5],
      schedulable: false,
    };

    const result = expandOnboardingCreationRequests(request);

    expect(result).toEqual([
      { ...request, local_group_id: 3, local_group_ids: [3] },
      { ...request, local_group_id: 5, local_group_ids: [5] },
    ]);
    expect(request.local_group_ids).toEqual([3, 5]);
  });

  it("keeps an existing-account group update as one request", () => {
    const request: OnboardingRequest = {
      host: "upstream.test",
      upstream_type: "sub2api",
      upstream_group_id: "pro",
      local_group_ids: [3, 5],
      account_ids: ["77"],
    };

    expect(expandOnboardingCreationRequests(request)).toEqual([request]);
  });
});
