import { render, screen } from "@testing-library/react";
import { expect, it } from "vitest";

import { AccountBatchDeleteTaskStatus } from "@/App";
import type { Task } from "@/api";

function task(status: Task["status"], result: Task["result"], message: string): Task {
  return {
    id: "batch-delete",
    operation: "account-delete-batch",
    skill: "accounts",
    status,
    result,
    message,
    progress: 100,
    created_at: "2026-09-07T00:00:00Z",
    updated_at: "2026-09-07T00:00:00Z",
  };
}

it("批量删除在生成明细前失败时展示后端失败原因", () => {
  render(
    <AccountBatchDeleteTaskStatus
      task={task("failed", { error: "管理目标已变化，请重新预览" }, "批量删除失败")}
    />,
  );
  expect(screen.getByText("管理目标已变化，请重新预览")).toBeVisible();
});

it("批量删除取消时显示取消原因并保留已完成明细", () => {
  render(
    <AccountBatchDeleteTaskStatus
      task={task(
        "cancelled",
        { items: [{ account_id: "41", account_name: "已清理账号", status: "succeeded" }] },
        "批量删除已由用户取消",
      )}
    />,
  );
  expect(screen.getByText("批量删除已由用户取消")).toBeVisible();
  expect(screen.getByText("已清理账号")).toBeVisible();
});
