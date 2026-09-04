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
    expect(markup).toContain('aria-label="刷新上游列表"');
    expect(toolbar.match(/data-slot="button"/g)?.length).toBeGreaterThanOrEqual(4);
    expect(toolbar).not.toContain('data-slot="select-trigger"');
    expect(toolbar).not.toContain("全部类型");
    expect(toolbar).not.toContain("全部状态");
  });

  it("supports selecting upstreams and displays the last successful authentication method", () => {
    const queryClient = new QueryClient();
    queryClient.setQueryData(["upstreams"], {
      hosts: [
        {
          upstream_id: "upstream-1",
          host: "api.example.test",
          hosts: ["api.example.test"],
          base_url: "https://api.example.test",
          name: "示例上游",
          upstream_type: "sub2api",
          account_count: 2,
          group_count: 1,
          auth_status: "已鉴权",
          raw_balance: "10",
          balance: "10",
          recharge_rate: "1",
          balance_status: "已读取",
          checked_at: "2026-09-04T00:00:00Z",
          last_auth_success_method: "sub2api_user_token",
          last_auth_recovery_method: "refresh_token",
          last_auth_success_at: "2026-09-04T00:00:00Z",
        },
      ],
      total_hosts: 1,
      authenticated_hosts: 1,
      recovery_required: 0,
      source: "Console 业务库",
    });
    const markup = renderToStaticMarkup(
      <QueryClientProvider client={queryClient}>
        <RouterContextProvider router={router}>
          <UpstreamsPage />
        </RouterContextProvider>
      </QueryClientProvider>,
    );

    expect(markup).toContain('aria-label="选择当前页上游"');
    expect(markup).toContain('aria-label="选择上游 示例上游"');
    expect(markup).toContain("h-20");
    expect(markup).toContain("最近方式：");
    expect(markup).toContain("Token + 刷新 Token");
  });
});
