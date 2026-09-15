import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { UpstreamRecoverySelectionToolbar } from "../upstream-recovery-selection-toolbar";

describe("上游批量操作条", () => {
  it("恢复进行中禁止通过按钮和 Escape 清空选择", async () => {
    const onClear = vi.fn();
    const user = userEvent.setup();
    render(
      <UpstreamRecoverySelectionToolbar
        selectedCount={2}
        pending
        onClear={onClear}
        onRecover={vi.fn()}
      />,
    );
    const clear = screen.getByRole("button", { name: "清空选择" });
    expect(clear).toBeDisabled();
    screen.getByRole("toolbar").focus();
    await user.keyboard("{Escape}");
    expect(onClear).not.toHaveBeenCalled();
  });

  it("大量选择时工具条限制宽度并允许控件换行", () => {
    render(
      <UpstreamRecoverySelectionToolbar
        selectedCount={10000}
        pending={false}
        onClear={vi.fn()}
        onRecover={vi.fn()}
      />,
    );
    expect(screen.getByRole("toolbar")).toHaveClass("max-w-[calc(100%-2rem)]");
    expect(screen.getByRole("toolbar")).not.toHaveClass("hover:scale-105");
  });
});
