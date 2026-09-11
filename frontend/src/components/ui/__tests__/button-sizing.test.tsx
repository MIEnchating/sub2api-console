import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Pencil } from "lucide-react";
import { describe, expect, it, vi } from "vitest";

import { TableActionButton } from "@/components/data-table/table-action-button";
import { RefreshButton } from "@/components/refresh-button";
import { Button } from "../button";
import { SegmentedControl, SegmentedControlItem } from "../segmented-control";

describe("全局按钮尺寸", () => {
  it("刷新与表格操作使用相同的 32px 方形按钮", () => {
    render(
      <>
        <RefreshButton onClick={vi.fn()} />
        <TableActionButton label="编辑分组">
          <Pencil />
        </TableActionButton>
      </>,
    );

    for (const name of ["刷新", "编辑分组"]) {
      expect(screen.getByRole("button", { name })).toHaveClass("size-8", "rounded-lg", "text-sm");
    }
  });

  it("切换选中项时分段按钮与常规按钮保持相同高度和字号", async () => {
    const user = userEvent.setup();
    const onClick = vi.fn();
    render(
      <>
        <Button>保存</Button>
        <SegmentedControl>
          <SegmentedControlItem selected>当前分组</SegmentedControlItem>
          <SegmentedControlItem selected={false} onClick={onClick}>
            所有分组
          </SegmentedControlItem>
        </SegmentedControl>
      </>,
    );

    for (const name of ["保存", "当前分组", "所有分组"]) {
      expect(screen.getByRole("button", { name })).toHaveClass(
        "h-8",
        "text-sm",
        "gap-1.5",
        "px-2.5",
      );
    }
    expect(screen.getByRole("button", { name: "当前分组" })).toHaveAttribute(
      "aria-pressed",
      "true",
    );
    const next = screen.getByRole("button", { name: "所有分组" });
    expect(next).toHaveAttribute("aria-pressed", "false");
    next.focus();
    await user.keyboard("{Enter}");
    expect(onClick).toHaveBeenCalledOnce();
  });

  it("刷新进入加载状态时保留尺寸并禁止重复操作", async () => {
    const user = userEvent.setup();
    const onClick = vi.fn();
    const view = render(<RefreshButton onClick={onClick} />);
    view.rerender(<RefreshButton onClick={onClick} pending />);

    const button = screen.getByRole("button", { name: "刷新" });
    expect(button).toHaveClass("size-8");
    expect(button).toBeDisabled();
    await user.click(button);
    expect(onClick).not.toHaveBeenCalled();
  });
});
