import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { TraceAccountActions } from "../trace-account-actions";
import { SystemLogSearchPanel } from "../system-log-search-panel";
let client: QueryClient;
afterEach(() => {
  cleanup();
  client?.clear();
  vi.unstubAllGlobals();
});
it("提交 request_id 查询后显示日志骨架，查询期间禁用重复提交", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(() => new Promise<Response>(() => {})),
  );
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <SystemLogSearchPanel />
    </QueryClientProvider>,
  );
  fireEvent.change(screen.getByPlaceholderText("输入完整 request_id"), {
    target: { value: "req-1" },
  });
  fireEvent.click(screen.getByRole("button", { name: "查询" }));
  const loading = await screen.findByRole("status", { name: "正在读取 Sub2API 系统日志" });
  expect(loading.querySelector('[data-slot="skeleton"]')).not.toBeNull();
  expect(screen.queryByRole("progressbar")).not.toBeInTheDocument();
  expect(screen.getByRole("button", { name: "查询中" })).toBeDisabled();
});

it("请求追踪读取账号状态时显示局部忙碌反馈并禁用处置", () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(() => new Promise<Response>(() => {})),
  );
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <TraceAccountActions accountId="206" />
    </QueryClientProvider>,
  );
  expect(screen.getByRole("status", { name: "正在读取账号状态" })).toHaveAttribute(
    "aria-busy",
    "true",
  );
  expect(screen.getByRole("button", { name: "手动熔断" })).toBeDisabled();
});
