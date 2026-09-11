import { render, screen } from "@testing-library/react";
import { expect, it } from "vitest";
import { AutoInspectionHeartbeatDetails } from "@/App";
import type { AutoInspectionStatus } from "@/api";

it.each(["主巡检", "独立倍率任务"])("%s记录尚未读取时显示轻量提示，不显示伪造的零进度", (kind) => {
  const record: AutoInspectionStatus["heartbeat_history"][number] = {
    checked_at: "2026-09-10T00:00:00Z",
    completed_at: null,
    status: "running",
    operations: kind === "主巡检" ? [] : ["account_rate_sync"],
    operation_timings: [],
    task_id: "task-1",
    error: null,
    skipped: false,
  };
  render(
    <AutoInspectionHeartbeatDetails
      record={record}
      taskLoading={kind === "主巡检"}
      accountRateSyncTaskLoading={kind !== "主巡检"}
    />,
  );
  const label = kind === "主巡检" ? "正在读取本轮任务队列" : "正在读取独立任务记录";
  const loading = screen.getByRole("status", { name: label });
  expect(loading).toHaveTextContent(label);
  expect(loading.querySelector('[data-slot="skeleton"]')).toBeNull();
  expect(screen.queryByRole("progressbar")).not.toBeInTheDocument();
});
