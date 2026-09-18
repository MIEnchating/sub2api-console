import { expect, it } from "vitest";

import { searchOnboardingCandidates } from "../onboarding-candidate-search";

const candidates = [
  { group_id: "113", group_name: "站长专用分组", description: "OpenAI 专线" },
  { group_id: "92", group_name: "超算中心", description: null },
];

it("输入分组名称、稳定 ID 或说明时匹配对应分组", () => {
  for (const query of ["站长", "113", " openai "]) {
    expect(searchOnboardingCandidates(candidates, query)).toEqual([candidates[0]]);
  }
});

it("清空搜索后保留全部分组及原始顺序", () => {
  expect(searchOnboardingCandidates(candidates, "  ")).toEqual(candidates);
});

it("没有匹配项或没有候选分组时返回空列表", () => {
  expect(searchOnboardingCandidates(candidates, "不存在")).toEqual([]);
  expect(searchOnboardingCandidates([], "站长")).toEqual([]);
});
