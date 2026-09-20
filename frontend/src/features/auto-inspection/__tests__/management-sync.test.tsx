import { render, screen } from "@testing-library/react";
import { expect, it } from "vitest";

import { AutoInspectionHeartbeatDetails } from "@/App";
import type { AutoInspectionStatus, Task } from "@/api";

it("心跳完成管理端同步后显示中文步骤与同步账号分组数量", () => {
  const record: AutoInspectionStatus["heartbeat_history"][number] = {
    checked_at: "2026-09-20T07:00:00Z",
    completed_at: "2026-09-20T07:00:01Z",
    status: "succeeded",
    operations: ["management_sync"],
    task_id: "heartbeat-1",
    operation_timings: [{ operation: "management_sync", duration_seconds: 1 }],
    error: null,
    skipped: false,
  };
  const task: Task = {
    id: "heartbeat-1",
    skill: "console",
    operation: "auto-inspection",
    status: "succeeded",
    progress: 100,
    message: "巡检完成",
    created_at: record.checked_at,
    updated_at: record.completed_at!,
    result: {
      management_sync: { accounts: 3, groups: 2 },
      completed_operations: ["management_sync"],
    },
  };
  render(<AutoInspectionHeartbeatDetails record={record} task={task} />);
  expect(screen.getByText("已同步 3 个账号、2 个分组")).toBeVisible();
  expect(screen.getAllByText("管理端账号与分组同步").length).toBeGreaterThan(0);
});
