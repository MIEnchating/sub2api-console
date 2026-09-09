import { renderToStaticMarkup } from "react-dom/server";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterAll, beforeEach, afterEach, describe, expect, it, vi } from "vitest";

// JSDOM 26 的样式匹配不支持浏览器顶层选择器，会在菜单焦点计算时递归。
const restoreSelectorMatching = vi.hoisted(() => {
  const matches = Element.prototype.matches;
  Element.prototype.matches = function (selector: string): boolean {
    if ([":fullscreen", ":popover-open", ":modal"].includes(selector)) return false;
    return matches.call(this, selector);
  };
  return () => {
    Element.prototype.matches = matches;
  };
});
afterAll(restoreSelectorMatching);
beforeEach(() => {
  const getComputedStyle = window.getComputedStyle;
  vi.spyOn(window, "getComputedStyle").mockImplementation((element, pseudoElement) => {
    if (element instanceof HTMLSelectElement) {
      const style = document.createElement("div").style;
      style.display = "none";
      return style;
    }
    return getComputedStyle(element, pseudoElement);
  });
});

import { AccountSelectionToolbar, AccountTaskCancelButton, AccountsPage } from "../../../../App";
import { api, type AccountStatus } from "../../../../api";
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
  afterEach(() => {
    vi.restoreAllMocks();
  });

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
    expect(toolbar).toContain("平台");
    expect(toolbar).not.toContain("个账号");
    expect(markup).not.toMatch(/<th[^>]*>分组<\/th>/);
    expect(markup).toContain("调度权重");
    expect(markup).toMatch(/<th[^>]*>状态<\/th>/);
    expect(markup).not.toMatch(/<th[^>]*>Key 状态<\/th>/);
    expect(markup).not.toMatch(/<th[^>]*>Sub2API 状态<\/th>/);
    expect(markup).toContain('aria-label="按账号升序排列"');
    expect(markup).toContain('aria-label="按健康分升序排列"');
    expect(markup).toContain('aria-label="按流量首字升序排列"');
    expect(markup).toContain('aria-label="按账号成本升序排列"');
    expect(markup).toContain('aria-label="按调度权重升序排列"');
    expect(markup).toContain('aria-label="按调度参数升序排列"');
    expect(markup.match(/aria-sort="none"/g)).toHaveLength(6);
    expect(markup).not.toMatch(/<th[^>]*>Base URL 校验<\/th>/);
    expect(markup).toContain("min-w-[1336px]");
    expect(markup).toContain('data-table-panel=""');
    expect(markup).toContain('aria-label="选择当前页账号"');
    expect(markup).toContain('aria-disabled="true"');
  });

  it("未选择账号时仍可从维护菜单操作当前筛选结果", async () => {
    vi.spyOn(api, "accounts").mockResolvedValue([account()]);
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    queryClient.setQueryData(["accounts"], [account()]);
    render(
      <QueryClientProvider client={queryClient}>
        <AccountsPage />
      </QueryClientProvider>,
    );
    expect(screen.getByRole("checkbox", { name: "选择当前页账号" })).not.toBeChecked();
    expect(screen.queryByRole("button", { name: "配置校验与修复" })).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "账号维护" }));
    for (const name of ["配置校验与修复", "同步倍率", "同步模型", "复验绑定", "命名修复"]) {
      expect(await screen.findByRole("menuitem", { name })).not.toHaveAttribute(
        "aria-disabled",
        "true",
      );
    }
    queryClient.clear();
  });

  it("把刷新账号池按钮放在账号管理操作栏最前面", () => {
    const queryClient = new QueryClient();
    queryClient.setQueryData(["accounts"], [account()]);
    const markup = renderToStaticMarkup(
      <QueryClientProvider client={queryClient}>
        <AccountsPage />
      </QueryClientProvider>,
    );

    const refreshPosition = markup.indexOf('aria-label="刷新账号池"');
    const probePosition = markup.indexOf("平台模型探活");

    expect(refreshPosition).toBeGreaterThan(-1);
    expect(refreshPosition).toBeLessThan(probePosition);
  });

  it("从账号管理入口直接打开平台模型探活", () => {
    const queryClient = new QueryClient();
    queryClient.setQueryData(["accounts"], [account()]);
    render(
      <QueryClientProvider client={queryClient}>
        <AccountsPage />
      </QueryClientProvider>,
    );

    fireEvent.click(screen.getByRole("button", { name: "平台模型探活" }));
    expect(screen.getByRole("dialog", { name: "平台模型探活" })).toBeVisible();
  });

  it("shows selected account count and destructive action in the floating toolbar", () => {
    const markup = renderToStaticMarkup(
      <AccountSelectionToolbar
        selectedCount={3}
        pending={false}
        onClear={vi.fn()}
        onSyncModels={vi.fn()}
        onProbe={vi.fn()}
        onDelete={vi.fn()}
      />,
    );

    expect(markup).toContain('role="toolbar"');
    expect(markup).toContain("3");
    expect(markup).toContain("账号");
    expect(markup).toContain("已选择");
    expect(markup).toContain('aria-label="清空选择"');
    expect(markup).toContain('aria-label="同步已选择的 3 个账号模型"');
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

  it("只有人工优先账号时维护菜单禁用批量倍率同步", async () => {
    const rows = [{ ...account(), manual_priority: 3 }];
    vi.spyOn(api, "accounts").mockResolvedValue(rows);
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    queryClient.setQueryData(["accounts"], rows);
    render(
      <QueryClientProvider client={queryClient}>
        <AccountsPage />
      </QueryClientProvider>,
    );
    fireEvent.click(screen.getByRole("button", { name: "账号维护" }));
    expect(await screen.findByRole("menuitem", { name: "同步倍率" })).toHaveAttribute(
      "aria-disabled",
      "true",
    );
    queryClient.clear();
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

  it("账号探活运行时不增加取消按钮", () => {
    const queryClient = new QueryClient();
    render(
      <QueryClientProvider client={queryClient}>
        <AccountTaskCancelButton taskId="probe-task-11" pending activeAction="探活测试" />
      </QueryClientProvider>,
    );
    expect(screen.queryByRole("button", { name: "取消任务" })).not.toBeInTheDocument();
  });

  it("同步账号倍率任务提供取消入口，删除任务不显示行内取消", async () => {
    const cancel = vi.spyOn(api, "cancelTask").mockResolvedValue({ cancelled: true });
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
    });

    const view = render(
      <QueryClientProvider client={queryClient}>
        <AccountTaskCancelButton taskId="rate-task-11" pending activeAction="同步账号倍率" />
      </QueryClientProvider>,
    );

    const cancelButton = screen.getByRole("button", { name: "取消任务" });
    fireEvent.click(cancelButton);

    await waitFor(() => expect(cancel).toHaveBeenCalledWith("rate-task-11"));

    view.rerender(
      <QueryClientProvider client={queryClient}>
        <AccountTaskCancelButton taskId="delete-task-11" pending activeAction="删除账号" />
      </QueryClientProvider>,
    );
    expect(screen.queryByRole("button", { name: "取消任务" })).not.toBeInTheDocument();
  });
});
