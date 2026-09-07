import { render, screen, within } from "@testing-library/react";
import { expect, it } from "vitest";

import { AutoInspectionHeartbeatDetails } from "../../../App";
import type { AutoInspectionStatus, Task } from "../../../api";

it("巡检已取消时展示中性取消状态和取消原因", () => {
  const record: AutoInspectionStatus["heartbeat_history"][number] = {
    checked_at: "2026-09-07T12:00:00Z",
    completed_at: "2026-09-07T12:00:02Z",
    status: "cancelled",
    operations: [],
    operation_timings: [],
    task_id: "cancelled-inspection",
    error: null,
    skipped: false,
  };
  const task: Task = {
    id: "cancelled-inspection",
    skill: "sub2api-auto-inspection",
    operation: "automatic-inspection",
    status: "cancelled",
    progress: 40,
    message: "操作员取消巡检",
    result: { cancel_reason: "操作员取消巡检" },
    created_at: record.checked_at,
    updated_at: record.completed_at!,
  };

  render(<AutoInspectionHeartbeatDetails record={record} task={task} />);

  expect(
    within(screen.getByRole("region", { name: "巡检概况" })).getByText("已取消"),
  ).toBeVisible();
  expect(screen.getByText("操作员取消巡检")).toBeVisible();
  expect(screen.queryByText("失败")).not.toBeInTheDocument();
});
