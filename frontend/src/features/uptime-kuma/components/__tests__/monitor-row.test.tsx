import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { MonitorTreeRow } from "../monitor-tree-row";
import { monitor } from "./fixtures";

describe("监控行按钮", () => {
  it("长名称保持单行截断，详情按钮仍可通过键盘打开", async () => {
    const user = userEvent.setup();
    const name = "用于校验按钮尺寸的长监控名称".repeat(4);
    const onDetail = vi.fn();
    render(
      <>
        <table>
          <tbody>
            <MonitorTreeRow
              row={{
                monitor: { ...monitor, name },
                ancestors: [],
                hasChildren: false,
                parentLabel: "未分组",
              }}
              expanded={false}
              filtering={false}
              management={false}
              disabled={false}
              onToggle={vi.fn()}
              onDetail={onDetail}
              onEdit={vi.fn()}
              onAction={vi.fn()}
            />
          </tbody>
        </table>
      </>,
    );

    const detail = screen.getByRole("button", { name: `查看 ${name} 详情` });
    expect(detail).toHaveClass("h-8", "min-w-0");
    expect(screen.getByText(name)).toHaveClass("truncate");
    await user.tab();
    expect(detail).toHaveFocus();
    await user.keyboard("{Enter}");
    expect(onDetail).toHaveBeenCalledWith(monitor.key);
  });

  it("筛选中的分组展开按钮保留统一尺寸并禁止操作", async () => {
    const user = userEvent.setup();
    const onToggle = vi.fn();
    render(
      <>
        <table>
          <tbody>
            <MonitorTreeRow
              row={{
                monitor: { ...monitor, type: "group" },
                ancestors: [],
                hasChildren: true,
                parentLabel: "未分组",
              }}
              expanded
              filtering
              management={false}
              disabled={false}
              onToggle={onToggle}
              onDetail={vi.fn()}
              onEdit={vi.fn()}
              onAction={vi.fn()}
            />
          </tbody>
        </table>
      </>,
    );

    const toggle = screen.getByRole("button", { name: `收起分组 ${monitor.name}` });
    expect(toggle).toHaveClass("size-8");
    expect(toggle).toBeDisabled();
    expect(toggle).toHaveAttribute("aria-expanded", "true");
    await user.click(toggle);
    expect(onToggle).not.toHaveBeenCalled();
  });
});
