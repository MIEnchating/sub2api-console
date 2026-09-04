import { describe, expect, it } from "vitest";

import type { AccountStatus } from "@/api";
import {
  accountSortDirection,
  nextAccountSort,
  sortAccounts,
} from "@/features/accounts/lib/account-sort";

function account(id: string, overrides: Partial<AccountStatus> = {}): AccountStatus {
  return {
    id,
    name: `账号 ${id}`,
    groups: ["codex"],
    upstream_id: "upstream-1",
    upstream_host: "api.example.test",
    upstream_type: "newapi",
    schedulable: true,
    priority: 1,
    load_factor: "1",
    concurrency: 10,
    multiplier: "0.1",
    balance: "10",
    paused: false,
    paused_reason: null,
    routing_state: "healthy",
    health_status: "healthy",
    health: "healthy",
    desired_health: "healthy",
    apply_pending: false,
    apply_error: null,
    decision_state: "applied",
    decision_reason: null,
    failure_streak: 0,
    recovery_pass_streak: 0,
    target_priority: 1,
    target_load_factor: "1",
    target_schedulable: true,
    target_concurrency: 10,
    health_score: 100,
    short_score: 100,
    long_score: 100,
    sample_count: 1,
    recent_results: [],
    ttfb_p50_ms: 100,
    ttfb_p95_ms: 200,
    weight: 1,
    ...overrides,
  };
}

describe("sortAccounts", () => {
  it("cycles a column through ascending, descending and default order", () => {
    expect(nextAccountSort("default", "health")).toBe("health_asc");
    expect(nextAccountSort("health_asc", "health")).toBe("health_desc");
    expect(nextAccountSort("health_desc", "health")).toBe("default");
    expect(nextAccountSort("cost_desc", "health")).toBe("health_asc");
    expect(accountSortDirection("health_desc", "health")).toBe("desc");
    expect(accountSortDirection("health_desc", "cost")).toBeNull();
  });

  it("uses the visible manual priority and places missing priorities last in either direction", () => {
    const accounts = [
      account("missing", { priority: null }),
      account("automatic", { priority: 4 }),
      account("manual", { priority: 99, manual_priority: 2 }),
    ];

    expect(sortAccounts(accounts, "priority_asc").map((item) => item.id)).toEqual([
      "manual",
      "automatic",
      "missing",
    ]);
    expect(sortAccounts(accounts, "priority_desc").map((item) => item.id)).toEqual([
      "automatic",
      "manual",
      "missing",
    ]);
  });

  it("sorts account names naturally in both directions", () => {
    const accounts = [
      account("10", { name: "Account 10" }),
      account("2", { name: "Account 2" }),
      account("1", { name: "Account 1" }),
    ];

    expect(sortAccounts(accounts, "name_asc").map((item) => item.id)).toEqual(["1", "2", "10"]);
    expect(sortAccounts(accounts, "name_desc").map((item) => item.id)).toEqual(["10", "2", "1"]);
  });

  it("sorts decimal account costs numerically and keeps invalid values last", () => {
    const accounts = [
      account("invalid", { multiplier: "not-a-number" }),
      account("expensive", { multiplier: "0.12" }),
      account("cheap", { multiplier: "0.2" }),
    ];

    expect(sortAccounts(accounts, "cost_desc").map((item) => item.id)).toEqual([
      "cheap",
      "expensive",
      "invalid",
    ]);
  });

  it("preserves source order for the default option without mutating the source array", () => {
    const accounts = [account("2"), account("1")];
    const sorted = sortAccounts(accounts, "default");

    expect(sorted.map((item) => item.id)).toEqual(["2", "1"]);
    expect(sorted).not.toBe(accounts);
    expect(accounts.map((item) => item.id)).toEqual(["2", "1"]);
  });
});
