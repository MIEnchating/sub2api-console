import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import type { AnimationResult } from "@/api";
import { AnimationAccountResult } from "../animation-account-result";

const clients: QueryClient[] = [];
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
});
const result: AnimationResult = {
  source_task_id: "current-task",
  source_task_status: "running",
  account_id: "41",
  account_name: "多模型账号",
  request_id: "r-a",
  model: "model-a",
  status: "succeeded",
  phase: "completed",
  svg: "<svg />",
  duration_ms: 100,
  completed_at: "2026-10-15T00:00:00Z",
};
for (const layout of ["card", "record"] as const) {
  it(`${layout} 布局在同批另一个模型仍生成时保留已生成模型的预览`, () => {
    const client = new QueryClient();
    clients.push(client);
    render(
      <QueryClientProvider client={client}>
        <AnimationAccountResult
          layout={layout}
          result={result}
          activity={{ taskID: "current-task", status: "generating" }}
          retryDisabled
          onRetry={() => {}}
        />
      </QueryClientProvider>,
    );
    expect(screen.getByRole("button", { name: "放大查看 多模型账号 的动画" })).toBeEnabled();
    expect(screen.getByText("model-a")).toBeVisible();
  });
  it(`${layout} 布局在新任务评审时不会把旧任务动画当成本轮结果`, () => {
    const client = new QueryClient();
    clients.push(client);
    render(
      <QueryClientProvider client={client}>
        <AnimationAccountResult
          layout={layout}
          result={result}
          activity={{ taskID: "new-task", status: "reviewing" }}
          retryDisabled
          onRetry={() => {}}
        />
      </QueryClientProvider>,
    );
    expect(
      screen.queryByRole("button", { name: "放大查看 多模型账号 的动画" }),
    ).not.toBeInTheDocument();
    expect(screen.getByRole("status", { name: "动画已生成，正在自动标注质量" })).toBeVisible();
  });
}
