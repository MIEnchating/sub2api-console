import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { RouterContextProvider } from "@tanstack/react-router";
import { within } from "@testing-library/react";
import { renderToStaticMarkup } from "react-dom/server";
import { expect, it } from "vitest";

import { UpstreamsPage } from "../../../../App";
import type { UpstreamSummary } from "../../../../api";
import { router } from "../../../../router";

it.each([
  { balance: "20", expected: "20.00", condition: "为正数" },
  { balance: "-0.125", expected: "-0.125", condition: "为负数" },
  { balance: "0", expected: "0.00", condition: "为零" },
  { balance: null, expected: "未返回余额", condition: "缺失" },
])("上游原生显示余额存在且换算后余额$condition时显示$expected", (fixture) => {
  const queryClient = new QueryClient();
  const summary: UpstreamSummary = {
    hosts: [
      {
        upstream_id: "upstream-1",
        host: "api.example.test",
        hosts: ["api.example.test"],
        base_url: "https://api.example.test",
        name: "示例上游",
        upstream_type: "newapi",
        account_count: 1,
        group_count: 1,
        auth_status: "已鉴权",
        raw_balance: "100",
        balance: fixture.balance,
        display_balance: "730",
        balance_unit: "cny",
        recharge_rate: "5",
        balance_status: "未返回余额",
        checked_at: null,
      },
    ],
    total_hosts: 1,
    authenticated_hosts: 1,
    recovery_required: 0,
    source: "Console 业务库",
  };
  queryClient.setQueryData(["upstreams"], summary);
  try {
    const markup = renderToStaticMarkup(
      <QueryClientProvider client={queryClient}>
        <RouterContextProvider router={router}>
          <UpstreamsPage />
        </RouterContextProvider>
      </QueryClientProvider>,
    );
    const container = document.createElement("div");
    container.innerHTML = markup;
    const row = within(container).getByRole("row", { name: /示例上游/ });

    expect(within(row).getByRole("cell", { name: fixture.expected })).toHaveTextContent(
      fixture.expected,
    );
    expect(row).not.toHaveTextContent("CN¥730.00");
  } finally {
    queryClient.clear();
  }
});
