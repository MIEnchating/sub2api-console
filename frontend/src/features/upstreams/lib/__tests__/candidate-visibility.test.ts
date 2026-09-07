import { describe, expect, it } from "vitest";

import {
  defaultOnlyShowEnabledOnboardingGroups,
  filterOnboardingCandidates,
} from "../onboarding-candidate-visibility";

const candidates = [
  { groupName: "active", status: "active" },
  { groupName: "enabled", status: " ENABLED " },
  { groupName: "disabled", status: "disabled" },
  { groupName: "inactive", status: "inactive" },
  { groupName: "unknown", status: null },
];

describe("账号添加候选分组可见性", () => {
  it("默认只保留上游标记为启用的分组", () => {
    expect(defaultOnlyShowEnabledOnboardingGroups).toBe(true);
    expect(filterOnboardingCandidates(candidates).map((candidate) => candidate.groupName)).toEqual([
      "active",
      "enabled",
    ]);
  });

  it("关闭仅显示启用分组后保留全部候选分组", () => {
    expect(
      filterOnboardingCandidates(candidates, false).map((candidate) => candidate.groupName),
    ).toEqual(["active", "enabled", "disabled", "inactive", "unknown"]);
  });
});
