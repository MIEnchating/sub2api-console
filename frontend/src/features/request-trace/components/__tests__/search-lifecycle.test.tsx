import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";

import type { SystemLogPage } from "@/api";
import { SystemLogSearchPanel } from "../system-log-search-panel";

const clients: QueryClient[] = [];
afterEach(() => {
  cleanup();
  for (const client of clients.splice(0)) client.clear();
  vi.unstubAllGlobals();
});

function page(requestId: string): SystemLogPage {
  return {
    page: 1,
    page_size: 20,
    total: 1,
    items: [
      {
        id: 1,
        request_id: requestId,
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
      },
    ],
  };
}

function mountSearch(): void {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  clients.push(client);
  render(
    <QueryClientProvider client={client}>
      <SystemLogSearchPanel />
    </QueryClientProvider>,
  );
}

function submit(requestId = "request-1"): void {
  fireEvent.change(screen.getByRole("textbox", { name: "request_id" }), {
    target: { value: requestId },
  });
  fireEvent.click(screen.getByRole("button", { name: "查询" }));
}

it("查询尚未返回时重置仍可用，重置取消请求且迟到结果不再显示", async () => {
  let signal: AbortSignal | null | undefined;
  let release!: (response: Response) => void;
  vi.stubGlobal(
    "fetch",
    vi.fn((_input: RequestInfo | URL, init?: RequestInit) => {
      signal = init?.signal;
      return new Promise<Response>((resolve) => {
        release = resolve;
      });
    }),
  );
  mountSearch();
  submit();
  await screen.findByRole("status", { name: "正在读取 Sub2API 系统日志" });
  try {
    const reset = screen.getByRole("button", { name: "重置" });
    expect(reset).toBeEnabled();
    fireEvent.click(reset);
    expect(signal?.aborted).toBe(true);
    expect(screen.getByText("设置筛选条件后点击查询")).toBeVisible();
    expect(screen.getByRole("textbox", { name: "request_id" })).toHaveValue("");
  } finally {
    await act(async () => release(Response.json(page("request-1"))));
  }
  expect(screen.queryByText("request-1")).not.toBeInTheDocument();
});

it("重复查询同一请求时保留已有日志，返回后更新结果", async () => {
  let release!: (response: Response) => void;
  const response = new Promise<Response>((resolve) => {
    release = resolve;
  });
  const fetch = vi
    .fn()
    .mockResolvedValueOnce(Response.json(page("request-1")))
    .mockReturnValueOnce(response);
  vi.stubGlobal("fetch", fetch);
  mountSearch();
  submit();
  await screen.findByText("request-1");
  submit();
  try {
    expect(screen.getByText("request-1")).toBeVisible();
    expect(
      screen.queryByRole("status", { name: "正在读取 Sub2API 系统日志" }),
    ).not.toBeInTheDocument();
    await waitFor(() => expect(fetch).toHaveBeenCalledTimes(2));
    expect(screen.getByRole("button", { name: "查询中" })).toBeDisabled();
  } finally {
    await act(async () => release(Response.json({ ...page("request-1"), total: 0, items: [] })));
  }
  expect(await screen.findByText("没有匹配的系统日志")).toBeVisible();
});

it("同一请求刷新失败时保留已读日志，并允许再次查询", async () => {
  vi.stubGlobal(
    "fetch",
    vi
      .fn()
      .mockResolvedValueOnce(Response.json(page("request-1")))
      .mockResolvedValueOnce(Response.json({ detail: "管理端暂不可用" }, { status: 503 })),
  );
  mountSearch();
  submit();
  await screen.findByText("request-1");
  submit();
  await waitFor(() => expect(screen.getByRole("button", { name: "查询" })).toBeEnabled());
  expect(screen.getByText("request-1")).toBeVisible();
  expect(screen.queryByText("没有匹配的系统日志")).not.toBeInTheDocument();
});

it("首次查询失败时显示重新读取入口，重试成功后显示日志", async () => {
  vi.stubGlobal(
    "fetch",
    vi
      .fn()
      .mockResolvedValueOnce(Response.json({ detail: "管理端暂不可用" }, { status: 503 }))
      .mockResolvedValueOnce(Response.json(page("request-1"))),
  );
  mountSearch();
  submit();
  fireEvent.click(await screen.findByRole("button", { name: "重新读取" }));
  expect(await screen.findByText("request-1")).toBeVisible();
  expect(screen.queryByRole("button", { name: "重新读取" })).not.toBeInTheDocument();
});

it("查询不同请求时不把上一次结果当作新请求结果展示", async () => {
  let release!: (response: Response) => void;
  const response = new Promise<Response>((resolve) => {
    release = resolve;
  });
  vi.stubGlobal(
    "fetch",
    vi
      .fn()
      .mockResolvedValueOnce(Response.json(page("request-1")))
      .mockReturnValueOnce(response),
  );
  mountSearch();
  submit();
  await screen.findByText("request-1");
  submit("request-2");
  try {
    expect(await screen.findByRole("status", { name: "正在读取 Sub2API 系统日志" })).toBeVisible();
    expect(screen.queryByText("request-1")).not.toBeInTheDocument();
  } finally {
    await act(async () => release(Response.json(page("request-2"))));
  }
  expect(await screen.findByText("request-2")).toBeVisible();
});
