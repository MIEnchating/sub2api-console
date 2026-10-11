import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, expect, it, vi } from "vitest";
import type { Task } from "@/api";
import { useAnimationTasks } from "../use-animation-tasks";

const task: Task = {
  id: "animation-41",
  skill: "sub2api-model-animation",
  operation: "account-model-animation",
  status: "succeeded",
  progress: 100,
  message: "检测完成",
  created_at: "2026-10-09T00:00:00Z",
  updated_at: "2026-10-09T00:00:01.123456789Z",
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
const changed: Task = {
  ...task,
  updated_at: "2026-10-09T00:00:01.123456790Z",
  result: {
    animations: [
      {
        ...(task.result.animations as Record<string, unknown>[])[0],
        manual_verdict: {
          verdict: "not_passed",
          updated_at: "2026-10-09T00:00:01Z",
          updated_by: "operator",
        },
      },
    ],
  },
};
const clients: QueryClient[] = [];
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  vi.unstubAllGlobals();
});

function setup(summary = task) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  });
  clients.push(client);
  client.setQueryData(["model-animation", "history"], [{ ...summary, result: {} }]);
  client.setQueryData(["model-animation", "task", task.id], task);
  const fetcher = vi.fn<typeof fetch>().mockImplementation(async (input) => {
    if (String(input) === `/api/tasks/${task.id}`) return Response.json(changed);
    return Response.json([{ ...changed, result: {} }]);
  });
  vi.stubGlobal("fetch", fetcher);
  const view = renderHook(() => useAnimationTasks(), {
    wrapper: (props: { children: ReactNode }) => (
      <QueryClientProvider client={client}>{props.children}</QueryClientProvider>
    ),
  });
  return { ...view, client, fetcher };
}

it("历史出现较新纳秒版本时刷新已完成详情并显示其他会话的新结论", async () => {
  const view = setup();
  expect(view.result.current.results.get("41")?.manual_verdict).toBeUndefined();
  await act(async () =>
    view.client.refetchQueries({ queryKey: ["model-animation", "history"], exact: true }),
  );
  await waitFor(() =>
    expect(view.result.current.results.get("41")?.manual_verdict?.verdict).toBe("not_passed"),
  );
  expect(view.fetcher.mock.calls.map(([input]) => String(input))).toEqual([
    "/api/model-checks/animations",
    `/api/tasks/${task.id}`,
  ]);
});

it.each([task.updated_at, "2026-10-09T00:00:00Z"])(
  "历史版本%s不晚于缓存时不重复读取已完成详情",
  async (updated_at) => {
    const view = setup({ ...task, updated_at });
    await act(async () => undefined);
    expect(view.result.current.results.get("41")?.svg).toBe("<svg />");
    expect(view.fetcher).not.toHaveBeenCalled();
  },
);

it("较新历史触发的详情读取失败时保留结果并允许重试恢复新结论", async () => {
  const view = setup();
  view.fetcher.mockImplementation(async (input) => {
    if (String(input) === `/api/tasks/${task.id}`)
      return Response.json({ detail: "详情读取失败" }, { status: 503 });
    return Response.json([{ ...changed, result: {} }]);
  });
  await act(async () =>
    view.client.refetchQueries({ queryKey: ["model-animation", "history"], exact: true }),
  );
  await waitFor(() => expect(view.result.current.historyError).toBe(true));
  expect(view.result.current.results.get("41")?.svg).toBe("<svg />");
  expect(view.result.current.historyLoading).toBe(false);
  view.fetcher.mockImplementation(async () => Response.json(changed));
  await act(async () => view.result.current.retryHistory());
  await waitFor(() =>
    expect(view.result.current.results.get("41")?.manual_verdict?.verdict).toBe("not_passed"),
  );
  expect(view.result.current.historyError).toBe(false);
});

it("历史变新触发慢详情读取时保留原结果且不被后续历史刷新中断", async () => {
  const view = setup();
  let release!: (response: Response) => void;
  let signal: AbortSignal | null | undefined;
  view.fetcher.mockImplementation(
    (_input, init) =>
      new Promise<Response>((resolve) => {
        signal = init?.signal;
        release = resolve;
      }),
  );
  await act(async () =>
    view.client.setQueryData(["model-animation", "history"], [{ ...changed, result: {} }]),
  );
  await waitFor(() =>
    expect(view.fetcher).toHaveBeenCalledWith(`/api/tasks/${task.id}`, expect.anything()),
  );
  expect(view.result.current.results.get("41")?.svg).toBe("<svg />");
  expect(view.result.current.historyLoading).toBe(false);
  await act(async () =>
    view.client.setQueryData(["model-animation", "history"], [{ ...changed, result: {} }], {
      updatedAt: view.client.getQueryState(["model-animation", "history"])!.dataUpdatedAt + 1,
    }),
  );
  expect(signal?.aborted).toBe(false);
  expect(view.fetcher).toHaveBeenCalledTimes(1);
  await act(async () => release(Response.json(changed)));
  await waitFor(() =>
    expect(view.result.current.results.get("41")?.manual_verdict?.verdict).toBe("not_passed"),
  );
});

it("重新挂载时历史缓存已有较新版本则主动刷新旧详情", async () => {
  const view = setup(changed);
  await waitFor(() =>
    expect(view.result.current.results.get("41")?.manual_verdict?.verdict).toBe("not_passed"),
  );
  expect(view.fetcher.mock.calls.map(([input]) => String(input))).toEqual([
    `/api/tasks/${task.id}`,
  ]);
});

it("页面重渲染但任务内容未变时复用公开结果集合，避免结果卡片重复计算", async () => {
  const view = setup();
  const results = view.result.current.results;
  const prechecks = view.result.current.precheckResults;
  view.rerender();
  await act(async () => undefined);
  expect(view.result.current.results).toBe(results);
  expect(view.result.current.precheckResults).toBe(prechecks);
  expect(view.fetcher).not.toHaveBeenCalled();
});

it("详情后台刷新尚未改变内容时复用结果，完成后展示新手动结论", async () => {
  const view = setup();
  const results = view.result.current.results;
  let release!: (response: Response) => void;
  view.fetcher.mockImplementation(
    () =>
      new Promise<Response>((resolve) => {
        release = resolve;
      }),
  );
  await act(async () => {
    void view.client.refetchQueries({
      queryKey: ["model-animation", "task", task.id],
      exact: true,
    });
  });
  await waitFor(() => expect(view.result.current.retryingHistory).toBe(true));
  expect(view.result.current.results).toBe(results);
  await act(async () => release(Response.json(changed)));
  await waitFor(() =>
    expect(view.result.current.results.get("41")?.manual_verdict?.verdict).toBe("not_passed"),
  );
  expect(view.result.current.results).not.toBe(results);
});
