import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { BatchAuthTaskProgress, UpstreamSyncTaskStatus } from "../../App";
import type { Task } from "../../api";

function task(status: Task["status"], message: string, result: Task["result"] = {}): Task {
  return {
    id: "upstream-task",
    skill: "upstream",
    operation: "batch",
    status,
    progress: 100,
    message,
    result,
    created_at: "2026-09-07T12:00:00Z",
    updated_at: "2026-09-07T12:00:01Z",
  };
}

describe("上游批量任务终态", () => {
  it("批量鉴权在产生明细前失败时显示失败状态和原因", () => {
    render(<BatchAuthTaskProgress task={task("failed", "凭据读取失败，请检查密码箱配置")} />);

    expect(screen.getByText("批量鉴权恢复失败", { exact: true })).toBeVisible();
    expect(screen.getByText("凭据读取失败，请检查密码箱配置")).toBeVisible();
  });

  it("批量鉴权部分成功后取消时显示取消状态并保留已完成明细", () => {
    render(
      <BatchAuthTaskProgress
        task={task("cancelled", "已停止剩余上游的鉴权恢复", {
          summary: { hosts: 2, recovered: 1, failed: 0 },
          outcomes: [{ host: "completed.example.test", success: true, auth_method: "token" }],
        })}
      />,
    );

    expect(screen.getByText("批量鉴权恢复已取消", { exact: true })).toBeVisible();
    expect(screen.getByText("已停止剩余上游的鉴权恢复")).toBeVisible();
    expect(screen.getByText("completed.example.test")).toBeVisible();
  });

  it("上游同步在产生明细前取消时显示取消原因而非无目标空状态", () => {
    render(<UpstreamSyncTaskStatus task={task("cancelled", "已停止尚未执行的同步")} />);

    expect(screen.getByText("上游同步已取消", { exact: true })).toBeVisible();
    expect(screen.getByText("已停止尚未执行的同步")).toBeVisible();
    expect(screen.queryByText("当前没有需要同步的上游 Host")).not.toBeInTheDocument();
  });
});
