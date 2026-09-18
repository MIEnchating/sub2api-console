import { describe, expect, it } from "vitest";
import { effectiveAccountLoadFactor, effectiveTargetLoadFactor } from "../account-routing-values";

describe("账号有效负载因子", () => {
  it("上游省略负载因子时跟随并发上限", () => {
    expect(effectiveAccountLoadFactor({ load_factor: null, concurrency: 1 })).toBe("1");
  });

  it("已配置负载因子时保留配置值", () => {
    expect(effectiveAccountLoadFactor({ load_factor: " 2.5 ", concurrency: 1 })).toBe("2.5");
  });

  it("只改变目标并发时不会覆盖显式配置的负载因子", () => {
    expect(
      effectiveTargetLoadFactor({
        target_load_factor: null,
        target_concurrency: 4,
        load_factor: "2",
        concurrency: 1,
      }),
    ).toBe("2");
  });

  it("已知并发为零且没有负载因子时使用上游默认值1", () => {
    expect(effectiveAccountLoadFactor({ load_factor: null, concurrency: 0 })).toBe("1");
  });

  it("并发数据也缺失时保留未知状态", () => {
    expect(effectiveAccountLoadFactor({ load_factor: null, concurrency: null })).toBeNull();
  });

  it("目标值省略时优先跟随目标并发，再回退当前有效值", () => {
    expect(
      effectiveTargetLoadFactor({
        target_load_factor: null,
        target_concurrency: 4,
        load_factor: null,
        concurrency: 1,
      }),
    ).toBe("4");
    expect(
      effectiveTargetLoadFactor({
        target_load_factor: null,
        target_concurrency: null,
        load_factor: null,
        concurrency: 1,
      }),
    ).toBe("1");
  });
});
