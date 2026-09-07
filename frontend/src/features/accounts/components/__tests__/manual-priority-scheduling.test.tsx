import { fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { AccountStatus } from "@/api";
import { ManualPriorityDialog } from "../manual-priority-dialog";

const account = {
  id: "41",
  name: "人工账号",
  groups: ["codex"],
  manual_priority: 3,
  schedulable: false,
  load_factor: "100",
  concurrency: 100,
  manual_sync_balance_multiplier: false,
} as AccountStatus;

afterEach(() => vi.unstubAllGlobals());

describe("人工优先位调度控制", () => {
  it("账号原本停止调度时允许人工开启并随设置一起提交", () => {
    vi.stubGlobal("PointerEvent", MouseEvent);
    const onAssign = vi.fn();
    render(
      <ManualPriorityDialog
        open
        account={account}
        accounts={[account]}
        reservedMax={10}
        pending={false}
        onOpenChange={vi.fn()}
        onAssign={onAssign}
        onClear={vi.fn()}
      />,
    );

    const scheduling = screen.getByRole("switch", { name: "参与调度" });
    expect(scheduling).not.toBeChecked();
    expect(screen.getByRole("button", { name: "参与调度说明" })).toBeVisible();
    expect(screen.getByRole("button", { name: "同步上游余额说明" })).toBeVisible();
    expect(
      screen.queryByText("关闭后停止接收流量；人工优先位期间系统不会自动切换此开关。"),
    ).not.toBeInTheDocument();

    fireEvent.click(scheduling);
    fireEvent.click(screen.getByRole("button", { name: "更新人工优先位" }));

    expect(onAssign).toHaveBeenCalledWith({
      priority: 3,
      loadFactor: "100",
      concurrency: 100,
      schedulable: true,
      syncBalanceMultiplier: false,
    });
  });
});
