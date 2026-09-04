import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { RouterContextProvider } from "@tanstack/react-router";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { UpstreamsPage } from "../../../../App";
import { router } from "../../../../router";

describe("上游管理列表布局", () => {
  it("新增选择列后为状态、余额和操作保留互不挤占的列宽", () => {
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
          last_auth_success_method: "newapi_admin_key",
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

    const headerStart = markup.indexOf('data-slot="table-header"');
    const bodyStart = markup.indexOf('data-slot="table-body"');
    const header = markup.slice(headerStart, bodyStart);
    const authStatusStart = markup.indexOf('data-slot="upstream-auth-status"');
    const authStatusCell = markup.slice(authStatusStart, markup.indexOf("</td>", authStatusStart));

    expect(markup).toContain("min-w-[1240px]");
    expect(header).toMatch(/class="[^"]*w-\[22%\][^"]*"[^>]*>状态<\/th>/);
    expect(header).toMatch(/class="[^"]*w-\[11%\][^"]*"[^>]*>余额<\/th>/);
    expect(header).toMatch(/class="[^"]*w-\[12%\][^"]*"[^>]*>操作<\/th>/);
    expect(authStatusStart).toBeGreaterThan(-1);
    expect(authStatusCell).toContain("overflow-hidden");
    expect(authStatusCell).toContain("truncate");
  });
});
