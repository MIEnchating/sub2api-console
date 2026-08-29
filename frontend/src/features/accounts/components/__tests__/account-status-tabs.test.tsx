import { renderToStaticMarkup } from "react-dom/server";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { describe, expect, it } from "vitest";

import { AccountsPage } from "../../../../App";
import { AccountStatusFilter } from "../account-status-tabs";

describe("AccountStatusFilter", () => {
  it("renders only the selected account state without duplicate counts", () => {
    const markup = renderToStaticMarkup(
      <AccountStatusFilter value="degraded" onValueChange={() => {}} />,
    );

    expect(markup).toContain('aria-label="账号状态"');
    expect(markup).toContain("降级");
    expect(markup).not.toContain("21");
    expect(markup).not.toContain(" · ");
    expect(markup).toContain('data-slot="select-trigger"');
    expect(markup).not.toContain('role="tablist"');
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
    expect(toolbar).toContain('aria-label="账号状态"');
    expect(toolbar).toContain('data-slot="select-trigger"');
    expect(toolbar).toContain("分组");
    expect(toolbar).toContain("类型");
    expect(toolbar).not.toContain("个账号");
    expect(markup).not.toMatch(/<th[^>]*>分组<\/th>/);
    expect(markup).toContain("最终权重");
    expect(markup).toContain("min-w-[1240px]");
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
    expect(markup).not.toContain('type="checkbox"');
  });
});
