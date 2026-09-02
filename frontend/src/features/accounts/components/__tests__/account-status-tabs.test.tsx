import { renderToStaticMarkup } from "react-dom/server";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { describe, expect, it } from "vitest";

import { AccountsPage } from "../../../../App";
import { AccountStatusFilter, accountStatusFilterOptions } from "../account-status-tabs";

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
    expect(toolbar).toContain("分组");
    expect(toolbar).toContain("类型");
    expect(toolbar).not.toContain("个账号");
    expect(markup).not.toMatch(/<th[^>]*>分组<\/th>/);
    expect(markup).toContain("调度权重");
    expect(markup).toContain("配置校验与修复");
    expect(markup).toContain("Key 状态");
    expect(markup).toContain("Sub2API 状态");
    expect(markup).not.toMatch(/<th[^>]*>Base URL 校验<\/th>/);
    expect(markup).toContain("min-w-[1500px]");
    expect(markup).toContain('data-table-panel=""');
  });

  it("shows balance sync, batch revalidation, and name repair as page-level actions", () => {
    const queryClient = new QueryClient();
    queryClient.setQueryData(["accounts"], []);
    const markup = renderToStaticMarkup(
      <QueryClientProvider client={queryClient}>
        <AccountsPage />
      </QueryClientProvider>,
    );

    expect(markup).toContain("同步余额");
    expect(markup).toContain("批量复验");
    expect(markup).toContain("同步倍率");
    expect(markup).toContain("命名修复");
    expect(markup).toContain("配置校验与修复");
    expect(markup).not.toContain(">参数修复<");
    expect(markup).not.toContain('type="checkbox"');
  });
});
