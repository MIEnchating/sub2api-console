import { QueryClient, QueryClientProvider, QueryObserver } from "@tanstack/react-query";
import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { Toaster, toast } from "sonner";

import type { Task } from "@/api";
import { LogCleanupAction } from "../log-cleanup-action";

afterEach(() => {
  toast.dismiss();
  vi.unstubAllGlobals();
});

function fixture(status: Task["status"] = "running"): Task {
  return {
    id: "cleanup-1",
    skill: "console",
    operation: "log-cleanup",
    status,
    progress: status === "succeeded" ? 100 : 0,
    message: status === "succeeded" ? "日志清理完成，已清理 700 条" : "清理事件记录，已清理 500 条",
    result: { total: 700, retention_days: 30 },
    created_at: "2026-10-09T00:00:00Z",
    updated_at: "2026-10-09T00:00:00Z",
  };
}

it("清理任务提交后显示实际数量，允许关闭弹窗且阻止重复提交", async () => {
  const user = userEvent.setup();
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => new Response(JSON.stringify(fixture()), { status: 202 })),
  );
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <LogCleanupAction retentionDays={30} ready />
      <Toaster />
    </QueryClientProvider>,
  );
  await user.click(screen.getByRole("button", { name: "立即按期限清理" }));
  await user.click(screen.getByRole("button", { name: "确认清理" }));
  expect(await screen.findByText("清理事件记录，已清理 500 条")).toBeInTheDocument();
  expect(screen.queryByRole("progressbar")).not.toBeInTheDocument();
  expect(screen.getByRole("button", { name: "取消任务" })).toBeEnabled();
  await user.keyboard("{Escape}");
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "查看清理进度" }));
  expect(await screen.findByText("清理事件记录，已清理 500 条")).toBeInTheDocument();
  expect(screen.queryByRole("button", { name: "确认清理" })).not.toBeInTheDocument();
  client.clear();
});

it("清理完成后立即显示成功，不等待概览刷新完成", async () => {
  const user = userEvent.setup();
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => new Response(JSON.stringify(fixture()), { status: 202 })),
  );
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <LogCleanupAction retentionDays={30} ready />
      <Toaster />
    </QueryClientProvider>,
  );
  await user.click(screen.getByRole("button", { name: "立即按期限清理" }));
  await user.click(screen.getByRole("button", { name: "确认清理" }));
  await screen.findByText("清理事件记录，已清理 500 条");
  let finishRefresh!: () => void;
  const refresh = new Promise<void>((resolve) => {
    finishRefresh = resolve;
  });
  // The external request remains pending while the task reaches its terminal state.
  const observer = new QueryObserver(client, {
    queryKey: ["overview"],
    queryFn: async () => {
      await refresh;
      return { total: 0 };
    },
  });
  const unsubscribe = observer.subscribe(() => {});
  const refreshRequest = client.fetchQuery({
    queryKey: ["overview"],
    queryFn: async () => {
      await refresh;
      return { total: 0 };
    },
  });
  act(() => {
    client.setQueryData(["task", "cleanup-1"], fixture("succeeded"));
  });
  await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  expect(await screen.findByText("日志清理完成，已清理 700 条")).toBeInTheDocument();
  finishRefresh();
  await refreshRequest;
  unsubscribe();
  client.clear();
});

it("清理完成后使检测历史和任务列表失效", async () => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  client.setQueryData(["task", "cleanup-1"], fixture("succeeded"));
  client.setQueryData(["model-animation", "history"], []);
  client.setQueryData(["tasks"], []);
  render(
    <QueryClientProvider client={client}>
      <LogCleanupAction retentionDays={30} ready activeTaskId="cleanup-1" />
      <Toaster />
    </QueryClientProvider>,
  );

  await waitFor(() => {
    expect(client.getQueryState(["model-animation", "history"])?.isInvalidated).toBe(true);
    expect(client.getQueryState(["tasks"])?.isInvalidated).toBe(true);
  });
  client.clear();
});

it("重新进入设置时能查看已有清理任务，并保留任务提交时的期限", async () => {
  const user = userEvent.setup();
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => new Response(JSON.stringify(fixture()))),
  );
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <LogCleanupAction retentionDays={7} ready activeTaskId="cleanup-1" />
    </QueryClientProvider>,
  );
  await user.click(screen.getByRole("button", { name: "查看清理进度" }));
  expect(await screen.findByText("清理事件记录，已清理 500 条")).toBeInTheDocument();
  expect(screen.getByRole("dialog")).toHaveAccessibleDescription(
    expect.stringContaining("将删除 30"),
  );
  expect(screen.queryByRole("button", { name: "确认清理" })).not.toBeInTheDocument();
  client.clear();
});

it("配置先报告清理已结束时仍跟踪原任务，直到读取到最终结果", async () => {
  const user = userEvent.setup();
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => new Response(JSON.stringify(fixture()))),
  );
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const view = render(
    <QueryClientProvider client={client}>
      <LogCleanupAction retentionDays={30} ready activeTaskId="cleanup-1" />
      <Toaster />
    </QueryClientProvider>,
  );
  await user.click(screen.getByRole("button", { name: "查看清理进度" }));
  await screen.findByText("清理事件记录，已清理 500 条");
  view.rerender(
    <QueryClientProvider client={client}>
      <LogCleanupAction retentionDays={30} ready />
      <Toaster />
    </QueryClientProvider>,
  );
  expect(screen.getByText("清理事件记录，已清理 500 条")).toBeInTheDocument();
  act(() => {
    client.setQueryData(["task", "cleanup-1"], fixture("succeeded"));
  });
  expect(await screen.findByText("日志清理完成，已清理 700 条")).toBeInTheDocument();
  client.clear();
});

it("启动请求失败时显示具体原因并允许重试", async () => {
  const user = userEvent.setup();
  vi.stubGlobal(
    "fetch",
    vi.fn(
      async () =>
        new Response(JSON.stringify({ detail: "日志清理正在执行，请等待当前清理完成" }), {
          status: 409,
        }),
    ),
  );
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <LogCleanupAction retentionDays={30} ready />
      <Toaster />
    </QueryClientProvider>,
  );
  await user.click(screen.getByRole("button", { name: "立即按期限清理" }));
  await user.click(screen.getByRole("button", { name: "确认清理" }));
  expect(await screen.findByText(/日志清理正在执行/)).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "确认清理" })).toBeEnabled();
  expect(screen.queryByRole("progressbar")).not.toBeInTheDocument();
  client.clear();
});

it("已提交部分删除后任务失败时提示已完成数量并允许重试剩余记录", async () => {
  const user = userEvent.setup();
  const failed = {
    ...fixture("failed"),
    message: "日志清理未完成，已清理 500 条：磁盘空间不足；可重新按期限清理剩余记录",
  };
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => new Response(JSON.stringify(failed))),
  );
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <LogCleanupAction retentionDays={30} ready />
      <Toaster />
    </QueryClientProvider>,
  );
  await user.click(screen.getByRole("button", { name: "立即按期限清理" }));
  await user.click(screen.getByRole("button", { name: "确认清理" }));
  expect(await screen.findByText(/已清理 500 条：磁盘空间不足/)).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "确认清理" })).toBeEnabled();
  expect(screen.queryByRole("button", { name: "取消任务" })).not.toBeInTheDocument();
  client.clear();
});

it("配置尚未读取或保留天数无效时不能提交清理", () => {
  const client = new QueryClient();
  const view = render(
    <QueryClientProvider client={client}>
      <LogCleanupAction retentionDays={30} ready={false} />
    </QueryClientProvider>,
  );
  expect(screen.getByRole("button", { name: "立即按期限清理" })).toBeDisabled();
  view.rerender(
    <QueryClientProvider client={client}>
      <LogCleanupAction retentionDays={0} ready />
    </QueryClientProvider>,
  );
  expect(screen.getByRole("button", { name: "立即按期限清理" })).toBeDisabled();
  client.clear();
});
