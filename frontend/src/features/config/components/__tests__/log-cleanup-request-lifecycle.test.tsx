import { QueryClient, QueryClientProvider, useQuery } from "@tanstack/react-query";
import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactElement } from "react";
import { afterEach, expect, it, vi } from "vitest";
import { Toaster, toast } from "sonner";

import { api, type Task } from "@/api";
import { LogCleanupAction } from "../log-cleanup-action";

const clients: QueryClient[] = [];
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  toast.dismiss();
  vi.unstubAllGlobals();
});

function createClient(): QueryClient {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  clients.push(client);
  return client;
}

function completedTask(id = "cleanup-1"): Task {
  return {
    id,
    operation: "log-cleanup",
    skill: "console",
    status: "succeeded",
    progress: 100,
    message: `${id} 清理完成`,
    result: { retention_days: 30, total: 12 },
    created_at: "2026-10-09T00:00:00Z",
    updated_at: "2026-10-09T00:00:01Z",
  };
}

it("离开清理页面时取消任务读取，迟到响应不进入缓存且不发送任务取消", async () => {
  let signal: AbortSignal | null | undefined;
  let finish!: (response: Response) => void;
  const fetch = vi.fn((_input: string, init?: RequestInit) => {
    signal = init?.signal;
    return new Promise<Response>((resolve) => {
      finish = resolve;
    });
  });
  vi.stubGlobal("fetch", fetch);
  const client = createClient();
  const view = render(
    <QueryClientProvider client={client}>
      <LogCleanupAction retentionDays={30} ready activeTaskId="cleanup-1" />
    </QueryClientProvider>,
  );
  await waitFor(() => expect(fetch).toHaveBeenCalledTimes(1));
  view.unmount();
  await act(async () => finish(Response.json(completedTask())));
  expect(signal?.aborted).toBe(true);
  expect(client.getQueryData(["task", "cleanup-1"])).toBeUndefined();
  expect(fetch).toHaveBeenCalledTimes(1);
});

it("活动清理任务切换时取消旧读取并只展示新任务结果", async () => {
  let signal: AbortSignal | null | undefined;
  let finish!: (response: Response) => void;
  vi.stubGlobal(
    "fetch",
    vi.fn((input: string, init?: RequestInit) => {
      if (input.endsWith("cleanup-2"))
        return Promise.resolve(Response.json(completedTask("cleanup-2")));
      signal = init?.signal;
      return new Promise<Response>((resolve) => {
        finish = resolve;
      });
    }),
  );
  const client = createClient();
  const view = render(
    <QueryClientProvider client={client}>
      <LogCleanupAction retentionDays={30} ready activeTaskId="cleanup-1" />
      <Toaster />
    </QueryClientProvider>,
  );
  await waitFor(() => expect(finish).toBeTypeOf("function"));
  view.rerender(
    <QueryClientProvider client={client}>
      <LogCleanupAction retentionDays={30} ready activeTaskId="cleanup-2" />
      <Toaster />
    </QueryClientProvider>,
  );
  expect(await screen.findByText("cleanup-2 清理完成")).toBeVisible();
  await act(async () => finish(Response.json(completedTask())));
  expect(signal?.aborted).toBe(true);
  expect(client.getQueryData(["task", "cleanup-1"])).toBeUndefined();
  expect(screen.queryByText("cleanup-1 清理完成")).not.toBeInTheDocument();
});

it("只关闭清理弹窗时继续读取并在完成后通知", async () => {
  let signal: AbortSignal | null | undefined;
  let finish!: (response: Response) => void;
  const fetch = vi.fn((_input: string, init?: RequestInit) => {
    signal = init?.signal;
    return new Promise<Response>((resolve) => {
      finish = resolve;
    });
  });
  vi.stubGlobal("fetch", fetch);
  render(
    <QueryClientProvider client={createClient()}>
      <LogCleanupAction retentionDays={30} ready activeTaskId="cleanup-1" />
      <Toaster />
    </QueryClientProvider>,
  );
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "查看清理进度" }));
  expect(screen.getByRole("status", { name: "正在读取日志清理任务" })).toBeVisible();
  await user.keyboard("{Escape}");
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  await act(async () => finish(Response.json(completedTask())));
  expect(await screen.findByText("cleanup-1 清理完成")).toBeVisible();
  expect(signal?.aborted).not.toBe(true);
  expect(fetch).toHaveBeenCalledTimes(1);
});

function SharedTaskViewer(): ReactElement {
  const query = useQuery({
    queryKey: ["task", "cleanup-1"],
    queryFn: ({ signal }) => api.task("cleanup-1", signal),
  });
  return <p>{query.data?.message ?? "其他页面正在观察"}</p>;
}

it("卸载清理入口时其他观察者需要同一任务则共享读取继续", async () => {
  let signal: AbortSignal | null | undefined;
  let finish!: (response: Response) => void;
  const fetch = vi.fn((_input: string, init?: RequestInit) => {
    signal = init?.signal;
    return new Promise<Response>((resolve) => {
      finish = resolve;
    });
  });
  vi.stubGlobal("fetch", fetch);
  const client = createClient();
  const view = render(
    <QueryClientProvider client={client}>
      <LogCleanupAction retentionDays={30} ready activeTaskId="cleanup-1" />
    </QueryClientProvider>,
  );
  await waitFor(() => expect(fetch).toHaveBeenCalledTimes(1));
  const other = render(
    <QueryClientProvider client={client}>
      <SharedTaskViewer />
    </QueryClientProvider>,
  );
  view.unmount();
  await act(async () => finish(Response.json(completedTask())));
  expect(await screen.findByText("cleanup-1 清理完成")).toBeVisible();
  expect(signal?.aborted).not.toBe(true);
  expect(fetch).toHaveBeenCalledTimes(1);
  other.unmount();
});
