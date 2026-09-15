import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";

import { AutoInspectionPage } from "@/App";
import type { AutoInspectionStatus, Task } from "@/api";

const clients: QueryClient[] = [];

afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  vi.unstubAllGlobals();
});

function inspectionStatus(taskID: string | null = "inspection-live"): AutoInspectionStatus {
  return {
    enabled: true,
    interval_seconds: 15,
    running: true,
    monitoring_configured: true,
    monitoring_enabled: true,
    monitoring_checked_at: null,
    last_run_duration_ms: 0,
    last_summary: {
      channels: 0,
      probed: 0,
      samples: 0,
      fused: 0,
      recovered: 0,
      applied: 0,
      cleaned_up: 0,
      alerts: 0,
    },
    last_run_at: null,
    next_run_at: null,
    last_status: null,
    last_error: null,
    last_task_id: taskID,
    queue: [],
    heartbeat_history: [
      {
        checked_at: "2026-09-14T08:00:00Z",
        completed_at: null,
        status: "running",
        operations: [],
        operation_timings: [],
        task_id: taskID,
        error: null,
        skipped: false,
      },
    ],
  };
}

function inspectionTask(id: string, message: string): Task {
  return {
    id,
    skill: "sub2api-auto-inspection",
    operation: "automatic-inspection",
    status: "running",
    progress: 35,
    message,
    result: {},
    created_at: "2026-09-14T08:00:00Z",
    updated_at: "2026-09-14T08:00:01Z",
  };
}

function renderInspection(status: AutoInspectionStatus = inspectionStatus()): QueryClient {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  });
  clients.push(client);
  client.setQueryData(["auto-inspection"], status);
  render(
    <QueryClientProvider client={client}>
      <AutoInspectionPage />
    </QueryClientProvider>,
  );
  return client;
}

it("巡检运行时无需打开详情即可读取并显示当前执行阶段", async () => {
  vi.stubGlobal("fetch", async () =>
    Response.json(inspectionTask("inspection-live", "正在执行主动探测")),
  );
  renderInspection();

  expect(await screen.findByRole("cell", { name: "正在执行主动探测" })).toBeVisible();
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  expect(screen.queryByRole("cell", { name: "正在检查到期任务" })).not.toBeInTheDocument();
});

it("当前任务阶段更新时心跳行同步显示后端最新阶段", async () => {
  let current = inspectionTask("inspection-live", "正在执行主动探测");
  vi.stubGlobal("fetch", async () => Response.json(current));
  const client = renderInspection();
  await screen.findByRole("cell", { name: "正在执行主动探测" });

  current = inspectionTask("inspection-live", "正在自动恢复上游鉴权");
  await act(async () => {
    await client.invalidateQueries({ queryKey: ["auto-inspection-heartbeat-task", current.id] });
  });

  expect(await screen.findByRole("cell", { name: "正在自动恢复上游鉴权" })).toBeVisible();
  expect(screen.queryByRole("cell", { name: "正在执行主动探测" })).not.toBeInTheDocument();
});

it("切换巡检轮次后新任务未返回时不复用上一轮阶段", async () => {
  let finishNext!: (response: Response) => void;
  const nextTask = new Promise<Response>((resolve) => {
    finishNext = resolve;
  });
  vi.stubGlobal("fetch", async (input: string) => {
    if (input.endsWith("/inspection-next")) return nextTask;
    return Response.json(inspectionTask("inspection-live", "正在执行主动探测"));
  });
  const client = renderInspection();
  await screen.findByRole("cell", { name: "正在执行主动探测" });

  act(() => {
    client.setQueryData(["auto-inspection"], inspectionStatus("inspection-next"));
  });

  expect(await screen.findByRole("cell", { name: "正在执行本轮巡检任务" })).toBeVisible();
  expect(screen.queryByRole("cell", { name: "正在执行主动探测" })).not.toBeInTheDocument();
  await act(async () => {
    finishNext(Response.json(inspectionTask("inspection-next", "正在同步上游数据")));
  });
  expect(await screen.findByRole("cell", { name: "正在同步上游数据" })).toBeVisible();
});

it("运行中任务读取失败时仍显示执行状态并保留详情入口", async () => {
  vi.stubGlobal("fetch", async () => Response.json({ detail: "暂时不可用" }, { status: 503 }));
  const client = renderInspection();

  await waitFor(() =>
    expect(
      client.getQueryState(["auto-inspection-heartbeat-task", "inspection-live"])?.status,
    ).toBe("error"),
  );

  expect(screen.getByRole("cell", { name: "正在执行本轮巡检任务" })).toBeVisible();
  expect(screen.getByRole("button", { name: "查看心跳详情" })).toBeEnabled();
  expect(screen.queryByText("暂时不可用")).not.toBeInTheDocument();
});

it("心跳尚未创建任务时保持到期检查提示且不请求任务详情", () => {
  const fetcher = vi.fn();
  vi.stubGlobal("fetch", fetcher);
  renderInspection(inspectionStatus(null));

  expect(screen.getByRole("cell", { name: "正在检查到期任务" })).toBeVisible();
  expect(fetcher).not.toHaveBeenCalled();
});
