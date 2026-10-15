import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";

import { SystemLogSearchPanel } from "../system-log-search-panel";

const clients: QueryClient[] = [];
beforeEach(() => vi.stubGlobal("PointerEvent", MouseEvent));
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  vi.unstubAllGlobals();
});

function mountSearch(): URLSearchParams[] {
  const queries: URLSearchParams[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn((input: RequestInfo | URL) => {
      const query = new URL(String(input), "http://localhost").searchParams;
      queries.push(query);
      return Promise.resolve(
        Response.json({
          items: [],
          total: 21,
          page: Number(query.get("page")),
          page_size: Number(query.get("page_size")),
        }),
      );
    }),
  );
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  clients.push(client);
  render(
    <QueryClientProvider client={client}>
      <SystemLogSearchPanel />
    </QueryClientProvider>,
  );
  return queries;
}

it("不输入请求 ID 时可按默认最近一小时查询全部级别", async () => {
  const queries = mountSearch();
  expect(screen.getByRole("combobox", { name: "时间范围" })).toHaveTextContent("最近 1 小时");
  expect(screen.getByRole("combobox", { name: "级别" })).toHaveTextContent("全部级别");
  await userEvent.click(screen.getByRole("button", { name: "查询" }));
  await waitFor(() => expect(queries).toHaveLength(1));
  expect(Object.fromEntries(queries[0])).toMatchObject({
    request_id: "",
    time_range: "1h",
    page: "1",
  });
  expect(queries[0].has("level")).toBe(false);
});

it("选择时间范围和级别后提交筛选，编辑草稿不改变翻页条件", async () => {
  const queries = mountSearch();
  const user = userEvent.setup();
  await user.click(screen.getByRole("combobox", { name: "时间范围" }));
  await user.click(screen.getByRole("option", { name: "最近 7 天" }));
  await user.click(screen.getByRole("combobox", { name: "级别" }));
  await user.click(screen.getByRole("option", { name: "错误（error）" }));
  await user.type(screen.getByRole("textbox", { name: "request_id" }), " req-42 ");
  await user.click(screen.getByRole("button", { name: "查询" }));
  await screen.findByText("当前页没有系统日志");
  expect(Object.fromEntries(queries[0])).toMatchObject({
    request_id: "req-42",
    time_range: "7d",
    level: "error",
  });
  screen.getByRole("combobox", { name: "级别" }).focus();
  await user.keyboard("{Enter}");
  await user.click(await screen.findByRole("option", { name: "警告（warn）" }));
  await user.click(screen.getByRole("button", { name: "转到下一页" }));
  await waitFor(() => expect(queries).toHaveLength(2));
  expect(Object.fromEntries(queries[1])).toMatchObject({
    time_range: "7d",
    level: "error",
    page: "2",
  });
  await user.click(screen.getByRole("button", { name: "查询" }));
  await waitFor(() => expect(queries).toHaveLength(3));
  expect(Object.fromEntries(queries[2])).toMatchObject({
    time_range: "7d",
    level: "warn",
    page: "1",
  });
});

it("键盘选择级别后重置会恢复全部筛选默认值并移除结果", async () => {
  const queries = mountSearch();
  const user = userEvent.setup();
  const level = screen.getByRole("combobox", { name: "级别" });
  level.focus();
  await user.keyboard("{Enter}");
  expect(level).toHaveAttribute("aria-expanded", "true");
  await user.keyboard("{End}{Enter}");
  expect(level).toHaveTextContent("错误（error）");
  screen.getByRole("combobox", { name: "时间范围" }).focus();
  await user.keyboard("{Enter}");
  await user.click(await screen.findByRole("option", { name: "最近 30 天" }));
  await user.type(screen.getByRole("textbox", { name: "request_id" }), "req-reset");
  await user.click(screen.getByRole("button", { name: "查询" }));
  await screen.findByText("当前页没有系统日志");
  await user.click(screen.getByRole("button", { name: "重置" }));
  expect(level).toHaveTextContent("全部级别");
  expect(screen.getByRole("combobox", { name: "时间范围" })).toHaveTextContent("最近 1 小时");
  expect(screen.getByRole("textbox", { name: "request_id" })).toHaveValue("");
  expect(screen.getByRole("navigation", { name: "表格分页" })).toBeVisible();
  expect(screen.getByRole("button", { name: "转到下一页" })).toBeDisabled();
  expect(screen.getByText("设置筛选条件后点击查询")).toBeVisible();
  expect(queries).toHaveLength(1);
});
