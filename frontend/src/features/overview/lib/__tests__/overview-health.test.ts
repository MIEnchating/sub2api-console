import { describe, expect, it } from "vitest";

import type { AccountStatus, GroupStatus } from "@/api";
import {
  buildAttentionAccounts,
  buildGroupHealth,
  buildOverviewMetrics,
  visibleOverviewGroups,
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
