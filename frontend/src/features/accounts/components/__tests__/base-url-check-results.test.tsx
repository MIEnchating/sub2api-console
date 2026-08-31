import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import type { AccountStatus } from "@/api";

import { BaseURLCheckResults } from "../base-url-check-results";

function account(index: number): AccountStatus {
  const id = String(index).padStart(2, "0");
  return {
    id,
    name: `账号-${id}`,
    groups: ["默认分组"],
    upstream_id: "up_test",
    upstream_host: "api.example.com",
    upstream_type: "newapi",
    upstream_base_url: "https://api.example.com",
    base_url: "https://api.example.com/v1",
    base_url_check: "matched",
    base_url_check_reason: "账号 Base URL 与上游地址一致",
    platform: "openai",
    schedulable: true,
    priority: 1,
    load_factor: null,
    concurrency: 10,
    multiplier: "1",
    balance: null,
    paused: false,
    paused_reason: null,
    routing_state: null,
    health_status: null,
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
  };
}

describe("BaseURLCheckResults", () => {
  it("puts operations above search and paginates long result lists", () => {
    const markup = renderToStaticMarkup(
      <BaseURLCheckResults
        accounts={Array.from({ length: 25 }, (_, index) => account(index + 1))}
        running={false}
        repairing={false}
        repairingAccountId={null}
        onRerun={() => undefined}
        onRepair={() => undefined}
      />,
    );

    const actions = markup.indexOf('data-slot="page-actions"');
    const search = markup.indexOf('data-slot="table-filter-toolbar"');
    expect(actions).toBeGreaterThan(-1);
    expect(search).toBeGreaterThan(actions);
    expect(markup).toContain("搜索账号、ID、Host 或 Base URL");
    expect(markup).toContain("重新运行");
    expect(markup).toContain("账号-20");
    expect(markup).not.toContain("账号-21");
    expect(markup).toContain("转到第 2 页");
    expect(markup).toContain("min-w-[1120px]");
    expect(markup).toContain("grid h-full min-h-0");
  });

  it("does not repeat result counts above the searchable table", () => {
    const markup = renderToStaticMarkup(
      <BaseURLCheckResults
        accounts={[account(1)]}
        running={false}
        repairing={false}
        repairingAccountId={null}
        onRerun={() => undefined}
        onRepair={() => undefined}
      />,
    );

    expect(markup).not.toContain("账号总数");
    expect(markup).not.toContain("信息不完整");
    expect(markup).not.toContain("项异常");
  });
});
