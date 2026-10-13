import { QueryClient, QueryClientProvider, useQuery } from "@tanstack/react-query";
import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, expect, it, vi } from "vitest";
import { api, type Task } from "@/api";
import { DetectionTaskDetails } from "../detection-task-details";

const clients: QueryClient[] = [];
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  vi.unstubAllGlobals();
});

const task: Task = {
  id: "run-1",
  skill: "console",
  operation: "managed-model-detection",
  status: "running",
  progress: 0,
  message: "检测执行中",
  result: {},
  created_at: "2026-10-09T00:00:00Z",
  updated_at: "2026-10-09T00:00:00Z",
};

function Details() {
  const [open, setOpen] = useState(true);
  return open ? <DetectionTaskDetails id="run-1" onClose={() => setOpen(false)} /> : null;
}

function SharedTask() {
  const query = useQuery({
    queryKey: ["task", "run-1"],
    queryFn: ({ signal }) => api.task("run-1", signal),
  });
  return <output>{query.data?.message}</output>;
}

function setup(shared = false) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  });
  clients.push(client);
  client.setQueryData(["groups", "options"], []);
  let signal: AbortSignal | null | undefined;
  let release!: (response: Response) => void;
  const fetcher = vi.fn<typeof fetch>().mockImplementation((_input, init) => {
    signal = init?.signal;
    return new Promise<Response>((resolve) => {
      release = resolve;
    });
  });
  vi.stubGlobal("fetch", fetcher);
  render(
    <QueryClientProvider client={client}>
      <Details />
      {shared ? <SharedTask /> : null}
    </QueryClientProvider>,
  );
  return {
    client,
    fetcher,
    signal: () => signal,
    release: (response: Response) => release(response),
  };
}

it("关闭慢速检测详情时释放读取，迟到响应不会复活详情或取消后台任务", async () => {
  const view = setup();
  await screen.findByRole("status", { name: "正在读取检测任务" });
  try {
    await userEvent.setup().keyboard("{Escape}");
    expect(view.signal()?.aborted).toBe(true);
  } finally {
    await act(async () => view.release(Response.json(task)));
  }
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  expect(view.client.getQueryData(["task", "run-1"])).toBeUndefined();
  expect(view.fetcher.mock.calls.every(([, init]) => !init?.method || init.method === "GET")).toBe(
    true,
  );
});

it("检测详情关闭后有其他观察者时保留共享读取并向其提供结果", async () => {
  const view = setup(true);
  await screen.findByRole("status", { name: "正在读取检测任务" });
  await userEvent.setup().keyboard("{Escape}");
  expect(view.signal()?.aborted).toBe(false);
  await act(async () => view.release(Response.json(task)));
  await waitFor(() => expect(screen.getByRole("status")).toHaveTextContent(task.message));
  expect(view.fetcher).toHaveBeenCalledTimes(1);
});
