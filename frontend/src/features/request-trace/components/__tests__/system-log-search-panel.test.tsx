import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, cleanup } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { SystemLogSearchPanel } from "../system-log-search-panel";

afterEach(cleanup);

it("首次进入时结果区边框与分页可见，筛选标签与控件横向排列", () => {
  const client = new QueryClient({ defaultOptions: { queries: { enabled: false } } });
  const view = render(
    <QueryClientProvider client={client}>
      <SystemLogSearchPanel />
    </QueryClientProvider>,
  );
  expect(screen.getByRole("textbox", { name: "request_id" })).toBeVisible();
  expect(screen.getByRole("combobox", { name: "时间范围" })).toBeVisible();
  expect(screen.getByRole("combobox", { name: "级别" })).toBeVisible();
  const placeholder = screen.getByText("设置筛选条件后点击查询").parentElement;
  expect(placeholder).toHaveClass("min-h-0", "flex-1", "items-center", "justify-center");
  expect(placeholder?.closest("[data-table-panel]")).not.toBeNull();
  expect(screen.getByRole("navigation", { name: "表格分页" })).toBeVisible();
  expect(screen.getByRole("button", { name: "转到下一页" })).toBeDisabled();
  expect(screen.getByRole("textbox", { name: "request_id" }).parentElement).toHaveClass(
    "flex",
    "items-center",
  );
  view.unmount();
  client.clear();
});
