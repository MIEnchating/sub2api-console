import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import type { AccountStatus, GroupStatus, RunEvent } from "@/api";
import { OverviewPage } from "../overview-page";

function account(): AccountStatus {
  return {
    id: "42",
    name: "需要处理的渠道",
    groups: ["codex"],
    upstream_id: "up_test",
    upstream_host: "api.example.com",
    upstream_type: "apikey",
    platform: "openai",
    account_type: "apikey",
    schedulable: false,
    priority: 10,
    load_factor: "1",
    concurrency: 8,
    multiplier: "0.15",
    balance: "10",
    paused: false,
    paused_reason: null,
    routing_state: "fused",
    health_status: "fused",
    health: "fused",
    desired_health: "fused",
    apply_pending: false,
    apply_error: null,
    decision_state: "fused",
    decision_reason: "连续失败达到熔断线",
    failure_streak: 3,
    recovery_pass_streak: 0,
    target_priority: null,
    target_load_factor: null,
    target_schedulable: false,
    target_concurrency: null,
    health_score: 12,
    short_score: 10,
    long_score: 20,
    sample_count: 3,
    recent_results: [
      {
        result: "失败",
        observed_at: "2026-08-26T08:00:00Z",
        latency_ms: 1200,
        failure_reason: "HTTP 503",
        source: "traffic",
      },
    ],
    ttfb_p50_ms: 1200,
    ttfb_p95_ms: 1200,
    weight: 0,
  };
}

function group(overrides: Partial<GroupStatus> = {}): GroupStatus {
  return {
    name: "codex",
    id: "6",
    platform: "openai",
    platforms: ["openai"],
    account_count: 1,
    scheduling_open: 0,
    scheduling_closed: 1,
    scheduling_unknown: 0,
    strategy: "speed_first",
    strategy_source: "global_default",
    participation_status: "participating",
    participation_reason: null,
    status: "all_fused",
    ...overrides,
  };
}

describe("OverviewPage", () => {
  it("uses the global page, card, and responsive health matrix layout contract", () => {
    const queryClient = new QueryClient({
      defaultOptions: { queries: { enabled: false, retry: false } },
    });
    const markup = renderToStaticMarkup(
      <QueryClientProvider client={queryClient}>
        <OverviewPage
          onOpenAccounts={() => undefined}
          onOpenEvents={() => undefined}
          onOpenGroups={() => undefined}
        />
      </QueryClientProvider>,
    );

    expect(markup).toContain("受管渠道");
    expect(markup).toContain("平均健康分");
    expect(markup).toContain("已分配并发");
    expect(markup).toContain("风险分组");
    expect(markup).toContain("立即同步");
    expect(markup).toContain("分组管理");
    expect(markup).toContain('data-slot="page-heading"');
    expect(markup).toContain('data-slot="card"');
    expect(markup).toContain('data-slot="card-header"');
    expect(markup).toContain('data-slot="card-content"');
    expect(markup).toContain('data-testid="group-health-grid"');
    expect(markup).toContain("md:grid-cols-2");
    expect(markup).toContain("xl:grid-cols-3");
    expect(markup).not.toContain("border-y");
  });

  it("renders attention and recent events while hiding excluded groups", () => {
    const queryClient = new QueryClient({
      defaultOptions: { queries: { enabled: false, retry: false } },
    });
    const event: RunEvent = {
      id: 7,
      event_type: "routing.writeback",
      created_at: "2026-08-26T08:01:00Z",
      status: "failed",
      summary: "渠道写回失败",
      payload: { account_id: "42" },
    };
    queryClient.setQueryData(["accounts"], [account()]);
    queryClient.setQueryData(
      ["groups"],
      [
        group({
          account_count: 2,
          rate_limited_accounts: 1,
          needs_attention: 1,
          scored_accounts: 1,
          average_health_score: 12,
        }),
        group({
          name: "排除分组",
          id: "12",
          participation_status: "out_of_scope",
          participation_reason: "分组 ID 12 位于排除分组列表中",
          status: "excluded",
        }),
      ],
    );
    queryClient.setQueryData(["overview-events"], [event]);

    const markup = renderToStaticMarkup(
      <QueryClientProvider client={queryClient}>
        <OverviewPage
          onOpenAccounts={() => undefined}
          onOpenEvents={() => undefined}
          onOpenGroups={() => undefined}
        />
      </QueryClientProvider>,
    );

    expect(markup).toContain("需要关注的渠道");
    expect(markup).toContain("需要处理的渠道");
    expect(markup).toContain("#42");
    expect(markup).toContain("OpenAI");
    expect(markup).toContain("API Key");
    expect(markup).toContain("连续失败达到熔断线");
    expect(markup).toContain('data-slot="account-health-score"');
    expect(markup).toContain("健康分 12");
    expect(markup).toContain("短期 10");
    expect(markup).toContain("长期 20");
    expect(markup).toContain("1 个渠道需要处理");
    expect(markup).toContain("1 个限流中（会自愈）");
    expect(markup).toContain("1/2 有评分");
    expect(markup).toContain("最近事件");
    expect(markup).toContain("渠道写回失败");
    expect(markup).toContain('data-slot="status-badge"');
    expect(markup).toContain('data-slot="account-recent-results"');
    expect(markup).not.toContain("排除分组");
  });
});
