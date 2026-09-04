import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import type { GroupAllocation } from "@/api";
import { GroupAllocationContent, groupAllocationLayout } from "../group-allocation-dialog";

const allocation: GroupAllocation = {
  group_id: "6",
  group_name: "codex",
  platform: "openai",
  rate_multiplier: "0.15",
  status: "partial_degraded",
  probe_interval_seconds: 300,
  weight_budget: 400,
  total_weight: 175,
  has_allocation: true,
  strategy: "speed_first",
  account_count: 2,
  healthy_accounts: 1,
  available_accounts: 2,
  fused_accounts: 0,
  paused_accounts: 0,
  unavailable_accounts: 0,
  rate_limited_accounts: 1,
  pending_accounts: 0,
  highest_health_score: 79,
  average_health_score: 72,
  assigned_concurrency: 48,
  channels: [
    {
      account_id: "41",
      account_name: "tokenshen-0.15",
      health: "healthy",
      health_score: 79,
      short_score: 76,
      long_score: 82,
      sample_count: 5,
      ttfb_p95_ms: 39760,
      rate: "1.00",
      priority: 136,
      weight: 117,
      assigned_concurrency: 32,
      schedulable: true,
      rank: 1,
      reason: "健康渠道",
      updated_at: "2026-08-27T08:00:00Z",
    },
    {
      account_id: "42",
      account_name: "backup-0.09",
      health: "degraded",
      health_score: 65,
      short_score: 61,
      long_score: 69,
      sample_count: 4,
      ttfb_p95_ms: 29630,
      rate: "1.00",
      priority: 147,
      weight: 58,
      assigned_concurrency: 16,
      schedulable: true,
      rank: 2,
      reason: "健康分下降",
      updated_at: "2026-08-27T08:00:00Z",
    },
  ],
};

describe("group allocation detail", () => {
  it("prioritizes actionable allocation status and removes secondary summary details", () => {
    const markup = renderToStaticMarkup(<GroupAllocationContent allocation={allocation} />);

    expect(markup).toContain("当前状态");
    expect(markup).toContain("部分异常");
    expect(markup).toContain("巡检策略");
    expect(markup).toContain("速度优先策略");
    expect(markup).toContain("测试周期");
    expect(markup).toContain("每 5 分钟测试");
    expect(markup).toContain("可用账号");
    expect(markup).toContain("2 / 2");
    expect(markup).toContain("其中健康 1");
    expect(markup).toContain("需关注");
    expect(markup).toContain("限流 1");
    expect(markup).toContain("权重分配");
    expect(markup).toContain("175 / 400");
    expect(markup).toContain("已生成最终权重");
    expect(markup).toContain('data-slot="account-health-score"');
    expect(markup).toContain("短期 76");
    expect(markup).toContain("长期 82");
    expect(markup).toContain("分配并发");
    expect(markup).toContain("48");
    expect(markup).toContain("tokenshen-0.15");
    expect(markup).toContain("最终权重 117");
    expect(markup).toContain("39.8s");
    expect(markup).toContain('data-table-panel=""');
    expect(markup).not.toContain("#6");
    expect(markup).not.toContain("分组计费倍率");
    expect(markup).not.toContain("最高分");
    expect(markup).not.toContain("平均分");
    expect(markup).not.toContain("质量分 =");
  });

  it("collapses an all-zero exception breakdown into a healthy summary", () => {
    const markup = renderToStaticMarkup(
      <GroupAllocationContent
        allocation={{
          ...allocation,
          status: "healthy",
          healthy_accounts: 2,
          available_accounts: 2,
          fused_accounts: 0,
          paused_accounts: 0,
          unavailable_accounts: 0,
          rate_limited_accounts: 0,
          pending_accounts: 0,
        }}
      />,
    );

    expect(markup).toContain("当前无异常");
    expect(markup).not.toContain("熔断 0");
    expect(markup).not.toContain("暂停 0");
    expect(markup).not.toContain("不可用 0");
    expect(markup).not.toContain("限流 0");
    expect(markup).not.toContain("待探测 0");
  });

  it("keeps a long allocation table inside a stable scroll area", () => {
    expect(groupAllocationLayout.dialog).toContain("overflow-hidden");
    expect(groupAllocationLayout.width).toBe("table");
    expect(groupAllocationLayout.height).toBe("tall");
    expect(groupAllocationLayout.loading).toContain("h-full");
    expect(groupAllocationLayout.content).toContain("h-full");
    expect(groupAllocationLayout.content).toContain("min-h-0");
    expect(groupAllocationLayout.tableContainer).toContain("h-full");
    expect(groupAllocationLayout.tableContainer).toContain("overflow-auto");
    expect(groupAllocationLayout.tableContainer).toContain("overscroll-contain");
    expect(groupAllocationLayout.policy).toContain("bg-muted");
    expect(groupAllocationLayout.policy).toContain("grid-cols-3");
    expect(groupAllocationLayout.metrics).toContain("lg:grid-cols-4");
    expect(groupAllocationLayout.metric).toContain("bg-popover");
    expect(groupAllocationLayout.metric).not.toContain("bg-background");
    expect(groupAllocationLayout.table).toContain("min-w-[1060px]");
  });

  it("explains that an empty group has no allocation yet", () => {
    const markup = renderToStaticMarkup(
      <GroupAllocationContent allocation={{ ...allocation, account_count: 0, channels: [] }} />,
    );

    expect(markup).toContain("该分组暂无账号");
    expect(markup).toContain("尚未产生账号调度状态");
  });

  it("shows the effective budget without presenting stale weights as a current allocation", () => {
    const markup = renderToStaticMarkup(
      <GroupAllocationContent
        allocation={{
          ...allocation,
          total_weight: 0,
          has_allocation: false,
          channels: allocation.channels.map((channel) => ({ ...channel, weight: null })),
        }}
      />,
    );

    expect(markup).toContain("权重分配");
    expect(markup).toContain("未生成");
    expect(markup).toContain("预算 400");
    expect(markup).toContain("尚未生成账号最终调度状态");
    expect(markup).not.toContain("权重 0.5");
  });
});
