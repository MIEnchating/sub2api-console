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
  it("shows group summary and per-account routing allocation", () => {
    const markup = renderToStaticMarkup(<GroupAllocationContent allocation={allocation} />);

    expect(markup).toContain("健康 / 可用");
    expect(markup).toContain("codex");
    expect(markup).toContain("部分异常");
    expect(markup).toContain("每 5 分钟测试");
    expect(markup).toContain(
      "#6 · openai · 分组计费倍率 0.15 · 渠道 2 · 最终权重合计 175 · 分组预算 400",
    );
    expect(markup).toContain("1 / 2");
    expect(markup).toContain("最高分");
    expect(markup).toContain("79");
    expect(markup).toContain("分配并发");
    expect(markup).toContain("48");
    expect(markup).toContain("tokenshen-0.15");
    expect(markup).toContain("最终权重 117");
    expect(markup).toContain("39.8s");
    expect(markup).toContain("速度优先：质量分 = 健康门控 ×（80% 相对速度 + 20% 相对价格）");
    expect(markup).toContain("最终权重 = 组内预算 × 质量分 ÷ 质量分总和");
  });

  it("keeps a long allocation table inside a stable scroll area", () => {
    expect(groupAllocationLayout.dialog).toContain("overflow-hidden");
    expect(groupAllocationLayout.width).toBe("table");
    expect(groupAllocationLayout.height).toBe("tall");
    expect(groupAllocationLayout.loading).toContain("h-full");
    expect(groupAllocationLayout.content).toContain("min-h-0");
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

    expect(markup).toContain("分组权重预算 400");
    expect(markup).toContain("尚未生成账号最终调度状态");
    expect(markup).not.toContain("权重 0.5");
  });
});
