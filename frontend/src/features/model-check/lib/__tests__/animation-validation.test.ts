import { expect, it } from "vitest";
import { animationSchema, animationScheduleSchema } from "../animation-schema";

it("每日计划未使用间隔时仍允许保存", () => {
  expect(
    animationScheduleSchema.safeParse({
      account_id: "41",
      enabled: true,
      model: "daily-model",
      interval_minutes: 0,
      timeout_seconds: 120,
      version: 1,
      schedule_type: "daily",
      daily_time: "09:30",
      timezone: "Asia/Shanghai",
    }).success,
  ).toBe(true);
});

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

it("选择超过二十个账号时允许提交完整范围，空选择仍拒绝提交", () => {
  const value = {
    account_ids: Array.from({ length: 25 }, (_, index) => String(index + 1)),
    unified_model: "shared-model",
    timeout_seconds: 120,
  };
  expect(animationSchema.parse(value).account_ids).toEqual(value.account_ids);
  expect(animationSchema.safeParse({ ...value, account_ids: [] }).success).toBe(false);
});
