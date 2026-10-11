import { describe, expect, it } from "vitest";

import type { AccountStatus, GroupStatus } from "@/api";
import {
  buildAttentionAccounts,
  buildGroupHealth,
  buildOverviewMetrics,
  buildOverviewHealth,
  visibleOverviewGroups,
  strategyLabel,
} from "../overview-health";

function account(overrides: Partial<AccountStatus> = {}): AccountStatus {
  return {
    id: "1",
    name: "账号 1",
    groups: ["codex"],
    upstream_id: "up_test",
    upstream_host: "api.example.com",
    upstream_type: "newapi",
    schedulable: true,
    priority: 10,
    load_factor: "1",
    concurrency: 8,
    multiplier: "0.15",
    balance: "10",
    paused: false,
    paused_reason: null,
    routing_state: "active",
    health_status: "healthy",
    health: "healthy",
    desired_health: null,
    apply_pending: false,
    apply_error: null,
    decision_state: null,
    decision_reason: null,
    failure_streak: 0,
    recovery_pass_streak: 0,
    target_priority: null,
    target_load_factor: null,
    target_schedulable: null,
    target_concurrency: null,
    health_score: null,
    short_score: null,
    long_score: null,
    sample_count: 0,
    recent_results: [],
    ttfb_p50_ms: null,
    ttfb_p95_ms: null,
    weight: null,
    ...overrides,
  };
}

function group(overrides: Partial<GroupStatus> = {}): GroupStatus {
  return {
    name: "codex",
    id: "6",
    account_count: 4,
    scheduling_open: 3,
    scheduling_closed: 1,
    scheduling_unknown: 0,
    strategy: "speed_first",
    strategy_source: "global_default",
    participation_status: "participating",
    participation_reason: null,
    status: "healthy",
    ...overrides,
  };
}

describe("overview health calculations", () => {
  it("总览聚合保留分组顺序，多分组和重复成员名称不重复统计账号", () => {
    const shared = account({
      groups: [" CODEX ", "codex", "pro"],
      health: "fused",
      routing_state: "fused",
      health_score: 20,
    });
    const other = account({ id: "2", groups: ["pro"], health_score: 80 });
    const data = buildOverviewHealth(
      [shared, other, account({ id: "3", groups: ["excluded"], apply_pending: true })],
      [
        group({ id: "8", name: "pro" }),
        group(),
        group({ name: "excluded", participation_status: "out_of_scope" }),
      ],
    );
    expect(data.healthRows.map((row) => row.group.id)).toEqual(["8", "6"]);
    expect(data.healthRows.map((row) => row.accounts.map((item) => item.id))).toEqual([
      ["1", "2"],
      ["1"],
    ]);
    expect(data.metrics).toMatchObject({
      managedAccounts: 2,
      assignedConcurrency: 16,
      averageHealthScore: 50,
      fusedAccounts: 1,
      riskGroups: 2,
    });
    expect(data.attention.map((item) => item.account.id)).toEqual(["1"]);
  });

  it("无可见分组时总览不纳入未分组或范围外账号", () => {
    const data = buildOverviewHealth(
      [account({ groups: [] }), account({ groups: ["legacy"] })],
      [group({ name: "legacy", participation_status: "out_of_scope" })],
    );
    expect(data.healthRows).toEqual([]);
    expect(data.attention).toEqual([]);
    expect(data.metrics).toMatchObject({
      managedAccounts: 0,
      averageHealthScore: null,
      riskGroups: 0,
    });
  });

  it("名称与对象原型属性相同的分组正常聚合，空分组保留风险卡片", () => {
    const data = buildOverviewHealth(
      [account({ groups: ["__proto__"] })],
      [
        group({ name: "__proto__" }),
        group({ name: "empty", account_count: 0, scheduling_open: 0 }),
      ],
    );
    expect(data.healthRows[0].accounts).toHaveLength(1);
    expect(data.healthRows[1]).toMatchObject({ totalCount: 0, livePercent: 0, tone: "critical" });
    expect(data.metrics.managedAccounts).toBe(1);
  });
  it("等待并发额度的账号单独计数并列为待关注，已有健康分不受容量不足影响", () => {
    const limited = account({
      health: "concurrency_limited",
      routing_state: "concurrency_limited",
      schedulable: false,
      health_score: 98,
      recent_results: [
        {
          result: "失败",
          observed_at: "2026-09-15T00:00:00Z",
          failure_reason: "旧故障",
          latency_ms: null,
          source: "traffic",
        },
      ],
    });

    expect(buildOverviewMetrics([limited], [group()])).toMatchObject({
      concurrencyLimitedAccounts: 1,
      pausedAccounts: 0,
      fusedAccounts: 0,
      unknownAccounts: 0,
      averageHealthScore: 98,
    });
    const attention = buildAttentionAccounts([limited], [group()]);
    expect(attention).toHaveLength(1);
    expect(attention[0].state).toBe("concurrency_limited");
    expect(attention[0].reason).toContain("开启上游共享并发分配后");
  });
  it("策略值与对象原型属性同名时按原始文本显示", () => {
    expect(strategyLabel("__proto__")).toBe("__proto__");
    expect(strategyLabel("constructor")).toBe("constructor");
  });

  it("derives live ratio, group risk and real account health", () => {
    const health = buildGroupHealth(
      group({
        rate_limited_accounts: 1,
        needs_attention: 2,
        scored_accounts: 3,
        average_health_score: 48.6,
      }),
      [
        account(),
        account({ id: "2", health: "degraded", health_status: "degraded" }),
        account({ id: "3", health: "fused", health_status: "fused", routing_state: "fused" }),
        account({ id: "4", health: "unknown", health_status: null, routing_state: null }),
      ],
    );

    expect(health.livePercent).toBe(75);
    expect(health.healthScore).toBe(48.6);
    expect(health.scoredCount).toBe(3);
    expect(health.needsAttention).toBe(2);
    expect(health.rateLimitedCount).toBe(1);
    expect(health.fusedCount).toBe(1);
    expect(health.statusLabel).toBe("部分异常");
  });

  it("marks empty and low-survival groups as critical without dividing by zero", () => {
    expect(buildGroupHealth(group({ account_count: 0, scheduling_open: 0 }), []).livePercent).toBe(
      0,
    );
    expect(buildGroupHealth(group({ account_count: 0, scheduling_open: 0 }), []).tone).toBe(
      "critical",
    );
    expect(buildGroupHealth(group({ scheduling_open: 1 }), []).statusLabel).toBe("仅剩保底");
  });

  it("summarizes concurrency, unknown states and risk groups", () => {
    const metrics = buildOverviewMetrics(
      [
        account(),
        account({
          id: "2",
          concurrency: 12,
          health: "unknown",
          health_status: null,
          routing_state: null,
        }),
      ],
      [group(), group({ name: "pro", id: "8", scheduling_open: 4, scheduling_closed: 0 })],
    );

    expect(metrics.assignedConcurrency).toBe(20);
    expect(metrics.accountsWithConcurrency).toBe(2);
    expect(metrics.averageHealthScore).toBe(100);
    expect(metrics.unknownAccounts).toBe(1);
    expect(metrics.riskGroups).toBe(1);
  });

  it("does not count a pending desired fuse as already effective", () => {
    const metrics = buildOverviewMetrics(
      [account({ desired_health: "fused", decision_state: "fused", apply_pending: true })],
      [group()],
    );

    expect(metrics.healthyAccounts).toBe(1);
    expect(metrics.fusedAccounts).toBe(0);
  });

  it("does not count paused or disabled accounts as fused", () => {
    const metrics = buildOverviewMetrics(
      [
        account({ id: "paused", health: "paused", paused: true, schedulable: false }),
        account({
          id: "disabled",
          health: "disabled",
          routing_state: "disabled",
          schedulable: false,
        }),
        account({ id: "fused", health: "fused", routing_state: "fused", schedulable: false }),
      ],
      [group()],
    );

    expect(metrics.fusedAccounts).toBe(1);
    expect(metrics.pausedAccounts).toBe(1);
    expect(metrics.disabledAccounts).toBe(1);
    expect(
      buildAttentionAccounts(
        [
          account({ id: "paused", health: "paused", paused: true }),
          account({ id: "disabled", health: "disabled" }),
        ],
        [group()],
      ).map((item) => item.state),
    ).toEqual(["disabled", "paused"]);
  });

  it("hides out-of-scope groups and their channels from overview attention", () => {
    const visible = group();
    const excluded = group({
      name: "legacy",
      id: "12",
      participation_status: "out_of_scope",
      participation_reason: "分组 ID 12 位于排除分组列表中",
      status: "excluded",
    });
    const attention = buildAttentionAccounts(
      [
        account({
          id: "1",
          health: "fused",
          routing_state: "fused",
          decision_reason: "连续失败达到熔断线",
        }),
        account({
          id: "2",
          groups: ["legacy"],
          health: "fused",
          routing_state: "fused",
        }),
      ],
      [visible, excluded],
    );

    expect(visibleOverviewGroups([visible, excluded])).toEqual([visible]);
    expect(attention.map((item) => item.account.id)).toEqual(["1"]);
    expect(attention[0]?.reason).toBe("连续失败达到熔断线");
  });

  it("prioritizes failed writeback before fused survivor and degraded channels", () => {
    const attention = buildAttentionAccounts(
      [
        account({ id: "1", health: "degraded" }),
        account({ id: "2", health: "survivor" }),
        account({ id: "3", health: "fused" }),
        account({
          id: "4",
          apply_pending: true,
          apply_error: "写回超时",
        }),
      ],
      [group()],
    );

    expect(attention.map((item) => item.account.id)).toEqual(["4", "3", "2", "1"]);
    expect(attention[0]?.reason).toBe("写回超时");
  });

  it("returns empty metric defaults when there is no data", () => {
    expect(buildOverviewMetrics([], [])).toEqual({
      managedAccounts: 0,
      healthyAccounts: 0,
      degradedAccounts: 0,
      costBlockedAccounts: 0,
      concurrencyLimitedAccounts: 0,
      fusedAccounts: 0,
      survivorAccounts: 0,
      pausedAccounts: 0,
      disabledAccounts: 0,
      unknownAccounts: 0,
      averageHealthScore: null,
      assignedConcurrency: 0,
      accountsWithConcurrency: 0,
      riskGroups: 0,
      criticalGroups: 0,
    });
  });
});
