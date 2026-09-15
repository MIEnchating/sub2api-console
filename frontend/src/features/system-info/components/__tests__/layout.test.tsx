import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";

import type { TaskSummary } from "@/api";
import { SystemInfoPage } from "../system-info-page";

const task: TaskSummary = {
  id: "layout-probe",
  skill: "sub2api-connectivity-test",
  operation: "active-probe",
  status: "running",
  progress: 50,
  message: "正在探活",
  created_at: "2026-09-15T00:00:00Z",
  updated_at: "2026-09-15T00:00:01Z",
  system_info: true,
};

let client: QueryClient;

afterEach(() => {
  cleanup();
  client?.clear();
  vi.unstubAllGlobals();
});

function renderPage(rows: TaskSummary[] = [task]) {
  client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  });
  client.setQueryData(["tasks"], rows);
  client.setQueryData(["accounts"], []);
  client.setQueryData(["system-metrics"], {
    cpu: { usage_percent: 20, logical_cores: 4 },
    memory: { usage_percent: 30, used_bytes: 1024, total_bytes: 4096 },
    disk: { usage_percent: 40, used_bytes: 1024, total_bytes: 4096 },
  });
  client.setQueryData(["dictionaries", "task_status"], { items: [] });
  for (const row of rows) client.setQueryData(["task", row.id], { ...row, result: {} });
  return render(
    <QueryClientProvider client={client}>
      <SystemInfoPage />
    </QueryClientProvider>,
  );
}

it("任务消息和类型超长时约束在各自列内，百分比保留独立空间", () => {
  const message = "连接探活响应详细信息".repeat(40);
  const operation = "custom-operation-".repeat(20);
  renderPage([{ ...task, message, operation }]);

  const table = screen.getByRole("table");
  expect(within(table).getByText(message)).toHaveClass(
    "line-clamp-2",
    "wrap-anywhere",
    "whitespace-normal",
  );
  expect(within(table).getByText(operation)).toHaveClass("truncate");
  expect(within(table).getByText("50%")).toHaveClass("shrink-0");
  expect(screen.getByRole("progressbar", { name: "layout-probe 任务进度" })).toHaveAttribute(
    "aria-valuenow",
    "50",
  );
});

it("键盘切换任务分类时激活对应面板并清除上一分类的状态筛选", async () => {
  renderPage([task, { ...task, id: "completed-probe", status: "succeeded", message: "探活完成" }]);
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "任务状态筛选" }));
  await user.click(screen.getByRole("option", { name: "排队中" }));
  await user.keyboard("{Escape}");
  screen.getByRole("tab", { name: "进行中任务 1" }).focus();
  await user.keyboard("{ArrowRight}");

  const selected = screen.getByRole("tab", { name: "历史任务 1" });
  const panel = screen.getByRole("tabpanel", { name: "历史任务 1" });
  expect(selected).toHaveFocus();
  expect(selected).toHaveAttribute("aria-selected", "true");
  expect(selected).toHaveAttribute("aria-controls", panel.id);
  expect(within(panel).getByText("探活完成")).toBeVisible();
  expect(screen.getByRole("button", { name: "任务状态筛选" })).not.toHaveTextContent("排队中");
});

it("筛选没有匹配任务时说明筛选结果并允许直接清除筛选", async () => {
  renderPage();
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "任务状态筛选" }));
  await user.click(screen.getByRole("option", { name: "排队中" }));
  await user.keyboard("{Escape}");

  expect(screen.getByText("没有符合筛选条件的任务")).toBeVisible();
  expect(screen.queryByText("当前没有进行中的任务")).not.toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "清除筛选" }));
  expect(screen.getByText("正在探活")).toBeVisible();
});

it("长任务消息打开详情后完整换行展示，关闭后回到查看按钮", async () => {
  const message = "https://upstream.example.test/" + "request-detail".repeat(100);
  const operation = "external-operation-".repeat(120);
  renderPage([{ ...task, message, operation }]);
  const user = userEvent.setup();
  const openButton = screen.getByRole("button", { name: "查看任务" });
  await user.click(openButton);

  const dialog = screen.getByRole("dialog", { name: "任务详情" });
  expect(dialog).toHaveAccessibleDescription("查看任务状态与执行结果");
  expect(within(dialog).getByText(operation).closest("dd")).toBeInTheDocument();
  expect(within(dialog).getByText(message)).toHaveClass("wrap-anywhere", "whitespace-pre-wrap");
  expect(within(dialog).getByRole("progressbar", { name: "任务详情进度" })).toHaveAttribute(
    "aria-valuenow",
    "50",
  );
  await user.keyboard("{Escape}");
  expect(openButton).toHaveFocus();
});

it("后台刷新尚未完成时保留资源和任务内容并禁用刷新按钮", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(() => new Promise<Response>(() => {})),
  );
  renderPage();
  await userEvent.setup().click(screen.getByRole("button", { name: "刷新系统信息" }));

  expect(screen.getByRole("button", { name: "刷新系统信息" })).toBeDisabled();
  expect(screen.getByText("正在探活")).toBeVisible();
  expect(screen.getByText("4 个逻辑核心")).toBeVisible();
  expect(screen.queryByLabelText("正在加载任务")).not.toBeInTheDocument();
  expect(screen.queryAllByRole("status", { name: "正在读取资源占用" })).toHaveLength(0);
});
