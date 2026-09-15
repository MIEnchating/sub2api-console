import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";

import type { PricingConfig, PricingDecision, PricingGroup } from "@/api";
import { PricingPreviewTable, pricingPreviewDecisions } from "../pricing-page";

function fixture(): {
  config: PricingConfig;
  groups: PricingGroup[];
  decisions: PricingDecision[];
} {
  return {
    config: {
      enabled: true,
      profit_margin: 0.25,
      exchange_group_sets: [["8", "24", "25"]],
      exchange_group_set_names: ["pro 交换"],
      interval_seconds: 3600,
      write_concurrency: 4,
    },
    groups: [
      { id: "8", name: "codex-pro-平价", rate_multiplier: "0.2" },
      { id: "24", name: "codex-pro-特价", rate_multiplier: "0.15" },
      { id: "25", name: "codex-pro-旗舰", rate_multiplier: "0.25" },
    ].map((group) => ({
      ...group,
      platform: "openai",
      status: "active",
      managed: true,
      available: true,
      reason: null,
    })),
    decisions: [
      {
        account_id: "104",
        account_name: "DeepSea API-0.21",
        platform: "openai",
        cost_multiplier: "0.21",
        current_group_ids: ["8"],
        desired_group_ids: ["8"],
        eligible_groups: [],
        changed: false,
        skipped: false,
        reason: null,
      },
    ],
  };
}

describe("价格分组利润不足回退", () => {
  it("账号成本精确等于盈利边界时选择最低售价分组", () => {
    const data = fixture();
    data.config.profit_margin = 0.1;
    data.decisions[0].cost_multiplier = "0.1";
    data.groups[1].rate_multiplier = "0.11";
    expect(
      pricingPreviewDecisions(data.decisions, data.groups, data.config)[0].desired_group_ids,
    ).toEqual(["24"]);
  });

  it("账号成本仅以高精度小数超过盈利边界时选择更高售价分组", () => {
    const data = fixture();
    data.config.profit_margin = 0;
    data.decisions[0].cost_multiplier = "0.10000000000000001";
    data.groups[1].rate_multiplier = "0.1";
    expect(
      pricingPreviewDecisions(data.decisions, data.groups, data.config)[0].desired_group_ids,
    ).toEqual(["8"]);
  });

  it("两个候选售价仅在高精度小数上不同时仍选择较低售价", () => {
    const data = fixture();
    data.decisions[0].cost_multiplier = "0.01";
    data.groups[0].rate_multiplier = "0.10000000000000001";
    data.groups[1].rate_multiplier = "0.1";
    expect(
      pricingPreviewDecisions(data.decisions, data.groups, data.config)[0].desired_group_ids,
    ).toEqual(["24"]);
  });

  it("多个分组可覆盖成本但均未达利润目标时选择最高售价并按稳定 ID 决胜", () => {
    const data = fixture();
    data.groups.push(
      { ...data.groups[0], id: "23", name: "低利润候选", rate_multiplier: "0.23" },
      { ...data.groups[0], id: "28", name: "同价候选", rate_multiplier: "0.25" },
    );
    data.config.exchange_group_sets = [["8", "23", "24", "28", "25"]];
    const decisions = pricingPreviewDecisions(data.decisions, data.groups, data.config);
    expect(decisions[0].desired_group_ids).toEqual(["25"]);
  });

  it("平价组亏损且旗舰组未达到利润目标时预览迁入旗舰组", () => {
    const data = fixture();
    const decisions = pricingPreviewDecisions(data.decisions, data.groups, data.config);
    expect(decisions[0].desired_group_ids).toEqual(["25"]);
    expect(decisions[0].eligible_groups).toEqual(["codex-pro-旗舰"]);
    expect(decisions[0].changed).toBe(true);
  });

  it("回退到旗舰组时展示利润不足原因而非保证目标利润", () => {
    const data = fixture();
    const decisions = pricingPreviewDecisions(data.decisions, data.groups, data.config);
    render(<PricingPreviewTable decisions={decisions} groups={data.groups} config={data.config} />);
    expect(screen.getByText(/未达到目标盈利比例/)).toBeInTheDocument();
    expect(screen.queryByText(/仍能保证.*目标成本利润率的最低售价分组/)).not.toBeInTheDocument();
  });

  it("所有候选组都亏损时保留原组并说明原因", () => {
    const data = fixture();
    data.decisions[0].cost_multiplier = "0.26";
    const decisions = pricingPreviewDecisions(data.decisions, data.groups, data.config);
    expect(decisions[0].desired_group_ids).toEqual(["8"]);
    expect(decisions[0].changed).toBe(false);
    expect(decisions[0].reason).toContain("保留当前分组");
  });

  it("旗舰组售价恰好覆盖成本时允许迁入", () => {
    const data = fixture();
    data.decisions[0].cost_multiplier = "0.25";
    const decisions = pricingPreviewDecisions(data.decisions, data.groups, data.config);
    expect(decisions[0].desired_group_ids).toEqual(["25"]);
  });
});

describe("分组最低迁入倍率预览", () => {
  it("账号成本等于分组下限时允许迁入", () => {
    const data = fixture();
    data.decisions[0].cost_multiplier = "0.12";
    data.config.group_min_cost_multipliers = { "24": "0.12" };
    expect(
      pricingPreviewDecisions(data.decisions, data.groups, data.config)[0].desired_group_ids,
    ).toEqual(["24"]);
  });

  it("账号成本低于最低售价分组下限时迁入其他符合盈利目标的分组", () => {
    const data = fixture();
    data.decisions[0].cost_multiplier = "0.1";
    data.decisions[0].current_group_ids = ["24"];
    data.config.group_min_cost_multipliers = { "24": "0.10000000000000001" };
    expect(
      pricingPreviewDecisions(data.decisions, data.groups, data.config)[0].desired_group_ids,
    ).toEqual(["8"]);
  });

  it("分组未设置下限时继续按售价选择最低合适分组", () => {
    const data = fixture();
    data.decisions[0].cost_multiplier = "0.1";
    expect(
      pricingPreviewDecisions(data.decisions, data.groups, data.config)[0].desired_group_ids,
    ).toEqual(["24"]);
  });

  it("分组下限为零时允许正成本账号迁入", () => {
    const data = fixture();
    data.decisions[0].cost_multiplier = "0.1";
    data.config.group_min_cost_multipliers = { "24": "0" };
    expect(
      pricingPreviewDecisions(data.decisions, data.groups, data.config)[0].desired_group_ids,
    ).toEqual(["24"]);
  });

  it("保本回退的最高售价分组下限不满足时改选其他保本分组", () => {
    const data = fixture();
    data.groups.push({ ...data.groups[0], id: "23", name: "可用保本组", rate_multiplier: "0.23" });
    data.config.exchange_group_sets[0].push("23");
    data.config.group_min_cost_multipliers = { "25": "0.22" };
    expect(
      pricingPreviewDecisions(data.decisions, data.groups, data.config)[0].desired_group_ids,
    ).toEqual(["23"]);
  });

  it("全部分组最低迁入倍率都不满足时保留当前分组并说明原因", () => {
    const data = fixture();
    data.config.group_min_cost_multipliers = { "8": "0.3", "24": "0.3", "25": "0.3" };
    const decision = pricingPreviewDecisions(data.decisions, data.groups, data.config)[0];
    expect(decision.desired_group_ids).toEqual(["8"]);
    expect(decision.reason).toContain("最低迁入倍率");
    expect(decision.reason).toContain("保留当前分组");
  });

  it("因下限不足保留原组时预览原因和计算明细说明被排除的候选", async () => {
    const user = userEvent.setup();
    const data = fixture();
    data.config.group_min_cost_multipliers = { "8": "0.3", "24": "0.3", "25": "0.3" };
    const decisions = pricingPreviewDecisions(data.decisions, data.groups, data.config);
    render(<PricingPreviewTable decisions={decisions} groups={data.groups} config={data.config} />);
    await user.click(screen.getByRole("button", { name: "无需调整" }));
    expect(screen.getByText(/没有同时满足最低迁入倍率且能覆盖成本/)).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "计算明细" }));
    expect(screen.getByText(/codex-pro-特价.*低于最低迁入倍率 0.3/)).toBeInTheDocument();
  });
});
