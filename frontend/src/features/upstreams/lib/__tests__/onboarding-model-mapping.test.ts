import { expect, it } from "vitest";
import type { OnboardingRequest } from "@/api";
import { modelMappingSchema, withOnboardingModelMappings } from "../onboarding-model-mapping";
import { onboardingAccountPreviewID } from "../onboarding-probe-models";

it("映射留空时接受默认行为，填写多条时去除名称两端空白", () => {
  expect(modelMappingSchema.parse([])).toEqual([]);
  expect(
    modelMappingSchema.parse([
      { source: " alias ", target: " upstream " },
      { source: "second", target: "upstream" },
    ]),
  ).toEqual([
    { source: "alias", target: "upstream" },
    { source: "second", target: "upstream" },
  ]);
});

it.each([
  [{ source: "", target: "upstream" }],
  [{ source: "alias", target: " " }],
  [
    { source: "alias", target: "upstream" },
    { source: " alias ", target: "other" },
  ],
  [{ source: "gpt-*", target: "upstream" }],
  [{ source: "alias", target: "up stream" }],
  [{ source: "alias", target: "x".repeat(257) }],
])("空值、重复或不支持的模型名称阻止提交：%j", (...rows) => {
  expect(modelMappingSchema.safeParse(rows).success).toBe(false);
});

it("超过 100 条映射时拒绝提交", () => {
  const rows = Array.from({ length: 101 }, (_, index) => ({
    source: `alias-${index}`,
    target: "upstream",
  }));
  expect(modelMappingSchema.safeParse(rows).success).toBe(false);
});

const first: OnboardingRequest = {
  host: "upstream.test",
  upstream_type: "sub2api",
  upstream_group_id: "7",
  local_group_ids: [3],
};
it("批量请求重排时按账号匹配映射，已有绑定保持原值", () => {
  const second = { ...first, local_group_ids: [4] };
  const existing = { ...first, account_ids: ["41"] };
  expect(
    withOnboardingModelMappings([second, existing, first], {
      [onboardingAccountPreviewID(first)]: { alias: "upstream" },
      [onboardingAccountPreviewID(second)]: {},
    }),
  ).toEqual([second, existing, { ...first, model_mapping: { alias: "upstream" } }]);
});

it("新增账号缺少对应的映射确认结果时拒绝提交", () => {
  expect(() => withOnboardingModelMappings([first], {})).toThrow("重新预览");
});
