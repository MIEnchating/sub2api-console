import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";

import type { Task } from "@/api";
import { Table, TableBody } from "@/components/ui/table";
import { TaskDetailsDialog } from "../task-details-dialog";
import { TaskRow } from "../task-row";

const clients: QueryClient[] = [];
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
});

const task: Task = {
  id: "waiting-probe",
  skill: "console",
  operation: "active-probe",
  status: "queued",
  progress: 0,
  message: "正在等待探活执行",
  result: {},
  created_at: "2026-10-09T00:00:00Z",
  updated_at: "2026-10-09T00:00:00Z",
};

it.each(["queued", "running"] as const)(
  "%s 普通任务尚无实际进度时列表显示等待且不伪造百分比",
  (status) => {
    render(
      <Table>
        <TableBody>
          <TaskRow task={{ ...task, status, system_info: true }} onSelect={() => {}} />
        </TableBody>
      </Table>,
    );
    expect(screen.queryByRole("progressbar")).not.toBeInTheDocument();
    expect(screen.queryByText("0%")).not.toBeInTheDocument();
    expect(screen.getByRole("status", { name: "等待任务进度" })).toBeVisible();
    expect(screen.getByText(task.message)).toBeVisible();
  },
);

it("任务详情从启动等待变为实际进度时更新展示且保留后端消息", async () => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  });
  clients.push(client);
  client.setQueryData(["task", task.id], task);
  render(
    <QueryClientProvider client={client}>
      <TaskDetailsDialog taskId={task.id} onClose={() => {}} />
    </QueryClientProvider>,
  );
  expect(screen.queryByRole("progressbar")).not.toBeInTheDocument();
  expect(screen.getByRole("status", { name: task.message })).toBeVisible();
  await act(async () =>
    client.setQueryData(["task", task.id], {
      ...task,
      status: "running",
      progress: 40,
      message: "已完成 2/5 项",
    }),
  );
  expect(await screen.findByRole("progressbar", { name: "任务详情进度" })).toHaveAttribute(
    "aria-valuenow",
    "40",
  );
  expect(screen.getByText("已完成 2/5 项")).toBeVisible();
  expect(screen.queryByRole("status", { name: task.message })).not.toBeInTheDocument();
});

it("任务未开始即失败时展示失败原因并停止启动等待", () => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  });
  clients.push(client);
  client.setQueryData(["task", task.id], {
    ...task,
    status: "failed",
    message: "读取目标配置失败",
  });
  render(
    <QueryClientProvider client={client}>
      <TaskDetailsDialog taskId={task.id} onClose={() => {}} />
    </QueryClientProvider>,
  );
  expect(screen.getByText("已失败")).toBeVisible();
  expect(screen.getByText("读取目标配置失败")).toBeVisible();
  expect(screen.queryByRole("status", { name: "读取目标配置失败" })).not.toBeInTheDocument();
});
