import type { ReactNode } from "react";
import { QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import type { RenderHookResult } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";

import { createConsoleQueryClient } from "@/lib/query-client";
import { task } from "../../components/__tests__/fixtures";
import { useTaskCompletion } from "../use-task-completion";

const clients: ReturnType<typeof createConsoleQueryClient>[] = [];

afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  vi.unstubAllGlobals();
});

function setup(): RenderHookResult<ReturnType<typeof useTaskCompletion>, unknown> {
  const client = createConsoleQueryClient();
  clients.push(client);
  return renderHook(useTaskCompletion, {
    wrapper: (props: { children: ReactNode }) => (
      <QueryClientProvider client={client}>{props.children}</QueryClientProvider>
    ),
  });
}

it("页面关闭后写操作才返回任务时结束等待并提示在日志中心查看", async () => {
  const view = setup();
  let finishWrite!: (value: typeof task) => void;
  const write = new Promise<typeof task>((resolve) => (finishWrite = resolve));
  const completion = write.then(view.result.current.wait);
  view.unmount();

  finishWrite({ ...task, status: "running" });

  await expect(completion).rejects.toThrow("页面已关闭，后台任务仍可在日志中心查看");
});

it("任务已进入进度等待后关闭页面时结束等待并保留后台执行提示", async () => {
  vi.stubGlobal("fetch", async () => Response.json({ ...task, status: "running" }));
  const view = setup();
  let completion!: Promise<typeof task>;
  act(() => {
    completion = view.result.current.wait({ ...task, status: "running" });
  });

  view.unmount();

  await expect(completion).rejects.toThrow("页面已关闭，后台任务仍可在日志中心查看");
});

it("进度读取未返回时离页中止请求，迟到终态不能写入缓存", async () => {
  let signal: AbortSignal | null | undefined;
  let finish!: (response: Response) => void;
  const fetch = vi.fn((_input: string, init?: RequestInit) => {
    signal = init?.signal;
    return new Promise<Response>((resolve) => {
      finish = resolve;
    });
  });
  vi.stubGlobal("fetch", fetch);
  const view = setup();
  let completion!: Promise<typeof task>;
  act(() => {
    completion = view.result.current.wait({ ...task, status: "running" });
  });
  const rejected = expect(completion).rejects.toThrow("页面已关闭，后台任务仍可在日志中心查看");
  await waitFor(() => expect(fetch).toHaveBeenCalledTimes(1));
  view.unmount();
  await rejected;
  await act(async () => finish(Response.json({ ...task, status: "succeeded" })));
  expect(signal?.aborted).toBe(true);
  expect(clients[0].getQueryData(["tasks", task.id])).toBeUndefined();
  expect(fetch).toHaveBeenCalledTimes(1);
});
