import type { ReactNode } from "react";
import { QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { toast } from "sonner";

import { createConsoleQueryClient } from "@/lib/query-client";
import { notifyOperationError } from "@/lib/operation-feedback";
import { task } from "../../components/__tests__/fixtures";
import { useTaskCompletion } from "../use-task-completion";

const clients: ReturnType<typeof createConsoleQueryClient>[] = [];
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  toast.dismiss();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

it("任务轮询失败时只提示一次，并告知先核对后台结果再重试", async () => {
  vi.stubGlobal("fetch", async () => Response.json({ detail: "连接中断" }, { status: 502 }));
  const notify = vi.spyOn(toast, "error");
  const client = createConsoleQueryClient();
  clients.push(client);
  const view = renderHook(useTaskCompletion, {
    wrapper: (props: { children: ReactNode }) => (
      <QueryClientProvider client={client}>{props.children}</QueryClientProvider>
    ),
  });
  let completion!: Promise<void>;
  act(() => {
    completion = view.result.current.wait({ ...task, status: "running" }).then(
      () => {
        throw new Error("失败查询不应当作成功");
      },
      (error: unknown) => notifyOperationError(error, "监控操作失败"),
    );
  });
  await act(async () => {
    await completion;
  });

  expect(notify).toHaveBeenCalledTimes(1);
  expect(notify).toHaveBeenCalledWith(
    expect.stringMatching(/任务 test-task 的进度读取失败.*日志中心.*连接中断/),
    expect.any(Object),
  );
});

it("任务轮询遇到登录过期时不额外弹出操作失败", async () => {
  vi.stubGlobal("fetch", async () => Response.json({}, { status: 401 }));
  const notify = vi.spyOn(toast, "error");
  const client = createConsoleQueryClient();
  clients.push(client);
  const view = renderHook(useTaskCompletion, {
    wrapper: (props: { children: ReactNode }) => (
      <QueryClientProvider client={client}>{props.children}</QueryClientProvider>
    ),
  });
  let completion!: Promise<void>;
  act(() => {
    completion = view.result.current.wait({ ...task, status: "running" }).then(
      () => {
        throw new Error("过期查询不应当作成功");
      },
      (error: unknown) => notifyOperationError(error, "监控操作失败"),
    );
  });
  await waitFor(() => expect(view.result.current.task).toBeNull());
  await completion;
  expect(notify).not.toHaveBeenCalled();
});
