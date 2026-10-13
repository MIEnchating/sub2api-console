import { expect, it } from "vitest";
import {
  animationSchema,
  animationScheduleSchema,
  customAnimationSchema,
} from "../animation-schema";

const account = { account_ids: ["1"], unified_model: "first-model", timeout_seconds: 120 };
it("未提供多模型列表时保留原单模型表单", () => {
  expect(animationSchema.parse(account)).toEqual(account);
});
it("多个模型去除首尾空白并保留选择顺序", () => {
  expect(
    animationSchema.parse({ ...account, additional_models: [" second-model ", "third-model"] })
      .additional_models,
  ).toEqual(["second-model", "third-model"]);
});
it.each([
  [""],
  ["first-model"],
  [" second ", "second"],
  ["bad\nmodel"],
  ["x".repeat(257)],
  Array.from({ length: 16 }, (_, i) => `m-${i}`),
])("附加模型无效时拒绝提交 %j", (models) => {
  expect(animationSchema.safeParse({ ...account, additional_models: models }).success).toBe(false);
});
it("附加模型包含密钥时拒绝自定义接口提交", () => {
  expect(
    customAnimationSchema.safeParse({
      base_url: "https://example.invalid",
      api_key: "secret-test",
      platform: "openai",
      model: "first-model",
      additional_models: ["secret-test"],
      timeout_seconds: 120,
    }).success,
  ).toBe(false);
});
it("自动检测计划拒绝重复模型", () => {
  expect(
    animationScheduleSchema.safeParse({
      account_id: "1",
      enabled: true,
      model: "first-model",
      additional_models: ["first-model"],
      interval_minutes: 60,
      timeout_seconds: 120,
      version: 0,
    }).success,
  ).toBe(false);
});
