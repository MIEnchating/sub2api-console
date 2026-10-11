import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import type { Task } from "@/api";
import { RegularCheckPanel } from "../regular-check-panel";

const task: Task = {
  id: "check-41",
  skill: "console",
  operation: "model-check",
  status: "running",
  progress: 20,
  message: "旧任务状态",
  result: {},
  created_at: "2026-10-09T00:00:00Z",
  updated_at: "2026-10-09T00:00:01Z",
};

class TaskStream {
  static current: TaskStream;
  onopen: (() => void) | null = null;
  onerror: (() => void) | null = null;
  onmessage: ((event: MessageEvent<string>) => void) | null = null;
  close = vi.fn();
  constructor() {
    TaskStream.current = this;
  }
  send(value: unknown): void {
    this.onmessage?.(new MessageEvent("message", { data: JSON.stringify(value) }));
  }
}

const clients: QueryClient[] = [];
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

async function setup() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  });
  clients.push(client);
  client.setQueryData(
    ["accounts"],
    [{ id: "41", name: "测试账号", groups: [], platform: "openai" }],
  );
  client.setQueryData(["model-check-capabilities"], { claude_standards: [], sol_models: [] });
  client.setQueryData(["model-check-account-models", "41"], { models: [] });
  client.setQueryData(
    ["model-check-account-statuses"],
    [{ account_id: "41", status: "consistent", checked_at: task.updated_at, task_id: task.id }],
  );
  client.setQueryData(["dictionaries", "group"], { items: [] });
  let signal: AbortSignal | null | undefined;
  let release!: (response: Response) => void;
  const fetcher = vi.fn<typeof fetch>().mockImplementation((input, init) => {
    if (String(input) === "/api/tasks/check-41") {
      signal = init?.signal;
      return new Promise<Response>((resolve) => {
        release = resolve;
      });
    }
    return Promise.resolve(Response.json([]));
  });
  vi.stubGlobal("fetch", fetcher);
  vi.stubGlobal("EventSource", TaskStream);
  const view = render(
    <QueryClientProvider client={client}>
      <RegularCheckPanel accountID="41" />
    </QueryClientProvider>,
  );
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "查看上次检测结果" }));
  await waitFor(() =>
    expect(fetcher).toHaveBeenCalledWith("/api/tasks/check-41", expect.anything()),
  );
  return {
    ...view,
    client,
    fetcher,
    signal: () => signal,
    release: (response: Response) => release(response),
  };
}

it.each(["running", "succeeded"] as const)(
  "HTTP 任务读取晚于%s实时事件到达时保留推送的新状态",
  async (status) => {
    const view = await setup();
    const next = {
      ...task,
      status,
      progress: 80,
      message: "实时任务状态",
      updated_at: "2026-10-09T00:00:02Z",
    };
    try {
      await act(async () => TaskStream.current.send(next));
      await waitFor(() =>
        expect(view.client.getQueryData(["model-check-task", task.id])).toEqual(next),
      );
    } finally {
      await act(async () => view.release(Response.json(task)));
    }
    expect(view.client.getQueryData(["model-check-task", task.id])).toEqual(next);
    expect(view.signal()?.aborted).toBe(true);
    if (status === "succeeded") expect(TaskStream.current.close).toHaveBeenCalledOnce();
  },
);

it.each([
  { label: "空任务", value: null },
  { label: "其他任务", value: { ...task, id: "other-task" } },
  { label: "错误状态", value: { ...task, status: "broken" } },
  { label: "缺失结果", value: { ...task, result: null } },
  { label: "无效更新时间", value: { ...task, updated_at: "invalid" } },
])("实时事件为$label时保留当前任务且不取消可靠的HTTP读取", async (scenario) => {
  const view = await setup();
  view.client.setQueryData(["model-check-task", task.id], task);
  try {
    await act(async () => TaskStream.current.send(scenario.value));
    expect(view.client.getQueryData(["model-check-task", task.id])).toEqual(task);
    expect(view.signal()?.aborted).toBe(false);
  } finally {
    await act(async () => view.release(Response.json(task)));
  }
});

it.each([
  {
    label: "终态HTTP之后的旧运行事件",
    status: "succeeded" as const,
    currentTime: "2026-10-09T00:00:02Z",
    eventTime: "2026-10-09T00:00:01Z",
  },
  {
    label: "同一毫秒内的旧事件",
    status: "running" as const,
    currentTime: "2026-10-09T00:00:01.123456790Z",
    eventTime: "2026-10-09T00:00:01.123456789Z",
  },
  {
    label: "跨秒且小数位数不同的旧事件",
    status: "running" as const,
    currentTime: "2026-10-09T00:00:02Z",
    eventTime: "2026-10-09T00:00:01.999999999Z",
  },
  {
    label: "时区偏移不同的旧事件",
    status: "running" as const,
    currentTime: "2026-10-09T00:00:02.123456790Z",
    eventTime: "2026-10-09T08:00:02.123456789+08:00",
  },
])("$label到达时保留较新的HTTP快照", async (scenario) => {
  const view = await setup();
  const current = {
    ...task,
    status: scenario.status,
    progress: 80,
    message: "HTTP最新状态",
    updated_at: scenario.currentTime,
  };
  await act(async () => view.release(Response.json(current)));
  await waitFor(() =>
    expect(view.client.getQueryData(["model-check-task", task.id])).toEqual(current),
  );
  await act(async () => TaskStream.current.send({ ...task, updated_at: scenario.eventTime }));
  expect(view.client.getQueryData(["model-check-task", task.id])).toEqual(current);
  expect(screen.queryByText("旧任务状态")).not.toBeInTheDocument();
  expect(TaskStream.current.close).not.toHaveBeenCalled();
});

it("较新恢复事件到达时允许中断的任务重新排队", async () => {
  const view = await setup();
  const interrupted = { ...task, status: "failed", result: { interrupted: true } };
  await act(async () => view.release(Response.json(interrupted)));
  await waitFor(() =>
    expect(view.client.getQueryData(["model-check-task", task.id])).toEqual(interrupted),
  );
  const resumed = { ...task, status: "queued", updated_at: "2026-10-09T00:00:02Z" };
  await act(async () => TaskStream.current.send(resumed));
  expect(view.client.getQueryData(["model-check-task", task.id])).toEqual(resumed);
  expect(TaskStream.current.close).not.toHaveBeenCalled();
});

it("离开普通模型检测时释放任务读取和事件连接，不发送业务取消", async () => {
  const view = await setup();
  try {
    view.unmount();
    expect(view.signal()?.aborted).toBe(true);
    expect(TaskStream.current.close).toHaveBeenCalledOnce();
  } finally {
    await act(async () => view.release(Response.json(task)));
  }
  expect(view.fetcher.mock.calls.every(([, init]) => !init?.method)).toBe(true);
});

it("事件重新连接但未提供有效任务时保留轮询，收到有效任务后停止轮询", async () => {
  const view = await setup();
  await act(async () => TaskStream.current.send(task));
  vi.useFakeTimers();
  const polled = {
    ...task,
    progress: 40,
    message: "HTTP轮询状态",
    updated_at: "2026-10-09T00:00:02Z",
  };
  view.fetcher.mockImplementation(async () => Response.json(polled));
  view.fetcher.mockClear();
  await act(async () => TaskStream.current.onopen?.());
  await act(async () => {
    await vi.advanceTimersByTimeAsync(1000);
  });
  expect(view.fetcher).toHaveBeenCalledWith("/api/tasks/check-41", expect.anything());
  expect(view.client.getQueryData(["model-check-task", task.id])).toEqual(polled);
  await act(async () => TaskStream.current.send(polled));
  view.fetcher.mockClear();
  await act(async () => {
    await vi.advanceTimersByTimeAsync(2000);
  });
  expect(view.fetcher).not.toHaveBeenCalled();
});

it("有效事件后连接中断时恢复HTTP轮询", async () => {
  const view = await setup();
  await act(async () => TaskStream.current.send(task));
  vi.useFakeTimers();
  let progress = 30;
  view.fetcher.mockImplementation(async () => {
    progress += 10;
    return Response.json({ ...task, progress, message: "连接中断后轮询状态" });
  });
  view.fetcher.mockClear();
  await act(async () => TaskStream.current.onerror?.());
  await act(async () => {
    await vi.advanceTimersByTimeAsync(1000);
  });
  expect(view.fetcher).toHaveBeenCalledWith("/api/tasks/check-41", expect.anything());
  expect(view.client.getQueryData<Task>(["model-check-task", task.id])?.progress).toBe(50);
  expect(screen.getAllByText("连接中断后轮询状态").length).toBeGreaterThan(0);
});
