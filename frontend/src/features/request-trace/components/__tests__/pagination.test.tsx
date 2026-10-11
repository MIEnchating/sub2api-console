import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";

import type { SystemLogPage, UsageRecord } from "@/api";
import { SystemLogSearchPanel } from "../system-log-search-panel";

const clients: QueryClient[] = [];
afterEach(() => {
  cleanup();
  for (const client of clients.splice(0)) client.clear();
  vi.unstubAllGlobals();
});

const record: UsageRecord = {
  id: 1,
  request_id: "request-page-test",
  account_id: null,
  account_name: null,
  group_name: null,
  is_error: false,
  error_reason: null,
  first_token_ms: null,
  duration_ms: null,
  summary: "HTTP request completed",
  observed_at: null,
  source: "system-log",
  payload: {},
};

function mountSearch(): void {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  clients.push(client);
  render(
    <QueryClientProvider client={client}>
      <SystemLogSearchPanel />
    </QueryClientProvider>,
  );
}

it.each([1, 0])("第二页变空且总数降为 %i 时仍能返回前页", async (total) => {
  const firstPage: SystemLogPage = { page: 1, page_size: 20, total: 21, items: [record] };
  const fetch = vi
    .fn()
    .mockResolvedValueOnce(Response.json(firstPage))
    .mockResolvedValueOnce(Response.json({ ...firstPage, page: 2, total, items: [] }))
    .mockResolvedValueOnce(Response.json({ ...firstPage, total, items: total ? [record] : [] }));
  vi.stubGlobal("fetch", fetch);
  const user = userEvent.setup();
  mountSearch();
  await user.type(screen.getByRole("textbox", { name: "request_id" }), "request-page-test");
  await user.click(screen.getByRole("button", { name: "查询" }));
  await screen.findByText("request-page-test");
  await user.click(screen.getByRole("button", { name: "转到下一页" }));

  await screen.findByText(total ? "当前页没有系统日志" : "没有匹配的系统日志");
  const pagination = screen.getByRole("navigation", { name: "表格分页" });
  expect(within(pagination).getByRole("button", { name: "转到下一页" })).toBeDisabled();
  await user.click(within(pagination).getByRole("button", { name: "转到上一页" }));
  expect(await screen.findByRole("button", { name: "转到上一页" })).toBeDisabled();
  await waitFor(() => expect(fetch).toHaveBeenCalledTimes(3));
  expect(
    new URL(fetch.mock.calls[2][0] as string, "http://localhost").searchParams.get("page"),
  ).toBe("1");
  if (total) expect(screen.getByText("request-page-test")).toBeVisible();
});

it("首次查询没有结果时仍可更改每页行数并从首页重新查询", async () => {
  const fetch = vi.fn((input: RequestInfo | URL) => {
    const url = new URL(String(input), "http://localhost");
    return Promise.resolve(
      Response.json({
        page: Number(url.searchParams.get("page")),
        page_size: Number(url.searchParams.get("page_size")),
        total: 0,
        items: [],
      }),
    );
  });
  vi.stubGlobal("fetch", fetch);
  const user = userEvent.setup();
  mountSearch();
  await user.type(screen.getByRole("textbox", { name: "request_id" }), "request-page-test");
  await user.click(screen.getByRole("button", { name: "查询" }));
  await screen.findByText("没有匹配的系统日志");

  await user.click(screen.getByRole("combobox", { name: "每页行数" }));
  await user.click(await screen.findByRole("option", { name: /^10$/ }));
  expect(await screen.findByRole("combobox", { name: "每页行数" })).toHaveTextContent("10");
  await waitFor(() => expect(fetch).toHaveBeenCalledTimes(2));
  const query = new URL(fetch.mock.calls[1][0] as string, "http://localhost").searchParams;
  expect(query.get("page")).toBe("1");
  expect(query.get("page_size")).toBe("10");
  expect(query.get("request_id")).toBe("request-page-test");
});
