import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { RouterContextProvider } from "@tanstack/react-router";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { UpstreamsPage } from "../../../../App";
import { router } from "../../../../router";

describe("上游管理筛选工具栏", () => {
  it("类型和状态使用统一筛选菜单且不显示全部选项", () => {
    const queryClient = new QueryClient();
    const markup = renderToStaticMarkup(
      <QueryClientProvider client={queryClient}>
        <RouterContextProvider router={router}>
          <UpstreamsPage />
        </RouterContextProvider>
      </QueryClientProvider>,
    );
    const formStart = markup.indexOf('placeholder="搜索 Host 或名称"');
    const tableStart = markup.indexOf('data-table-panel=""');
    const toolbar = markup.slice(formStart, tableStart);

    expect(formStart).toBeGreaterThan(-1);
    expect(tableStart).toBeGreaterThan(formStart);
    expect(toolbar).toContain('aria-label="类型筛选"');
    expect(toolbar).toContain('aria-label="状态筛选"');
    expect(toolbar.match(/data-slot="button"/g)?.length).toBeGreaterThanOrEqual(4);
    expect(toolbar).not.toContain('data-slot="select-trigger"');
    expect(toolbar).not.toContain("全部类型");
    expect(toolbar).not.toContain("全部状态");
  });
});
