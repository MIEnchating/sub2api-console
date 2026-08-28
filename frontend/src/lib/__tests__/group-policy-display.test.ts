import { describe, expect, it } from "vitest";

import {
  groupStatusMeta,
  overviewStrategyLabel,
  participationReasonLabel,
  participationStatusLabel,
  strategySourceLabel,
} from "../group-policy-display";

describe("group policy display", () => {
  it("shows the persisted strategy source", () => {
    expect(strategySourceLabel("global_default")).toBe("全局默认");
    expect(strategySourceLabel("group_override")).toBe("分组覆盖");
  });

  it("shows participation independently from strategy source", () => {
    expect(participationStatusLabel("participating")).toBe("已参与");
    expect(participationStatusLabel("out_of_scope")).toBe("未参与");
  });

  it("keeps the server reason and uses a neutral marker when absent", () => {
    expect(participationReasonLabel("分组 ID 12 位于排除分组列表中")).toBe(
      "分组 ID 12 位于排除分组列表中",
    );
    expect(participationReasonLabel(null)).toBe("—");
  });

  it("shows unparticipating groups as unparticipating in the overview strategy column", () => {
    expect(overviewStrategyLabel("均衡", "out_of_scope")).toBe("未参与");
    expect(overviewStrategyLabel("均衡", "participating")).toBe("均衡");
  });

  it.each([
    ["healthy", "健康", "success"],
    ["rate_limited", "限流中", "warning"],
    ["partial_degraded", "部分异常", "warning"],
    ["survivor_only", "仅剩保底", "danger"],
    ["all_fused", "全部熔断", "danger"],
    ["all_unavailable", "全部不可调度", "danger"],
    ["excluded", "已排除", "neutral"],
    ["skipped", "未参与", "neutral"],
    ["empty", "无账号", "neutral"],
    ["configuration_error", "配置异常", "danger"],
  ])("maps group status %s to %s", (status, label, tone) => {
    expect(groupStatusMeta(status)).toEqual({ label, tone });
  });
});
