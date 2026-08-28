import { describe, expect, it } from "vitest";

import { modelCheckSchema } from "../model-check-schema";

const valid = {
  account_ids: ["41", "42"],
  models: ["claude-opus-5", "gpt-5.6-sol"],
  rounds: 1,
  timeout_seconds: 45,
};

describe("模型检测参数", () => {
  it("接受多账号和多模型矩阵", () => {
    expect(modelCheckSchema.safeParse(valid).success).toBe(true);
  });

  it("拒绝空账号或空模型选择", () => {
    expect(modelCheckSchema.safeParse({ ...valid, account_ids: [] }).success).toBe(false);
    expect(modelCheckSchema.safeParse({ ...valid, models: [] }).success).toBe(false);
  });

  it("拒绝无效账号 ID 和超过 100 个组合", () => {
    expect(modelCheckSchema.safeParse({ ...valid, account_ids: ["account-name"] }).success).toBe(
      false,
    );
    expect(
      modelCheckSchema.safeParse({
        ...valid,
        account_ids: Array.from({ length: 11 }, (_, index) => String(index + 1)),
        models: Array.from({ length: 10 }, (_, index) => `model-${index}`),
      }).success,
    ).toBe(false);
  });
});
