import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { expect, it } from "vitest";

import type { Task, UnifiedLogEntry } from "@/api";
import { LogDetailsDialog, LogDetailsContent } from "../log-details-dialog";

it("任务已失败而列表摘要仍在运行时详情展示最新失败原因与进度", () => {
  const entry: UnifiedLogEntry = {
    id: "log-1",
    kind: "task",
    occurred_at: "2026-09-01T00:00:00Z",
    title: "active-probe",
    summary: "准备探测",
    status: "running",
    actor: null,
    object_label: null,
    source: "task",
    source_id: "task-1",
    related_count: 0,
    details: { progress: 10 },
  };
  const task: Task = {
    id: "task-1",
    skill: "console",
    operation: "active-probe",
    status: "failed",
    progress: 75,
    message: "上游凭据已过期，请恢复鉴权后重试",
    result: {},
    created_at: entry.occurred_at,
    updated_at: "2026-09-01T00:00:10Z",
  };
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  });
  client.setQueryData(["task", task.id], task);
  render(
    <QueryClientProvider client={client}>
      <LogDetailsDialog entry={entry} onClose={() => undefined} />
    </QueryClientProvider>,
  );

  expect(screen.getByText(task.message)).toBeInTheDocument();
  expect(screen.getByText("失败")).toBeInTheDocument();
  expect(screen.getByText("75%")).toBeInTheDocument();
  expect(screen.queryByText("执行中")).not.toBeInTheDocument();
});

it("日志完整结果读取中保留列表摘要并显示紧凑忙碌提示", () => {
  render(
    <LogDetailsContent
      entry={{
        id: "log-loading",
        kind: "task",
        occurred_at: "2026-09-11T00:00:00Z",
        title: "active-probe",
        summary: "正在验证账号",
        status: "running",
        actor: null,
        object_label: null,
        source: "task",
        source_id: "task-loading",
        related_count: 0,
        details: {},
      }}
      loading
    />,
  );
  expect(screen.getByText("正在验证账号")).toBeVisible();
  expect(screen.getByRole("status", { name: "正在读取完整任务结果…" })).toHaveAttribute(
    "aria-busy",
    "true",
  );
});
