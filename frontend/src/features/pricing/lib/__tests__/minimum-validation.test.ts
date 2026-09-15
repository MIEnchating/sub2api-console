import { describe, expect, it } from "vitest";

import { groupMinimumSchema, groupMeetsMinimumCost } from "../group-minimum";

describe("最低迁入倍率校验", () => {
  it("留空时视为未设置且任何账号成本均可参与选择", () => {
    expect(groupMinimumSchema.parse({ minimum: "  " })).toEqual({ minimum: "" });
    expect(groupMeetsMinimumCost("0.01", "")).toBe(true);
  });

  it.each(["0", "0.10000000000000001", ".5", "1.", "1e-3"])(
    "输入有效非负倍率 %s 时保留原始精度",
    (minimum) => {
      expect(groupMinimumSchema.parse({ minimum })).toEqual({ minimum });
    },
  );

  it.each(["-0.1", "NaN", "Infinity", "abc", "1,2", "1e1001", "1".repeat(129)])(
    "输入无效倍率 %s 时拒绝保存",
    (minimum) => {
      expect(groupMinimumSchema.safeParse({ minimum }).success).toBe(false);
    },
  );

  it("成本精确等于下限时允许迁入", () => {
    expect(groupMeetsMinimumCost("0.10000000000000001", "1.0000000000000001e-1")).toBe(true);
  });

  it("成本仅低于下限最后一位时拒绝迁入", () => {
    expect(groupMeetsMinimumCost("0.1", "0.10000000000000001")).toBe(false);
  });
});
