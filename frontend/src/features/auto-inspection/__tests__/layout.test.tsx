import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";

import { AutoInspectionPage } from "@/App";
import type { AutoInspectionStatus } from "@/api";

const clients: QueryClient[] = [];

afterEach(() => {
  clients.splice(0).forEach((client) => client.clear());
  vi.unstubAllGlobals();
});

function renderInspection(fetchInitially = false): AutoInspectionStatus {
  const client = new QueryClient({
    defaultOptions: { queries: { enabled: fetchInitially, retry: false } },
  });
  clients.push(client);
  const status: AutoInspectionStatus = {
    enabled: false,
    interval_seconds: 15,
    running: false,
    monitoring_configured: true,
    monitoring_enabled: false,
    monitoring_checked_at: null,
    last_run_duration_ms: 1000,
    last_summary: {
      channels: 1,
      probed: 1,
      samples: 1,
      fused: 0,
      recovered: 0,
      applied: 0,
      cleaned_up: 0,
      alerts: 0,
    },
    last_run_at: null,
    next_run_at: null,
    last_status: "partial",
    last_error: null,
    last_task_id: null,
    queue: [],
    heartbeat_history: [
      {
        checked_at: "2026-09-15T08:00:00Z",
        completed_at: "2026-09-15T08:00:01Z",
        status: "partial",
        operations: ["active_probe"],
        operation_timings: [],
        task_id: null,
        error: "主动探测失败，请检查上游连接。",
        skipped: false,
      },
    ],
  };
  if (!fetchInitially) client.setQueryData(["auto-inspection"], status);
  render(
    <QueryClientProvider client={client}>
      <AutoInspectionPage />
    </QueryClientProvider>,
  );
  return status;
}

it("心跳部分失败时在同一执行概况中显示操作与原因，并保留详情入口", () => {
  renderInspection();
  const table = screen.getByRole("table", { name: "巡检心跳记录" });
  expect(within(table).getAllByRole("columnheader")).toHaveLength(5);
  const summary = within(table).getByRole("cell", { name: /主动探测.*主动探测失败/ });
  expect(summary).toHaveTextContent("主动探测失败，请检查上游连接。");
  expect(within(table).getByRole("button", { name: "查看心跳详情" })).toBeEnabled();
});

it("任务队列为空时仍提供可用键盘聚焦的滚动区域和空态", async () => {
  const user = userEvent.setup();
  renderInspection();
  const queue = screen.getByRole("region", { name: "巡检任务队列" });
  expect(queue).toHaveAttribute("tabindex", "0");
  expect(within(queue).getByText(/暂无调度计划/)).toBeVisible();
  await user.click(queue);
  expect(queue).toHaveFocus();
});

it("心跳周期为空并保存时，将校验原因关联到输入框", async () => {
  const user = userEvent.setup();
  renderInspection();
  const interval = screen.getByRole("spinbutton", { name: "调度心跳周期" });
  await user.clear(interval);
  await user.click(screen.getByRole("button", { name: "保存自动巡检" }));
  expect(interval).toHaveAttribute("aria-invalid", "true");
  expect(interval).toHaveAccessibleDescription("调度心跳必须为 15 到 86400 秒");
  await user.type(interval, "30");
  expect(interval).toHaveAttribute("aria-invalid", "false");
  expect(screen.queryByText("调度心跳必须为 15 到 86400 秒")).not.toBeInTheDocument();
});

it("首次读取期间显示设置骨架并禁止保存，读取完成后显示配置", async () => {
  let finish!: (response: Response) => void;
  vi.stubGlobal(
    "fetch",
    () =>
      new Promise<Response>((resolve) => {
        finish = resolve;
      }),
  );
  const status = renderInspection(true);
  const loading = screen.getByRole("status", { name: "正在读取巡检服务" });
  expect(loading).toHaveAttribute("aria-busy", "true");
  expect(loading.querySelector('[data-slot="skeleton"]')).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "保存自动巡检" })).toBeDisabled();
  expect(screen.queryByRole("progressbar")).not.toBeInTheDocument();
  await act(async () => finish(Response.json(status)));
  expect(await screen.findByRole("spinbutton", { name: "调度心跳周期" })).toHaveValue(15);
  expect(screen.getByRole("button", { name: "保存自动巡检" })).toBeEnabled();
});

it("首次读取失败时可在设置区重新读取，成功后恢复编辑", async () => {
  const user = userEvent.setup();
  let response = Response.json({ detail: "巡检状态暂不可用" }, { status: 503 });
  vi.stubGlobal("fetch", async () => response);
  const status = renderInspection(true);
  const retry = await screen.findByRole("button", { name: "重新读取" });
  expect(screen.getByRole("button", { name: "保存自动巡检" })).toBeDisabled();
  response = Response.json(status);
  await user.click(retry);
  expect(await screen.findByRole("spinbutton", { name: "调度心跳周期" })).toHaveValue(15);
  expect(screen.queryByRole("button", { name: "重新读取" })).not.toBeInTheDocument();
});
