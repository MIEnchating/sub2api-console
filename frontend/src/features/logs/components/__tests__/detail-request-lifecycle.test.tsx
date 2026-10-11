import { QueryClient, QueryClientProvider, useQuery } from "@tanstack/react-query";
import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState, type ReactElement } from "react";
import { afterEach, expect, it, vi } from "vitest";

import { api, type Task, type UnifiedLogEntry } from "@/api";
import { LogDetailsDialog } from "../log-details-dialog";

const clients: QueryClient[] = [];
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  vi.unstubAllGlobals();
});

function entry(id: string): UnifiedLogEntry {
  return {
    id: `log-${id}`,
    kind: "task",
    source: "task",
    source_id: id,
    occurred_at: "2026-10-09T00:00:00Z",
    title: "active-probe",
    summary: `${id} 摘要`,
    status: "running",
    actor: null,
    object_label: null,
    related_count: 0,
    details: {},
  };
}

function task(id: string): Task {
  return {
    id,
    skill: "console",
    operation: "active-probe",
    status: "succeeded",
    message: `${id} 完整结果`,
    progress: 100,
    result: {},
    created_at: "2026-10-09T00:00:00Z",
    updated_at: "2026-10-09T00:00:01Z",
  };
}

function createClient(): QueryClient {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  clients.push(client);
  return client;
}

function DialogHarness(): ReactElement {
  const [selected, setSelected] = useState<UnifiedLogEntry | null>(entry("first"));
  return <LogDetailsDialog entry={selected} onClose={() => setSelected(null)} />;
}

it("关闭日志详情时取消完整任务读取，迟到结果不会进入缓存或重新打开弹窗", async () => {
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
  render(
    <QueryClientProvider client={client}>
      <DialogHarness />
    </QueryClientProvider>,
  );
  await screen.findByRole("status", { name: "正在读取完整任务结果…" });
  await userEvent.setup().keyboard("{Escape}");
  await act(async () => finish(Response.json(task("first"))));
  expect(signal?.aborted).toBe(true);
  expect(client.getQueryData(["task", "first"])).toBeUndefined();
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  expect(fetch).toHaveBeenCalledTimes(1);
});

it("切换日志任务时取消旧读取并保留新任务完整结果", async () => {
  let signal: AbortSignal | null | undefined;
  let finish!: (response: Response) => void;
  vi.stubGlobal(
    "fetch",
    vi.fn((input: string, init?: RequestInit) => {
      if (input.endsWith("second")) return Promise.resolve(Response.json(task("second")));
      signal = init?.signal;
      return new Promise<Response>((resolve) => {
        finish = resolve;
      });
    }),
  );
  const client = createClient();
  const view = render(
    <QueryClientProvider client={client}>
      <LogDetailsDialog entry={entry("first")} onClose={() => {}} />
    </QueryClientProvider>,
  );
  await screen.findByRole("status", { name: "正在读取完整任务结果…" });
  view.rerender(
    <QueryClientProvider client={client}>
      <LogDetailsDialog entry={entry("second")} onClose={() => {}} />
    </QueryClientProvider>,
  );
  await screen.findByText("second 完整结果");
  await act(async () => finish(Response.json(task("first"))));
  expect(signal?.aborted).toBe(true);
  expect(screen.getByText("second 完整结果")).toBeVisible();
  expect(screen.queryByText("first 完整结果")).not.toBeInTheDocument();
});

it("事件与缓存任务同 ID 时只展示事件且不提供取消任务操作", () => {
  const client = createClient();
  client.setQueryData(["task", "same"], { ...task("same"), status: "running" });
  const event: UnifiedLogEntry = {
    ...entry("same"),
    kind: "event",
    source: "runtime_event",
    summary: "本次事件已记录",
  };
  const fetch = vi.fn();
  vi.stubGlobal("fetch", fetch);
  render(
    <QueryClientProvider client={client}>
      <LogDetailsDialog entry={event} onClose={() => {}} />
    </QueryClientProvider>,
  );
  expect(screen.getByText("本次事件已记录")).toBeVisible();
  expect(screen.queryByRole("button", { name: "取消任务" })).not.toBeInTheDocument();
  expect(screen.queryByText("same 完整结果")).not.toBeInTheDocument();
  expect(fetch).not.toHaveBeenCalled();
});

function OtherTaskViewer(): ReactElement {
  const query = useQuery({
    queryKey: ["task", "first"],
    queryFn: ({ signal }) => api.task("first", signal),
  });
  return <p>{query.data?.message ?? "其他组件等待读取"}</p>;
}

it("关闭日志详情时其他组件仍观察同一任务则共享读取继续", async () => {
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
      <DialogHarness />
      <OtherTaskViewer />
    </QueryClientProvider>,
  );
  await waitFor(() => expect(fetch).toHaveBeenCalledTimes(1));
  await userEvent.setup().keyboard("{Escape}");
  await act(async () => finish(Response.json(task("first"))));
  expect(await screen.findByText("first 完整结果")).toBeVisible();
  expect(signal?.aborted).not.toBe(true);
  expect(fetch).toHaveBeenCalledTimes(1);
});
