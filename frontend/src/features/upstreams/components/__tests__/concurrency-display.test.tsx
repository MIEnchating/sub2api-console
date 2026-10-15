import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { RouterContextProvider } from "@tanstack/react-router";
import { within } from "@testing-library/react";
import { renderToStaticMarkup } from "react-dom/server";
import { expect, it } from "vitest";

import { UpstreamsPage } from "@/App";
import { router } from "@/router";

it("混合上游列表中 New API 并发显示占位符，Sub2API 保留未读取状态", () => {
  const queryClient = new QueryClient();
  queryClient.setQueryData(["upstreams"], {
    hosts: ["newapi", "sub2api"].map((type) => ({
      upstream_id: type,
      upstream_type: type,
      name: `${type} 示例上游`,
      host: `${type}.example.test`,
      hosts: [`${type}.example.test`],
      base_url: `https://${type}.example.test`,
      account_count: 1,
      group_count: 1,
      auth_status: "已鉴权",
      balance: "10",
      balance_status: "已读取",
      concurrency_limit: null,
      concurrency_status: "unknown",
      allocated_concurrency: null,
      target_concurrency: null,
    })),
  });

  try {
    const container = document.createElement("div");
    container.innerHTML = renderToStaticMarkup(
      <QueryClientProvider client={queryClient}>
        <RouterContextProvider router={router}>
          <UpstreamsPage />
        </RouterContextProvider>
      </QueryClientProvider>,
    );
    const view = within(container);
    const columnIndex = view
      .getAllByRole("columnheader")
      .indexOf(view.getByRole("columnheader", { name: /并发/ }));
    const newapiRow = within(view.getByRole("row", { name: /newapi 示例上游/ }));
    const sub2apiRow = within(view.getByRole("row", { name: /sub2api 示例上游/ }));

    expect(newapiRow.getAllByRole("cell")[columnIndex]).toHaveTextContent(/^—$/);
    expect(newapiRow.queryByRole("group", { name: "上游并发" })).not.toBeInTheDocument();
    expect(sub2apiRow.getAllByRole("cell")[columnIndex]).toHaveTextContent(/^未读取$/);
  } finally {
    queryClient.clear();
  }
});
