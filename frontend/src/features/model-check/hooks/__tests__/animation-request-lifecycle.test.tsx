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

it.each([
  { label: "历史尚未读取", cached: false },
  { label: "已有历史、任务详情尚未读取", cached: true },
])("$label时离开动画检测取消在途读取而不取消业务任务", async (scenario) => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  });
  clients.push(client);
  const task: Task = {
    id: "animation-1",
    skill: "console",
    operation: "account-model-animation",
    status: "running",
    progress: 0,
    message: "动画生成中",
    result: { account_ids: ["41"] },
    created_at: "2026-10-09T00:00:00Z",
    updated_at: "2026-10-09T00:00:00Z",
  };
  if (scenario.cached) client.setQueryData(["model-animation", "history"], [task]);
  let signal: AbortSignal | null | undefined;
  let release!: (response: Response) => void;
  const fetcher = vi.fn<typeof fetch>().mockImplementation((_input, init) => {
    signal = init?.signal;
    return new Promise<Response>((resolve) => {
      release = resolve;
    });
  });
  vi.stubGlobal("fetch", fetcher);
  const view = renderHook(() => useAnimationTasks(), {
    wrapper: (props: { children: ReactNode }) => (
      <QueryClientProvider client={client}>{props.children}</QueryClientProvider>
    ),
  });
  await waitFor(() => expect(fetcher).toHaveBeenCalled());
  try {
    view.unmount();
    expect(signal?.aborted).toBe(true);
  } finally {
    await act(async () => release(Response.json(scenario.cached ? task : [task])));
  }
  expect(fetcher.mock.calls.map(([input]) => String(input))).toEqual([
    scenario.cached ? "/api/tasks/animation-1" : "/api/model-checks/animations",
  ]);
});
