import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";

import type { AccountControlState, Task } from "@/api";
import { TraceAccountActions } from "../trace-account-actions";

const account: AccountControlState = {
  id: "41",
  name: "测试账号",
  health: "healthy",
  routing_state: "healthy",
  health_status: null,
  paused: false,
  schedulable: true,
  manual_priority: null,
};
const clients: QueryClient[] = [];
afterEach(() => {
  cleanup();
  for (const client of clients.splice(0)) client.clear();
  vi.unstubAllGlobals();
});

it("日志处置仅读取轻量状态，保留已有完整账号详情缓存", async () => {
  const client = queryClient();
  const detail = { ...account, metadata: { notes: "完整详情" }, bindings: [] };
  client.setQueryData(["account-detail", "41"], detail);
  const fetcher = vi.fn<typeof fetch>().mockResolvedValue(Response.json(account));
  vi.stubGlobal("fetch", fetcher);
  render(
    <QueryClientProvider client={client}>
      <TraceAccountActions accountId="41" />
    </QueryClientProvider>,
  );
  await waitFor(() => expect(screen.getByRole("button", { name: "手动熔断" })).toBeEnabled());
  expect(fetcher).toHaveBeenCalledWith("/api/accounts/41/control-state", expect.any(Object));
  expect(client.getQueryData(["account-detail", "41"])).toEqual(detail);
  expect(client.getQueryData(["account-detail", "41", "control-state"])).toEqual(account);
});

function queryClient(): QueryClient {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  clients.push(client);
  return client;
}

it("离开账号处置区域时取消尚未完成的账号读取", async () => {
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
  const view = render(
    <QueryClientProvider client={queryClient()}>
      <TraceAccountActions accountId="41" />
    </QueryClientProvider>,
  );
  await screen.findByRole("status", { name: "正在读取账号状态" });
  try {
    view.unmount();
    expect(signal?.aborted).toBe(true);
  } finally {
    await act(async () => release(Response.json(account)));
  }
});

it("同一账号仍有可见处置区域时保留共享读取，最后一处移除后取消", async () => {
  let signal: AbortSignal | null | undefined;
  let release!: (response: Response) => void;
  const fetch = vi.fn((_input: RequestInfo | URL, init?: RequestInit) => {
    signal = init?.signal;
    return new Promise<Response>((resolve) => {
      release = resolve;
    });
  });
  vi.stubGlobal("fetch", fetch);
  const client = queryClient();
  const view = render(
    <QueryClientProvider client={client}>
      <TraceAccountActions key="first" accountId="41" />
      <TraceAccountActions key="second" accountId="41" />
    </QueryClientProvider>,
  );
  await waitFor(() =>
    expect(screen.getAllByRole("status", { name: "正在读取账号状态" })).toHaveLength(2),
  );
  try {
    view.rerender(
      <QueryClientProvider client={client}>
        <TraceAccountActions key="second" accountId="41" />
      </QueryClientProvider>,
    );
    expect(fetch).toHaveBeenCalledTimes(1);
    expect(signal?.aborted).toBe(false);
    view.unmount();
    expect(signal?.aborted).toBe(true);
  } finally {
    await act(async () => release(Response.json(account)));
  }
});

it("处置任务已创建后离开页面只取消状态读取，不发送任务取消", async () => {
  const task: Task = {
    id: "control-41",
    skill: "console",
    operation: "account-control",
    status: "running",
    progress: 10,
    message: "处置执行中",
    result: {},
    created_at: "2026-10-01T00:00:00Z",
    updated_at: "2026-10-01T00:00:00Z",
  };
  let signal: AbortSignal | null | undefined;
  let release!: (response: Response) => void;
  const methods: string[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn((input: RequestInfo | URL, init?: RequestInit): Promise<Response> => {
      methods.push(init?.method ?? "GET");
      if (String(input) === "/api/accounts/41/control-state")
        return Promise.resolve(Response.json(account));
      if (String(input) === "/api/accounts/41/control") return Promise.resolve(Response.json(task));
      if (String(input) === "/api/tasks/control-41") {
        signal = init?.signal;
        return new Promise<Response>((resolve) => {
          release = resolve;
        });
      }
      return Promise.reject(new Error(`unexpected request: ${String(input)}`));
    }),
  );
  const view = render(
    <QueryClientProvider client={queryClient()}>
      <TraceAccountActions accountId="41" />
    </QueryClientProvider>,
  );
  await waitFor(() => expect(screen.getByRole("button", { name: "手动熔断" })).toBeEnabled());
  fireEvent.click(screen.getByRole("button", { name: "手动熔断" }));
  fireEvent.click(screen.getByRole("button", { name: "确认手动熔断" }));
  await waitFor(() => expect(release).toBeTypeOf("function"));
  try {
    view.unmount();
    expect(signal?.aborted).toBe(true);
    expect(methods.filter((method) => method !== "GET")).toEqual(["POST"]);
  } finally {
    await act(async () => release(Response.json(task)));
  }
});
