import { QueryClient, QueryClientProvider, QueryObserver } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, expect, it, vi } from "vitest";

import type { Task } from "@/api";
import { useAnimationTasks } from "../use-animation-tasks";

const created: Task = {
  id: "animation-created",
  skill: "sub2api-model-animation",
  operation: "account-model-animation",
  status: "succeeded",
  progress: 100,
  message: "检测完成",
  created_at: "2026-10-09T00:00:00Z",
  updated_at: "2026-10-09T00:00:01Z",
  result: {
    animations: [
      {
        account_id: "41",
        account_name: "测试账号",
        model: "test-model",
        request_id: "request-41",
        status: "succeeded",
        duration_ms: 100,
        completed_at: "2026-10-09T00:00:01Z",
        svg: "<svg />",
      },
    ],
  },
};

afterEach(() => vi.unstubAllGlobals());

it("成功刷新确认新任务已从历史移除时释放无人观察的详情缓存", async () => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  });
  client.setQueryData(["model-animation", "history"], []);
  let historyItems: Task[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (_input, init) => Response.json(init?.method === "POST" ? created : historyItems)),
  );
  const view = renderHook(() => useAnimationTasks(), {
    wrapper: (props: { children: ReactNode }) => (
      <QueryClientProvider client={client}>{props.children}</QueryClientProvider>
    ),
  });
  try {
    await act(async () => {
      await view.result.current.start({ targets: [], timeout_seconds: 30 });
    });
    await waitFor(() => expect(view.result.current.results.has("41")).toBe(true));

    historyItems = [{ ...created, result: {} }];
    await act(async () => {
      await client.refetchQueries({ queryKey: ["model-animation", "history"], exact: true });
    });
    expect(
      client.getQueryData<Task[]>(["model-animation", "history"])?.map((item) => item.id),
    ).toEqual([created.id]);
    view.rerender();
    await act(async () => undefined);
    expect(view.result.current.results.has("41")).toBe(true);

    historyItems = [];
    await act(async () => {
      await client.refetchQueries({ queryKey: ["model-animation", "history"], exact: true });
    });
    await waitFor(() => expect(view.result.current.results.has("41")).toBe(false));
    await waitFor(() =>
      expect(client.getQueryData(["model-animation", "task", created.id])).toBeUndefined(),
    );
  } finally {
    view.unmount();
    client.clear();
  }
});

it("只有旧缓存历史且尚未成功刷新时保留未列出的详情", async () => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  });
  client.setQueryData(["model-animation", "history"], []);
  client.setQueryData(["model-animation", "task", created.id], created);
  const view = renderHook(() => useAnimationTasks(), {
    wrapper: (props: { children: ReactNode }) => (
      <QueryClientProvider client={client}>{props.children}</QueryClientProvider>
    ),
  });
  try {
    expect(client.getQueryData(["model-animation", "task", created.id])).toEqual(created);
  } finally {
    view.unmount();
    client.clear();
  }
});

it("历史成功刷新时不清除本页从未展示的任务详情", async () => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  });
  client.setQueryData(["model-animation", "history"], []);
  client.setQueryData(["model-animation", "task", created.id], created);
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => Response.json([])),
  );
  const view = renderHook(() => useAnimationTasks(), {
    wrapper: (props: { children: ReactNode }) => (
      <QueryClientProvider client={client}>{props.children}</QueryClientProvider>
    ),
  });
  try {
    await act(async () => {
      await client.refetchQueries({ queryKey: ["model-animation", "history"], exact: true });
    });
    expect(client.getQueryData(["model-animation", "task", created.id])).toEqual(created);
  } finally {
    view.unmount();
    client.clear();
  }
});

it("成功刷新确认任务已移除但详情仍被其他组件观察时保留缓存", async () => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  });
  client.setQueryData(["model-animation", "history"], [created]);
  client.setQueryData(["model-animation", "task", created.id], created);
  const observer = new QueryObserver(client, {
    queryKey: ["model-animation", "task", created.id],
    queryFn: async () => created,
  });
  const unsubscribe = observer.subscribe(() => undefined);
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => Response.json([])),
  );
  const view = renderHook(() => useAnimationTasks(), {
    wrapper: (props: { children: ReactNode }) => (
      <QueryClientProvider client={client}>{props.children}</QueryClientProvider>
    ),
  });
  try {
    await act(async () => {
      await client.refetchQueries({ queryKey: ["model-animation", "history"], exact: true });
    });
    await waitFor(() => expect(view.result.current.results.has("41")).toBe(false));
    expect(client.getQueryData(["model-animation", "task", created.id])).toEqual(created);
  } finally {
    view.unmount();
    unsubscribe();
    client.clear();
  }
});

it("新任务创建后历史刷新失败时继续显示本页任务", async () => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  });
  client.setQueryData(["model-animation", "history"], []);
  vi.stubGlobal(
    "fetch",
    vi.fn(async (_input, init) =>
      init?.method === "POST"
        ? Response.json(created)
        : Response.json({ detail: "历史读取失败" }, { status: 503 }),
    ),
  );
  const view = renderHook(() => useAnimationTasks(), {
    wrapper: (props: { children: ReactNode }) => (
      <QueryClientProvider client={client}>{props.children}</QueryClientProvider>
    ),
  });
  try {
    await act(async () => {
      await view.result.current.start({ targets: [], timeout_seconds: 30 });
    });
    await waitFor(() => expect(view.result.current.historyError).toBe(true));
    expect(view.result.current.results.has("41")).toBe(true);
    expect(client.getQueryData(["model-animation", "task", created.id])).toEqual(created);
  } finally {
    view.unmount();
    client.clear();
  }
});

it("清理使缓存历史失效后重新进入页面会读取最新历史", async () => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  });
  client.setQueryData(["model-animation", "history"], [created]);
  client.setQueryData(["model-animation", "task", created.id], created);
  await client.invalidateQueries({ queryKey: ["model-animation", "history"] });
  const fetchHistory = vi.fn(async () => Response.json([]));
  vi.stubGlobal("fetch", fetchHistory);
  const view = renderHook(() => useAnimationTasks(), {
    wrapper: (props: { children: ReactNode }) => (
      <QueryClientProvider client={client}>{props.children}</QueryClientProvider>
    ),
  });
  try {
    await waitFor(() => expect(fetchHistory).toHaveBeenCalled());
    await waitFor(() => expect(view.result.current.results.has("41")).toBe(false));
  } finally {
    view.unmount();
    client.clear();
  }
});
