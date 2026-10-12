import { render, screen } from "@testing-library/react";
import { expect, it } from "vitest";
import { RunItems } from "../components/run-items";
import type { WorkbenchRun } from "../types";

it.each([
  ["completed", "已完成", "text-success"],
  ["failed", "失败", "text-destructive"],
  ["needs_attention", "需要处理", "text-warning"],
  ["running", "处理中", "text-info"],
  ["queued", "排队中", "text-muted-foreground"],
  ["future_status", "状态待确认", "text-muted-foreground"],
])("处理项状态为 %s 时显示 %s 及对应语义颜色", (status, label, color) => {
  const run: WorkbenchRun = {
    id: "run-1",
    revision: 1,
    task_id: "task-1",
    status: "completed",
    action: "import",
    created_at: "2026-10-12T00:00:00Z",
    updated_at: "2026-10-12T00:00:00Z",
    expires_at: "2026-10-12T02:00:00Z",
    duplicate_count: 0,
    items: [
      {
        id: "item-1",
        index: 0,
        kind: "login",
        name: "测试账号",
        email: "",
        plan: "",
        identity_source: "",
        status,
        message: "",
        template_name: "模板",
      },
    ],
  };
  render(<RunItems run={run} pending={false} onEnable={() => {}} />);
  expect(screen.getByText(label).closest('[data-slot="badge"]')).toHaveClass(color);
});
