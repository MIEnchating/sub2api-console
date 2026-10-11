import { QueryClientProvider } from "@tanstack/react-query";
import userEvent from "@testing-library/user-event";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { Toaster, toast } from "sonner";
import { createConsoleQueryClient } from "@/lib/query-client";
import { LogDetailsDialog } from "../log-details-dialog";

afterEach(() => {
  cleanup();
  toast.dismiss();
  vi.unstubAllGlobals();
});

it("已有详情刷新失败后重试期间保留结果并禁用重复读取", async () => {
  const client = createConsoleQueryClient();
  client.setDefaultOptions({ queries: { retry: false } });
  client.setQueryData(["task", "cached-task"], {
    id: "cached-task",
    skill: "console",
    operation: "active-probe",
    status: "succeeded",
    message: "已保存的探活结果",
    progress: 100,
    result: {},
    created_at: "2026-10-09T00:00:00Z",
    updated_at: "2026-10-09T00:00:01Z",
  });
  let finish!: (response: Response) => void;
  const fetch = vi
    .fn()
    .mockResolvedValueOnce(Response.json({ detail: "详情暂不可读" }, { status: 503 }))
    .mockImplementationOnce(
      () =>
        new Promise<Response>((resolve) => {
          finish = resolve;
        }),
    );
  vi.stubGlobal("fetch", fetch);
  render(
    <QueryClientProvider client={client}>
      <Toaster />
      <LogDetailsDialog
        entry={{
          id: "log-cached",
          kind: "task",
          source: "task",
          source_id: "cached-task",
          occurred_at: "2026-10-09T00:00:00Z",
          title: "active-probe",
          summary: "列表摘要",
          status: "succeeded",
          actor: null,
          object_label: null,
          related_count: 0,
          details: {},
        }}
        onClose={() => {}}
      />
    </QueryClientProvider>,
  );
  try {
    const retry = await screen.findByRole("button", { name: "重新读取" });
    await userEvent.setup().click(retry);
    await waitFor(() => expect(fetch).toHaveBeenCalledTimes(2));
    expect(screen.getByText("已保存的探活结果")).toBeVisible();
    expect(screen.getByRole("button", { name: "重新读取" })).toBeDisabled();
  } finally {
    finish?.(Response.json(client.getQueryData(["task", "cached-task"])));
    cleanup();
    client.clear();
  }
});
it("任务详情请求失败时保留日志摘要，只通过悬浮提示错误", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => Response.json({ detail: "任务详情暂不可用" }, { status: 503 })),
  );
  const client = createConsoleQueryClient();
  client.setDefaultOptions({ queries: { retry: false } });
  render(
    <QueryClientProvider client={client}>
      <Toaster />
      <LogDetailsDialog
        entry={{
          id: "log-1",
          kind: "task",
          occurred_at: "2026-09-01T00:00:00Z",
          title: "active-probe",
          summary: "正在探活账号",
          status: "running",
          actor: null,
          object_label: null,
          source: "task",
          source_id: "task-1",
          related_count: 0,
          details: { progress: 10 },
        }}
        onClose={() => {}}
      />
    </QueryClientProvider>,
  );
  await waitFor(() => expect(screen.getByText("任务详情暂不可用")).toBeVisible());
  expect(
    within(screen.getByRole("dialog")).queryByText(/读取失败|任务详情暂不可用/),
  ).not.toBeInTheDocument();
  expect(within(screen.getByRole("dialog")).getByText("正在探活账号")).toBeVisible();
  vi.stubGlobal(
    "fetch",
    vi.fn(async () =>
      Response.json({
        id: "task-1",
        skill: "console",
        operation: "active-probe",
        status: "succeeded",
        message: "探活完成",
        progress: 100,
        result: {},
        created_at: "2026-09-01T00:00:00Z",
        updated_at: "2026-09-01T00:00:10Z",
      }),
    ),
  );
  fireEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "重新读取" }));
  expect(await screen.findByText("探活完成")).toBeVisible();
  expect(
    within(screen.getByRole("dialog")).queryByRole("button", { name: "重新读取" }),
  ).not.toBeInTheDocument();
  cleanup();
  client.clear();
});
