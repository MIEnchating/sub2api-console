import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, expect, it, vi } from "vitest";
import type { Task } from "@/api";
import { useAnimationTasks } from "../use-animation-tasks";

const clients: QueryClient[] = [];
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  vi.unstubAllGlobals();
});

function historyTask(id: string, status: Task["status"] = "succeeded"): Task {
  return {
    id,
    skill: "sub2api-model-animation",
    operation: "account-model-animation",
    status,
    progress: status === "succeeded" ? 100 : 0,
    message: "检测任务",
    created_at: "2026-10-09T00:00:00Z",
    updated_at: "2026-10-09T00:00:01Z",
    result: {
      account_ids: [id],
      animations: [
        {
          account_id: id,
          account_name: id,
          model: "test-model",
          request_id: id,
          status: "succeeded",
          duration_ms: 1,
          completed_at: "2026-10-09T00:00:01Z",
          svg: "<svg />",
        },
      ],
    },
  };
}

function setup(history: Task[], cached?: Task) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  });
  clients.push(client);
  client.setQueryData(
    ["model-animation", "history"],
    history.map((task) => ({ ...task, result: {} })),
  );
  if (cached) client.setQueryData(["model-animation", "task", cached.id], cached);
  const pending = new Map<string, { resolve: (response: Response) => void; signal: AbortSignal }>();
  let running = 0;
  let maximum = 0;
  const fetcher = vi.fn<typeof fetch>().mockImplementation((input, init) => {
    const id = String(input).split("/").at(-1)!;
    running++;
    maximum = Math.max(maximum, running);
    return new Promise<Response>((resolve, reject) => {
      const signal = init!.signal!;
      signal.addEventListener("abort", () => reject(signal.reason), { once: true });
      pending.set(id, { resolve, signal });
    }).finally(() => {
      running--;
    });
  });
  vi.stubGlobal("fetch", fetcher);
  const view = renderHook(() => useAnimationTasks(), {
    wrapper: (props: { children: ReactNode }) => (
      <QueryClientProvider client={client}>{props.children}</QueryClientProvider>
    ),
  });
  return { ...view, client, pending, fetcher, maximum: () => maximum };
}

it("历史有六个账号时最多同时读取四个详情且最终保留全部账号结果", async () => {
  const history = ["1", "2", "3", "4", "5", "6"].map((id) => historyTask(id));
  const view = setup(history);
  await waitFor(() => expect(view.fetcher).toHaveBeenCalledTimes(4));
  expect(view.result.current.historyLoading).toBe(true);
  await act(async () => {
    for (const task of history.slice(0, 4)) view.pending.get(task.id)!.resolve(Response.json(task));
  });
  await waitFor(() => expect(view.fetcher).toHaveBeenCalledTimes(6));
  await act(async () => {
    for (const task of history.slice(4)) view.pending.get(task.id)!.resolve(Response.json(task));
  });
  await waitFor(() => expect(view.result.current.historyLoading).toBe(false));
  expect([...view.result.current.results.keys()].sort()).toEqual(["1", "2", "3", "4", "5", "6"]);
  expect(view.maximum()).toBe(4);
});

it("历史详情排队时活动任务优先获得下一空闲读取位置", async () => {
  const view = setup(
    ["1", "2", "3", "4", "5"].map((id) => historyTask(id)).concat(historyTask("active", "running")),
  );
  await waitFor(() => expect(view.fetcher).toHaveBeenCalledTimes(4));
  await act(async () => view.pending.get("1")!.resolve(Response.json(historyTask("1"))));
  await waitFor(() => expect(view.fetcher).toHaveBeenCalledTimes(5));
  expect(view.fetcher.mock.calls.at(-1)?.[0]).toBe("/api/tasks/active");
  expect(view.pending.has("5")).toBe(false);
});

it("历史显示任务已重新运行时旧终态缓存不降低其读取优先级", async () => {
  const active = { ...historyTask("active", "running"), updated_at: "2026-10-09T00:00:02Z" };
  const view = setup(
    ["1", "2", "3", "4", "5"].map((id) => historyTask(id)).concat(active),
    historyTask("active"),
  );
  await waitFor(() => expect(view.fetcher).toHaveBeenCalledTimes(4));
  await act(async () => view.pending.get("1")!.resolve(Response.json(historyTask("1"))));
  await waitFor(() => expect(view.fetcher).toHaveBeenCalledTimes(5));
  expect(view.fetcher.mock.calls.at(-1)?.[0]).toBe("/api/tasks/active");
});

it("响应体尚未读取完成时保留并发位置而不提前启动下一历史请求", async () => {
  const view = setup(["1", "2", "3", "4", "5"].map((id) => historyTask(id)));
  await waitFor(() => expect(view.fetcher).toHaveBeenCalledTimes(4));
  let body!: ReadableStreamDefaultController<Uint8Array>;
  const stream = new ReadableStream<Uint8Array>({
    start: (controller) => {
      body = controller;
    },
  });
  await act(async () =>
    view.pending
      .get("1")!
      .resolve(new Response(stream, { headers: { "Content-Type": "application/json" } })),
  );
  expect(view.fetcher).toHaveBeenCalledTimes(4);
  await act(async () => {
    body.enqueue(new TextEncoder().encode(JSON.stringify(historyTask("1"))));
    body.close();
  });
  await waitFor(() => expect(view.pending.has("5")).toBe(true));
});

it("历史为空时不发起任务详情读取", async () => {
  const view = setup([]);
  await act(async () => undefined);
  expect(view.fetcher).not.toHaveBeenCalled();
  expect(view.result.current.historyLoading).toBe(false);
  expect(view.result.current.results.size).toBe(0);
});

it("离开页面时取消在途读取并丢弃尚未发出的历史详情", async () => {
  const view = setup(["1", "2", "3", "4", "5", "6"].map((id) => historyTask(id)));
  await waitFor(() => expect(view.fetcher).toHaveBeenCalledTimes(4));
  await act(async () => view.unmount());
  expect([...view.pending.values()].every((request) => request.signal.aborted)).toBe(true);
  expect(view.fetcher).toHaveBeenCalledTimes(4);
});

it("一个详情失败时继续读取排队历史且重试能补回失败账号", async () => {
  const view = setup(["1", "2", "3", "4", "5"].map((id) => historyTask(id)));
  await waitFor(() => expect(view.fetcher).toHaveBeenCalledTimes(4));
  await act(async () =>
    view.pending.get("1")!.resolve(Response.json({ detail: "读取失败" }, { status: 503 })),
  );
  await waitFor(() => expect(view.pending.has("5")).toBe(true));
  await act(async () => {
    for (const id of ["2", "3", "4", "5"])
      view.pending.get(id)!.resolve(Response.json(historyTask(id)));
  });
  await waitFor(() => expect(view.result.current.historyError).toBe(true));
  await act(async () => view.result.current.retryHistory());
  await waitFor(() => expect(view.fetcher).toHaveBeenCalledTimes(6));
  await act(async () => view.pending.get("1")!.resolve(Response.json(historyTask("1"))));
  await waitFor(() => expect(view.result.current.results.size).toBe(5));
  expect(view.result.current.historyError).toBe(false);
  expect(view.maximum()).toBe(4);
});
