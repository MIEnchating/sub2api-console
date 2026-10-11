import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
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

describe("手动控制调度控制", () => {
  it.each([0, 1, 2])(
    "当前账号与 %s 个同组账号共用位置时，标记当前占用并显示包含自身的人数",
    async (peers) => {
      const user = userEvent.setup();
      render(
        <ManualPriorityDialog
          open
          account={account}
          accounts={[
            account,
            ...Array.from({ length: peers }, (_, index) => ({
              ...account,
              id: String(42 + index),
              name: `同组账号 ${index + 1}`,
            })),
          ]}
          reservedMax={10}
          pending={false}
          onOpenChange={vi.fn()}
          onAssign={vi.fn()}
          onClear={vi.fn()}
        />,
      );
      await user.click(screen.getByRole("combobox", { name: "选择手动控制位置" }));
      const option = await screen.findByRole("option", { name: /^3 ·/ });
      expect(option).toHaveTextContent(`当前账号使用（${peers + 1}/3）`);
      expect(option).toHaveAttribute("aria-selected", "true");
      expect(option).not.toHaveAttribute("aria-disabled", "true");
      expect(screen.getByRole("option", { name: /^4 ·/ })).toHaveTextContent("可用");
    },
  );

  it.each([
    { reason: "任务处理中", pending: true, priority: 3 },
    { reason: "优先级低于下限", pending: false, priority: 0 },
    { reason: "优先级高于上限", pending: false, priority: 11 },
  ])("$reason 时禁止提交手动控制", (scenario) => {
    render(
      <ManualPriorityDialog
        open
        account={{ ...account, manual_priority: scenario.priority }}
        accounts={[account]}
        reservedMax={10}
        pending={scenario.pending}
        onOpenChange={vi.fn()}
        onAssign={vi.fn()}
        onClear={vi.fn()}
      />,
    );
    expect(screen.getByRole("button", { name: "更新手动控制" })).toBeDisabled();
  });

  it("同组其他账号使用相同优先级时仍可选择并重复提交", async () => {
    const user = userEvent.setup();
    const onAssign = vi.fn();
    render(
      <ManualPriorityDialog
        open
        account={account}
        accounts={[account, { ...account, id: "42", name: "同组账号" }]}
        reservedMax={10}
        pending={false}
        onOpenChange={vi.fn()}
        onAssign={onAssign}
        onClear={vi.fn()}
      />,
    );
    const selector = screen.getByRole("combobox", { name: "选择手动控制位置" });
    expect(screen.getByRole("button", { name: "更新手动控制" })).toBeEnabled();
    await user.click(selector);
    const option = await screen.findByRole("option", { name: /^3 ·/ });
    expect(option).not.toHaveAttribute("aria-disabled", "true");
    await user.click(option);
    await user.click(screen.getByRole("button", { name: "更新手动控制" }));
    expect(onAssign).toHaveBeenCalledWith({
      priority: 3,
      loadFactor: "100",
      concurrency: 100,
      schedulable: false,
      syncBalanceMultiplier: false,
    });
  });

  it("同组已有三个账号占用时禁用该位置，其他位置仍可选", async () => {
    const user = userEvent.setup();
    const onAssign = vi.fn();
    render(
      <ManualPriorityDialog
        open
        account={{ ...account, manual_priority: null }}
        accounts={[
          { ...account, manual_priority: null },
          ...["42", "43", "44"].map((id) => ({ ...account, id, name: `同组账号 ${id}` })),
        ]}
        reservedMax={10}
        pending={false}
        onOpenChange={vi.fn()}
        onAssign={onAssign}
        onClear={vi.fn()}
      />,
    );

    await user.click(screen.getByRole("combobox", { name: "选择手动控制位置" }));
    expect(await screen.findByRole("option", { name: /3 .*已满/ })).toHaveAttribute(
      "aria-disabled",
      "true",
    );
    await user.click(screen.getByRole("option", { name: /4 .*可用/ }));
    await user.click(screen.getByRole("button", { name: "设置手动控制" }));
    expect(onAssign).toHaveBeenCalledWith(expect.objectContaining({ priority: 4 }));
  });

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
      screen.queryByText("关闭后停止接收流量；手动控制期间系统不会自动切换此开关。"),
    ).not.toBeInTheDocument();

    fireEvent.click(scheduling);
    fireEvent.click(screen.getByRole("button", { name: "更新手动控制" }));

    expect(onAssign).toHaveBeenCalledWith({
      priority: 3,
      loadFactor: "100",
      concurrency: 100,
      schedulable: true,
      syncBalanceMultiplier: false,
    });
  });
});
