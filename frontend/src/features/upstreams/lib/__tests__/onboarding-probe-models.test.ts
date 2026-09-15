import { expect, it } from "vitest";
import type { OnboardingRequest } from "@/api";
import { onboardingAccountPreviewID, withOnboardingProbeModels } from "../onboarding-probe-models";

const created: OnboardingRequest = {
  host: "upstream.example",
  upstream_type: "sub2api",
  upstream_group_id: "7",
  local_group_ids: [3],
};

it("两个新增账号分别读取自身模型，即使请求顺序调整也不串号", () => {
  const second = { ...created, local_group_ids: [4] };
  const existing = { ...created, account_ids: ["41"] };
  const models = {
    [onboardingAccountPreviewID(created)]: ["gpt-5.2"],
    [onboardingAccountPreviewID(second)]: ["gpt-5.1"],
  };
  expect(withOnboardingProbeModels([second, existing, created], models)).toEqual([
    { ...second, test_models: ["gpt-5.1"] },
    existing,
    { ...created, test_models: ["gpt-5.2"] },
  ]);
});

it("单个账号留空模型时仅该账号继承默认", () => {
  const second = { ...created, local_group_ids: [4] };
  const models = {
    [onboardingAccountPreviewID(created)]: ["gpt-5.2"],
    [onboardingAccountPreviewID(second)]: [],
  };
  expect(withOnboardingProbeModels([created, second], models)).toEqual([
    { ...created, test_models: ["gpt-5.2"] },
    { ...second, test_models: [] },
  ]);
});

it("账号与已确认模型配置不匹配时拒绝提交", () => {
  expect(() => withOnboardingProbeModels([created], {})).toThrow("重新预览");
});
