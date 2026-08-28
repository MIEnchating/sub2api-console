import { describe, expect, it } from "vitest";

import type { AccountStatus } from "@/api";
import { accountMatchesPoolFilter, accountPoolCounts, accountPoolState } from "../account-pool";

function account(overrides: Partial<AccountStatus> = {}): AccountStatus {
  return {
    id: "1",
    name: "operator",
    groups: ["default"],
    upstream_host: "api.example.test",
    upstream_type: "apikey",
    schedulable: true,
    priority: 1,
    load_factor: "1",
    concurrency: 10,
    multiplier: "1",
    balance: "20",
    paused: false,
    paused_reason: null,
    routing_state: "healthy",
    health_status: "通过",
    health: "healthy",
    desired_health: null,
    apply_pending: false,
    apply_error: null,
    decision_state: null,
    decision_reason: null,
    failure_streak: 0,
    recovery_pass_streak: 0,
    target_priority: 1,
    target_load_factor: "1",
    target_schedulable: true,
    target_concurrency: 10,
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

describe("accountPoolState", () => {
  it("gives explicit pause and fuse states priority over scheduling", () => {
    expect(accountPoolState(account({ paused: true, schedulable: false })).value).toBe("paused");
    expect(
      accountPoolState(account({ health: "fused", routing_state: "fused", schedulable: false }))
        .value,
    ).toBe("fused");
  });

  it("keeps disabled, paused, and fused as separate states", () => {
    expect(
      accountPoolState(
        account({ health: "disabled", routing_state: "disabled", schedulable: false }),
      ).value,
    ).toBe("disabled");
    expect(
      accountPoolState(account({ health: "paused", paused: true, schedulable: false })).value,
    ).toBe("paused");
    expect(accountPoolState(account({ health: "hard_open", schedulable: false })).value).toBe(
      "fused",
    );
  });

  it("classifies healthy, degraded, and unknown accounts independently from scheduling", () => {
    expect(
      accountPoolState(account({ health: "unknown", routing_state: null, schedulable: false }))
        .value,
    ).toBe("unknown");
    expect(accountPoolState(account()).value).toBe("healthy");
    expect(
      accountPoolState(account({ health: "degraded", routing_state: "observing" })).value,
    ).toBe("degraded");
    expect(accountPoolState(account({ health: "unknown", routing_state: null })).value).toBe(
      "unknown",
    );
  });

  it("uses effective health rather than a pending routing decision", () => {
    expect(
      accountPoolState(
        account({ health: "degraded", decision_state: "degraded", health_score: 6.5 }),
      ).value,
    ).toBe("degraded");
    expect(
      accountPoolState(account({ health: "survivor", decision_state: "survivor" })).value,
    ).toBe("survivor");
    expect(
      accountPoolState(account({ health: "healthy", decision_state: "fused", apply_pending: true }))
        .value,
    ).toBe("healthy");
    expect(
      accountPoolState(account({ health: "excluded", decision_state: "excluded" })).value,
    ).toBe("excluded");
  });

  it("counts one exclusive state per account and filters by that state", () => {
    const accounts = [
      account({ id: "healthy" }),
      account({ id: "degraded", health: "degraded", decision_state: "degraded" }),
      account({ id: "unknown", health: "unknown", routing_state: null, schedulable: false }),
    ];

    expect(accountPoolCounts(accounts)).toMatchObject({
      all: 3,
      healthy: 1,
      degraded: 1,
      unknown: 1,
    });
    expect(accounts.filter((item) => accountMatchesPoolFilter(item, "degraded"))).toHaveLength(1);
  });
});
