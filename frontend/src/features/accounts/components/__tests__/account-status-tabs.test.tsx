import { renderToStaticMarkup } from "react-dom/server";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { describe, expect, it, vi } from "vitest";

import { AccountSelectionToolbar, AccountsPage } from "../../../../App";
import type { AccountStatus } from "../../../../api";
import { AccountStatusFilter, accountStatusFilterOptions } from "../account-status-tabs";

function account(id = "11"): AccountStatus {
  return {
    id,
    name: `示例账号 ${id}`,
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
  };
}

describe("AccountStatusFilter", () => {
  it("shows the filter name and selected state using the shared faceted style", () => {
    const markup = renderToStaticMarkup(
      <AccountStatusFilter value="degraded" onValueChange={() => {}} />,
    );

    expect(markup).toContain('aria-label="状态筛选"');
    expect(markup).toContain(">状态<");
    expect(markup).toContain("降级");
    expect(markup).toContain('data-slot="badge"');
    expect(markup).toContain("max-w-64");
    expect(markup).not.toContain("w-32");
    expect(markup).not.toContain("21");
    expect(markup).not.toContain(" · ");
    expect(markup).toContain('data-slot="button"');
    expect(markup).not.toContain('data-slot="select-trigger"');
  });

  it("maps the internal all state to an empty filter without offering an all option", () => {
    const markup = renderToStaticMarkup(
      <AccountStatusFilter value="all" onValueChange={() => {}} />,
    );

    expect(markup).toContain(">状态<");
    expect(markup).not.toContain(">全部<");
    expect(markup).not.toContain('data-slot="badge"');
    expect(accountStatusFilterOptions.map((filter) => filter.value)).not.toContain("all");
  });

  it("places the account filter toolbar above the table card", () => {
    const queryClient = new QueryClient();
    queryClient.setQueryData(["accounts"], []);
    const markup = renderToStaticMarkup(
      <QueryClientProvider client={queryClient}>
        <AccountsPage />
      </QueryClientProvider>,
    );
    const toolbarStart = markup.indexOf('data-testid="account-filter-toolbar"');
    const cardStart = markup.indexOf('data-slot="card"');
    const tableStart = markup.indexOf('data-slot="table"');
    const toolbar = markup.slice(toolbarStart, cardStart);

    expect(toolbarStart).toBeGreaterThan(-1);
    expect(toolbarStart).toBeLessThan(cardStart);
    expect(cardStart).toBeLessThan(tableStart);
    expect(markup).toContain('data-slot="table-filter-toolbar"');
    expect(toolbar).toContain("搜索账号、ID、Host 或分组");
    expect(toolbar).toContain('aria-label="状态筛选"');
    expect(toolbar).not.toContain("排序");
    expect(toolbar).toContain("分组");
    expect(toolbar).toContain("类型");
    expect(toolbar).not.toContain("个账号");
    expect(markup).not.toMatch(/<th[^>]*>分组<\/th>/);
    expect(markup).toContain("调度权重");
    expect(markup).toContain("Key 状态");
    expect(markup).toContain("Sub2API 状态");
    expect(markup).toContain('aria-label="按账号升序排列"');
    expect(markup).toContain('aria-label="按健康分升序排列"');
    expect(markup).toContain('aria-label="按综合延迟升序排列"');
    expect(markup).toContain('aria-label="按账号成本升序排列"');
    expect(markup).toContain('aria-label="按调度权重升序排列"');
    expect(markup).toContain('aria-label="按调度参数升序排列"');
    expect(markup.match(/aria-sort="none"/g)).toHaveLength(6);
    expect(markup).not.toMatch(/<th[^>]*>Base URL 校验<\/th>/);
    expect(markup).toContain("min-w-[1540px]");
    expect(markup).toContain('data-table-panel=""');
    expect(markup).toContain('aria-label="选择当前页账号"');
    expect(markup).toContain('aria-disabled="true"');
  });

  it("keeps maintenance actions independent from selection", () => {
    const queryClient = new QueryClient();
    queryClient.setQueryData(["accounts"], [account()]);
    const markup = renderToStaticMarkup(
      <QueryClientProvider client={queryClient}>
        <AccountsPage />
      </QueryClientProvider>,
    );
    const buttons = [...markup.matchAll(/<button\b[^>]*>[\s\S]*?<\/button>/g)].map(
      (match) => match[0],
    );
    const actionButton = (label: string) => buttons.find((button) => button.includes(label)) ?? "";

    expect(markup).not.toContain("同步全部上游余额");
    expect(markup).toContain("配置校验与修复");
    expect(markup).toContain("同步倍率");
    expect(markup).toContain("复验绑定");
    expect(markup).toContain("命名修复");
    expect(markup).not.toContain("选择筛选结果");
    expect(markup).not.toContain("批量删除（");
    expect(markup).not.toContain("批量操作");
    expect(markup).toContain('aria-label="更多账号操作"');
    for (const label of ["配置校验与修复", "同步倍率", "复验绑定", "命名修复"]) {
      expect(actionButton(label)).not.toContain(' disabled=""');
    }
    expect(markup).toContain('role="checkbox"');
  });

  it("shows selected account count and destructive action in the floating toolbar", () => {
    const markup = renderToStaticMarkup(
      <AccountSelectionToolbar
        selectedCount={3}
        pending={false}
        onClear={vi.fn()}
        onDelete={vi.fn()}
      />,
    );

    expect(markup).toContain('role="toolbar"');
    expect(markup).toContain("3");
    expect(markup).toContain("账号");
    expect(markup).toContain("已选择");
    expect(markup).toContain('aria-label="清空选择"');
    expect(markup).toContain('aria-label="删除已选择的 3 个账号"');
    expect(markup).toContain("fixed");
  });

  it("hides manual-priority accounts by default and exposes an opt-in switch", () => {
    const queryClient = new QueryClient();
    queryClient.setQueryData(
      ["accounts"],
      [account(), { ...account("12"), name: "人工账号", manual_priority: 1 }],
    );
    const markup = renderToStaticMarkup(
      <QueryClientProvider client={queryClient}>
        <AccountsPage />
      </QueryClientProvider>,
    );

    expect(markup).not.toContain("人工账号");
    expect(markup).toContain('aria-label="显示人工优先账号"');
    expect(markup).toContain('aria-checked="false"');
  });

  it("disables bulk rate sync when the filtered result only contains manual-priority accounts", () => {
    const queryClient = new QueryClient();
    queryClient.setQueryData(["accounts"], [{ ...account(), manual_priority: 3 }]);
    const markup = renderToStaticMarkup(
      <QueryClientProvider client={queryClient}>
        <AccountsPage />
      </QueryClientProvider>,
    );
    const syncButton = [...markup.matchAll(/<button\b[^>]*>[\s\S]*?<\/button>/g)]
      .map((match) => match[0])
      .find((button) => button.includes("同步倍率"));

    expect(syncButton).toContain(' disabled=""');
  });

  it("shows 20 accounts on the first page by default", () => {
    const queryClient = new QueryClient();
    queryClient.setQueryData(
      ["accounts"],
      Array.from({ length: 21 }, (_, index) => account(String(index + 1))),
    );
    const markup = renderToStaticMarkup(
      <QueryClientProvider client={queryClient}>
        <AccountsPage />
      </QueryClientProvider>,
    );

    expect(markup).toContain('aria-label="选择账号 示例账号 20（#20）"');
    expect(markup).not.toContain('aria-label="选择账号 示例账号 21（#21）"');
  });

  it("renders accessible page and row selection controls when accounts are available", () => {
    const queryClient = new QueryClient();
    queryClient.setQueryData(["accounts"], [account()]);
    const markup = renderToStaticMarkup(
      <QueryClientProvider client={queryClient}>
        <AccountsPage />
      </QueryClientProvider>,
    );

    expect(markup).toContain('aria-label="选择当前页账号"');
    expect(markup).toContain('aria-label="选择账号 示例账号 11（#11）"');
    expect(markup.match(/role="checkbox"/g)).toHaveLength(2);
  });
});
