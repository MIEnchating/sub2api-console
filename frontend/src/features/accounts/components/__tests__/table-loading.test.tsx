import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { within } from "@testing-library/react";
import { renderToStaticMarkup } from "react-dom/server";
import { expect, it } from "vitest";

import { AccountsPage } from "@/App";

it("账号首次读取时骨架保留十一列及固定操作列，不将整行合并", () => {
  const client = new QueryClient();
  const container = document.createElement("div");
  container.innerHTML = renderToStaticMarkup(
    <QueryClientProvider client={client}>
      <AccountsPage />
    </QueryClientProvider>,
  );

  expect(within(container).getByRole("table")).toHaveAttribute("data-action-column", "true");
  const rows = within(container).getAllByRole("row", { name: "正在加载账号" });
  expect(rows).toHaveLength(6);
  for (const row of rows) {
    const cells = within(row).getAllByRole("cell");
    expect(cells).toHaveLength(11);
    expect(cells[10]).not.toHaveAttribute("colspan");
  }
  client.clear();
});
