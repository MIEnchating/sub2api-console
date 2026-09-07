import { describe, expect, it } from "vitest";

import { modelPriceAdjustmentSchema } from "../model-price-adjustment-schema";

describe("模型价格调整百分比校验", () => {
  it("下调允许 99.99% 但拒绝会把价格降为零的 100%", () => {
    const schema = modelPriceAdjustmentSchema("decrease");

    expect(schema.safeParse({ percentage: 99.99 }).success).toBe(true);
    expect(schema.safeParse({ percentage: 100 }).success).toBe(false);
  });

  it("上调拒绝非正数和超过 1000% 的输入", () => {
    const schema = modelPriceAdjustmentSchema("increase");

    expect(schema.safeParse({ percentage: 0 }).success).toBe(false);
    expect(schema.safeParse({ percentage: 1000 }).success).toBe(true);
    expect(schema.safeParse({ percentage: 1000.01 }).success).toBe(false);
  });
});
