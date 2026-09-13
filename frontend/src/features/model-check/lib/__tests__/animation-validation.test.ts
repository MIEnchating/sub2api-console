import { expect, it } from "vitest";
import { animationSchema, animationScheduleSchema } from "../animation-schema";

it("检测必须填写统一模型，空模型在统一字段返回错误", () => {
  const value = {
    account_ids: ["41", "42"],
    unified_model: "shared-model",
    timeout_seconds: 120,
  };
  expect(animationSchema.safeParse(value).success).toBe(true);
  const parsed = animationSchema.safeParse({ ...value, unified_model: "" });
  expect(parsed.success).toBe(false);
  if (!parsed.success) expect(parsed.error.issues[0].path).toEqual(["unified_model"]);
});

it.each([0, 1441])("自动检测间隔为 %s 分钟时拒绝保存", (interval) => {
  expect(
    animationScheduleSchema.safeParse({
      account_id: "41",
      enabled: true,
      model: "test-model",
      interval_minutes: interval,
      timeout_seconds: 120,
      version: 0,
    }).success,
  ).toBe(false);
});

it.each(["x".repeat(257), "test\u0085model"])("模型 ID 超长或包含控制字符时拒绝检测", (model) => {
  expect(
    animationSchema.safeParse({
      account_ids: ["41"],
      unified_model: model,
      timeout_seconds: 120,
    }).success,
  ).toBe(false);
});
