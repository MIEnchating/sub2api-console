import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, expect, it, vi } from "vitest";

import type { Task } from "@/api";
import { SystemInfoPage } from "../system-info-page";
import { TaskDetailsDialog } from "../task-details-dialog";

const clients: QueryClient[] = [];
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  vi.unstubAllGlobals();
});

function task(id: string): Task {
  return {
    id,
    operation: "active-probe",
    skill: "console",
    status: "succeeded",
    progress: 100,
    message: `${id} 已完成`,
    result: {},
    created_at: "2026-10-09T00:00:00Z",
    updated_at: "2026-10-09T00:00:00Z",
  };
}

function DetailHarness() {
  const [id, setID] = useState<string | null>("first-task");
  return <TaskDetailsDialog taskId={id} onClose={() => setID(null)} />;
}

function client(): QueryClient {
  const value = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  clients.push(value);
  return value;
}

it("离开系统信息页时取消尚未返回的任务列表与资源读取", async () => {
  const signals = new Map<string, AbortSignal | null | undefined>();
  vi.stubGlobal(
    "fetch",
    vi.fn((input: string, init?: RequestInit) => {
      const path = new URL(input, "http://localhost").pathname;
      if (path !== "/api/tasks" && path !== "/api/system/metrics")
        return Promise.resolve(Response.json({ items: [] }));
      signals.set(path, init?.signal);
      return new Promise<Response>(() => {});
    }),
  );
  const view = render(
    <QueryClientProvider client={client()}>
      <SystemInfoPage />
    </QueryClientProvider>,
  );
  await waitFor(() => expect(signals.size).toBe(2));
  view.unmount();
  expect(signals.get("/api/tasks")?.aborted).toBe(true);
  expect(signals.get("/api/system/metrics")?.aborted).toBe(true);
});

it("关闭仍在读取的任务详情时取消请求，迟到结果不会重新打开弹窗", async () => {
  let signal: AbortSignal | null | undefined;
  let release!: (response: Response) => void;
  vi.stubGlobal(
    "fetch",
    vi.fn((_input: string, init?: RequestInit) => {
      signal = init?.signal;
      return new Promise<Response>((resolve) => {
        release = resolve;
      });
    }),
  );
  render(
    <QueryClientProvider client={client()}>
      <DetailHarness />
    </QueryClientProvider>,
  );
  await screen.findByRole("status", { name: "正在读取任务详情" });
  try {
    await userEvent.setup().keyboard("{Escape}");
    expect(signal?.aborted).toBe(true);
  } finally {
    await act(async () => release(Response.json(task("first-task"))));
  }
  expect(screen.queryByRole("dialog", { name: "任务详情" })).not.toBeInTheDocument();
});

it("切换任务时取消上一项读取，旧响应不会覆盖当前任务", async () => {
  let signal: AbortSignal | null | undefined;
  let release!: (response: Response) => void;
  vi.stubGlobal(
    "fetch",
    vi.fn((input: string, init?: RequestInit) => {
      if (input.endsWith("second-task")) return Promise.resolve(Response.json(task("second-task")));
      signal = init?.signal;
      return new Promise<Response>((resolve) => {
        release = resolve;
      });
    }),
  );
  const queryClient = client();
  const view = render(
    <QueryClientProvider client={queryClient}>
      <TaskDetailsDialog taskId="first-task" onClose={() => {}} />
    </QueryClientProvider>,
  );
  await screen.findByRole("status", { name: "正在读取任务详情" });
  try {
    view.rerender(
      <QueryClientProvider client={queryClient}>
        <TaskDetailsDialog taskId="second-task" onClose={() => {}} />
      </QueryClientProvider>,
    );
    expect(await screen.findByText("second-task 已完成")).toBeVisible();
    expect(signal?.aborted).toBe(true);
  } finally {
    await act(async () => release(Response.json(task("first-task"))));
  }
  expect(screen.getByText("second-task 已完成")).toBeVisible();
  expect(screen.queryByText("first-task 已完成")).not.toBeInTheDocument();
});

it("关闭计划任务详情时取消关联的正式执行读取", async () => {
  let signal: AbortSignal | null | undefined;
  let release!: (response: Response) => void;
  vi.stubGlobal(
    "fetch",
    vi.fn((input: string, init?: RequestInit) => {
      if (input.endsWith("first-task"))
        return Promise.resolve(
          Response.json({
            ...task("first-task"),
            operation: "account-model-automation",
            result: { execution_task_id: "execution-task" },
          }),
        );
      signal = init?.signal;
      return new Promise<Response>((resolve) => {
        release = resolve;
      });
    }),
  );
  render(
    <QueryClientProvider client={client()}>
      <DetailHarness />
    </QueryClientProvider>,
  );
  await screen.findByRole("status", { name: "正在读取正式执行任务" });
  try {
    await userEvent.setup().keyboard("{Escape}");
    expect(signal?.aborted).toBe(true);
  } finally {
    await act(async () => release(Response.json(task("execution-task"))));
  }
  expect(screen.queryByRole("dialog", { name: "任务详情" })).not.toBeInTheDocument();
});
