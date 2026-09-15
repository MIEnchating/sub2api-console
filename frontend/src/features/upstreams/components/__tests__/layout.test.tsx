import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { RouterContextProvider } from "@tanstack/react-router";
import { within } from "@testing-library/react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { UpstreamsPage } from "../../../../App";
import { router } from "../../../../router";

describe("上游管理列表布局", () => {
  it("新增并发列后为状态、余额、并发和操作保留独立列宽", () => {
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
          concurrency_limit: 20,
          concurrency_status: "known",
          allocated_concurrency: 10,
          target_concurrency: 12,
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

    const container = document.createElement("div");
    container.innerHTML = markup;
    const view = within(container);
    expect(view.getByRole("table")).toHaveClass("min-w-[1440px]");
    expect(view.getByRole("columnheader", { name: "状态" })).toHaveClass("w-[16%]");
    expect(view.getByRole("columnheader", { name: "余额" })).toHaveClass("w-[10%]");
    expect(view.getByRole("columnheader", { name: /并发/ })).toHaveClass("w-[12%]");
    expect(view.getByRole("columnheader", { name: "操作" })).toHaveClass("w-36");
    expect(view.getAllByRole("columnheader")).toHaveLength(10);
    expect(view.getAllByRole("cell")).toHaveLength(10);
    expect(view.getByLabelText("已配置并发")).toHaveTextContent("10");
    expect(header).toContain('aria-label="上游并发说明"');
    expect(authStatusStart).toBeGreaterThan(-1);
    expect(authStatusCell).toContain("overflow-hidden");
    expect(authStatusCell).toContain("truncate");
    queryClient.clear();
  });

  it("初次读取上游时骨架为并发列保留相同数量的单元格", () => {
    const queryClient = new QueryClient();
    const markup = renderToStaticMarkup(
      <QueryClientProvider client={queryClient}>
        <RouterContextProvider router={router}>
          <UpstreamsPage />
        </RouterContextProvider>
      </QueryClientProvider>,
    );
    const container = document.createElement("div");
    container.innerHTML = markup;
    const row = within(container).getAllByRole("row", { name: "正在加载数据" })[0];

    expect(row).not.toBeNull();
    expect(within(row).getAllByRole("cell")).toHaveLength(10);
    queryClient.clear();
  });

  it("上游列表为空时空状态覆盖包含并发列在内的所有列", () => {
    const queryClient = new QueryClient();
    queryClient.setQueryData(["upstreams"], { hosts: [] });
    const markup = renderToStaticMarkup(
      <QueryClientProvider client={queryClient}>
        <RouterContextProvider router={router}>
          <UpstreamsPage />
        </RouterContextProvider>
      </QueryClientProvider>,
    );
    const container = document.createElement("div");
    container.innerHTML = markup;

    expect(within(container).getByRole("cell")).toHaveAttribute("colspan", "10");
    expect(within(container).getByRole("cell")).toHaveTextContent("当前业务库没有上游 Host");
    queryClient.clear();
  });
});
