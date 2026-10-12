import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import type { Task } from "@/api";
import { TaskDetailsDialog } from "../task-details-dialog";

let client: QueryClient;
afterEach(() => {
  cleanup();
  client.clear();
});

it.each([
  "queued",
  "running",
  "waiting_input",
  "succeeded",
  "partial",
  "failed",
  "cancelled",
] as const)("%s 运营任务详情只显示一次摘要且保留账号明细", (status) => {
  const task: Task = {
    id: "automation",
    skill: "console",
    operation: "account-model-automation",
    status,
    progress: status === "queued" ? 0 : 3,
    message: "账号目录处理说明",
    result: {
      result_version: 2,
      dry_run: false,
      models: ["model-a"],
      selected_models: ["model-a"],
      items: [
        { account_id: "14", status: "succeeded", models: ["model-a"], message: "目录已保存" },
      ],
    },
    created_at: "2026-10-12T00:00:00Z",
    updated_at: "2026-10-12T00:00:00Z",
  };
  client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } });
  client.setQueryData(["task", task.id], task);
  render(
    <QueryClientProvider client={client}>
      <TaskDetailsDialog taskId={task.id} onClose={() => {}} />
    </QueryClientProvider>,
  );
  expect(screen.getByText(task.message)).toBeVisible();
  const results = screen.getByRole("region", { name: "运营执行结果" });
  expect(within(results).queryByText(task.message)).not.toBeInTheDocument();
  expect(within(results).getByText("账号 14 · 目录已刷新")).toBeVisible();
  expect(within(results).getByText("目录已保存")).toBeVisible();
  expect(within(results).getByText("读取到模型 1 个 · 选定上架 1 个")).toBeVisible();
});
