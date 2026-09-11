import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { Task, TaskSummary } from "@/api";

import { SystemInfoPage } from "../system-info-page";

const runningTask: TaskSummary = {
  id: "probe-running",
  skill: "sub2api-connectivity-test",
  operation: "active-probe",
  status: "running",
  progress: 50,
  message: "正在探活",
  created_at: "2026-09-05T00:00:00Z",
  updated_at: "2026-09-05T00:00:01Z",
  system_info: true,
};

const completedTask: Task = {
  ...runningTask,
  id: "probe-completed",
  status: "succeeded",
  progress: 100,
  message: "探活完成",
  result: {
    platform: "openai",
    model: "gpt-5.6-sol",
    results: [
      {
        account_id: "41",
        account_name: "生产主账号",
        result: "通过",
        request_model: "gpt-5.6-sol",
        actual_model: "gpt-5.6-sol",
        status_code: 200,
      },
    ],
  },
};

const automaticInspectionTask = {
  ...runningTask,
  id: "automatic-inspection-1",
  operation: "automatic-inspection",
  message: "自动巡检中",
  system_info: undefined,
};

const unrelatedTask = {
  ...runningTask,
  id: "account-rate-sync-1",
  operation: "account-rate-sync",
  message: "账号倍率同步完成",
  system_info: undefined,
};

function renderPage(
  taskRows: unknown[] = [unrelatedTask, automaticInspectionTask, runningTask, completedTask],
) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Number.POSITIVE_INFINITY } },
  });
  queryClient.setQueryData(["tasks"], taskRows);
  queryClient.setQueryData(["accounts"], []);
  queryClient.setQueryData(["task", completedTask.id], completedTask);
  queryClient.setQueryData(["system-metrics"], {
    sampled_at: "2026-09-05T00:00:00Z",
    cpu: { usage_percent: 37.5, logical_cores: 8 },
    memory: {
      total_bytes: 16 * 1024 ** 3,
      used_bytes: 10 * 1024 ** 3,
      available_bytes: 6 * 1024 ** 3,
      usage_percent: 62.5,
    },
    disk: {
      total_bytes: 200 * 1024 ** 3,
      used_bytes: 80 * 1024 ** 3,
      available_bytes: 120 * 1024 ** 3,
      usage_percent: 40,
    },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <SystemInfoPage />
    </QueryClientProvider>,
  );
}

describe("系统信息任务页面", () => {
  it("显示 CPU、内存和硬盘实时占用", () => {
    renderPage();

    expect(screen.getByText("CPU 占用")).toBeVisible();
    expect(screen.getByText("37.5%")).toBeVisible();
    expect(screen.getByText("8 个逻辑核心")).toBeVisible();
    expect(screen.getByText("内存占用")).toBeVisible();
    expect(screen.getByText("10 GB / 16 GB")).toBeVisible();
    expect(screen.getByText("硬盘占用")).toBeVisible();
    expect(screen.getByText("80 GB / 200 GB")).toBeVisible();
  });

  it("分别展示进行中任务和历史任务", () => {
    renderPage();

    expect(screen.getByRole("tab", { name: "进行中任务 1" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    expect(screen.getByText("正在探活")).toBeVisible();
    expect(screen.queryByText("自动巡检中")).toBeNull();
    expect(screen.queryByText("账号倍率同步完成")).toBeNull();
    expect(screen.queryByText("探活完成")).toBeNull();

    fireEvent.click(screen.getByRole("tab", { name: "历史任务 1" }));
    expect(screen.getByText("探活完成")).toBeVisible();
    expect(screen.queryByText("正在探活")).toBeNull();
  });

  it("进行中与历史任务合计只显示最近二十条", () => {
    const taskRows = Array.from({ length: 21 }, (_, index): TaskSummary => ({
      ...runningTask,
      id: `recent-task-${index}`,
      message: `最近任务 ${index}`,
    }));

    renderPage(taskRows);

    expect(screen.getByRole("tab", { name: "进行中任务 20" })).toBeVisible();
    expect(screen.getByText("最近任务 19")).toBeVisible();
    expect(screen.queryByText("最近任务 20")).toBeNull();
  });

  it("可以查看平台模型探活的账号名称与分类结果", () => {
    renderPage();
    fireEvent.click(screen.getByRole("tab", { name: "历史任务 1" }));
    fireEvent.click(screen.getByRole("button", { name: "查看任务" }));

    expect(screen.getByRole("dialog", { name: "任务详情" })).toBeVisible();
    expect(screen.getByText("OpenAI · gpt-5.6-sol")).toBeVisible();
    expect(screen.getByRole("tab", { name: "成功 1" })).toBeVisible();
    expect(screen.getByText("生产主账号")).toBeVisible();
    expect(screen.getByText("ID 41")).toBeVisible();

    const resultTable = screen
      .getByRole("dialog", { name: "任务详情" })
      .querySelector('[data-slot="table-container"]');
    expect(resultTable).toHaveClass("max-h-[min(32rem,calc(100svh-18rem))]");
  });
});

afterEach(() => vi.unstubAllGlobals());
it("打开未缓存任务时显示轻量读取状态，失败后可以重新读取", async () => {
  vi.stubGlobal(
    "fetch",
    vi
      .fn()
      .mockResolvedValueOnce(Response.json({ detail: "任务读取失败" }, { status: 502 }))
      .mockResolvedValue(Response.json(completedTask)),
  );
  const view = renderPage([{ ...completedTask, id: "uncached" }]);
  fireEvent.click(screen.getByRole("tab", { name: "历史任务 1" }));
  fireEvent.click(screen.getByRole("button", { name: "查看任务" }));
  expect(screen.getByRole("status", { name: "正在读取任务详情" })).toHaveTextContent(
    "正在读取任务详情",
  );
  expect(screen.getByRole("dialog").querySelector('[data-slot="skeleton"]')).toBeNull();
  fireEvent.click(await screen.findByRole("button", { name: "重新读取" }, { timeout: 5_000 }));
  expect(await screen.findByText("OpenAI · gpt-5.6-sol")).toBeVisible();
  view.unmount();
});
