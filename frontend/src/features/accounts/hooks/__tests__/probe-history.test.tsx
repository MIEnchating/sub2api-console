import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, expect, it, vi } from "vitest";
import { api, type Task } from "@/api";
import { useOnboardingProbeTask } from "../use-onboarding-probe-task";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

it("上一轮完成后重试创建任务期间清除旧摘要并标记正在启动", async () => {
  const prior: Task = {
    id: "prior",
    skill: "onboarding",
    operation: "models",
    status: "succeeded",
    progress: 0,
    message: "完成",
    created_at: "",
    updated_at: "",
    result: {
      steps: [{ stage: "models", status: "succeeded", started_at: "2026-09-15T00:00:00Z" }],
    },
  };
  const start = vi.spyOn(api, "startOnboardingProbeTask").mockResolvedValue(prior);
  vi.spyOn(api, "task").mockResolvedValue(prior);
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const hook = renderHook(() => useOnboardingProbeTask("upstream.example", "7"), {
    wrapper: (props: { children: ReactNode }) => (
      <QueryClientProvider client={client}>{props.children}</QueryClientProvider>
    ),
  });
  let first!: Promise<Task>;
  act(() => {
    first = hook.result.current.run("models");
  });
  await waitFor(() => expect(hook.result.current.history).toHaveLength(1));
  await act(async () => {
    await first;
  });
  let rejectStart!: (error: Error) => void;
  start.mockImplementationOnce(
    () =>
      new Promise((_resolve, reject) => {
        rejectStart = reject;
      }),
  );
  let retried!: Promise<Task | null>;
  act(() => {
    retried = hook.result.current.run("models").catch(() => null);
  });
  expect(hook.result.current.history).toEqual([]);
  expect(hook.result.current.starting).toBe(true);
  await act(async () => {
    rejectStart(new Error("隔离启动失败"));
    await retried;
  });
  expect(hook.result.current.starting).toBe(false);
  hook.unmount();
  client.clear();
});

it("首次探活保留准备阶段，重试仅展示当前轮过程且轮询不重复添加步骤", async () => {
  const tasks = new Map<string, Task>();
  let sequence = 0;
  vi.spyOn(api, "startOnboardingProbeTask").mockImplementation(async (action) => {
    const id = `${action}-${++sequence}`;
    let stages = ["reuse_key", "request", "cleanup_key"];
    if (action === "models") stages = ["create_key", "models"];
    else if (sequence > 2) stages = ["credential", "create_key", "request", "cleanup_key"];
    const task: Task = {
      id,
      skill: "onboarding",
      operation: action,
      status: "succeeded",
      progress: 0,
      message: "完成",
      created_at: "",
      updated_at: "",
      result: {
        steps: stages.map((stage) => ({
          stage,
          status: "succeeded",
          started_at: `2026-09-15T00:00:0${sequence}Z`,
        })),
      },
    };
    tasks.set(id, task);
    return { ...task, status: "queued", result: { steps: [] } };
  });
  vi.spyOn(api, "task").mockImplementation(async (id) => tasks.get(id)!);
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const hook = renderHook(() => useOnboardingProbeTask("upstream.example", "7"), {
    wrapper: (props: { children: ReactNode }) => (
      <QueryClientProvider client={client}>{props.children}</QueryClientProvider>
    ),
  });
  async function run(action: "models" | "probe") {
    let completed!: Promise<Task>;
    act(() => {
      completed = hook.result.current.run(action, "model");
    });
    await waitFor(() => expect(hook.result.current.task?.id).toBe(`${action}-${sequence}`));
    await act(async () => {
      await completed;
    });
  }
  await run("models");
  await run("probe");
  expect(hook.result.current.history.map((step) => step.stage)).toEqual([
    "create_key",
    "models",
    "reuse_key",
    "request",
    "cleanup_key",
  ]);
  await run("probe");
  expect(hook.result.current.history.map((step) => step.stage)).toEqual([
    "credential",
    "create_key",
    "request",
    "cleanup_key",
  ]);
  expect(hook.result.current.history[2]?.started_at).toBe("2026-09-15T00:00:03Z");
  await act(async () => {
    await hook.result.current.refetch();
  });
  expect(hook.result.current.history).toHaveLength(4);
  hook.unmount();
  client.clear();
});
