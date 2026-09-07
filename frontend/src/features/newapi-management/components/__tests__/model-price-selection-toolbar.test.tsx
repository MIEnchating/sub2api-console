import { fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { ModelPriceSelectionToolbar } from "../model-price-selection-toolbar";
import { NewAPIModelPrices } from "../model-prices";

beforeEach(() => vi.stubGlobal("PointerEvent", MouseEvent));
afterEach(() => vi.unstubAllGlobals());

describe("模型价格悬浮批量操作栏", () => {
  it("未选择时隐藏，勾选后显示底部居中操作栏，清空后消失", () => {
    render(
      <NewAPIModelPrices
        models={[{ model: "model-a", input_ratio: "1", completion_ratio: "4" }]}
        onLoadManagementPrices={async () => ({ models: [] })}
        onWriteModelPrices={vi.fn()}
      />,
    );
    expect(screen.queryByRole("toolbar", { name: /个模型的批量操作/ })).not.toBeInTheDocument();
    const filters = screen.getByLabelText("模型价格筛选与操作");
    expect(
      within(filters).queryByRole("button", { name: /批量同步|全选筛选结果/ }),
    ).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("checkbox", { name: "选择模型 model-a" }));
    const toolbar = screen.getByRole("toolbar", { name: "已选择 1 个模型的批量操作" });
    expect(toolbar).toHaveClass("fixed", "bottom-6", "left-1/2", "-translate-x-1/2");
    expect(toolbar).toHaveClass("max-w-[calc(100%-2rem)]");
    expect(within(toolbar).getByLabelText("1 个已选择模型")).toBeVisible();
    expect(within(toolbar).getByRole("button", { name: "批量同步（1）" })).toBeEnabled();

    fireEvent.click(within(toolbar).getByRole("button", { name: "清空选择" }));
    expect(screen.queryByRole("toolbar", { name: /个模型的批量操作/ })).not.toBeInTheDocument();
    expect(screen.getByRole("checkbox", { name: "选择模型 model-a" })).not.toBeChecked();
  });

  it("操作栏获得焦点后按 Esc 清空选择", async () => {
    const user = userEvent.setup();
    const clear = vi.fn();
    render(
      <ModelPriceSelectionToolbar
        selectedCount={2}
        pending={false}
        selectAllDisabled={false}
        onClear={clear}
        onSelectAll={vi.fn()}
        onSync={vi.fn()}
      />,
    );
    screen.getByRole("toolbar").focus();
    await user.keyboard("{Escape}");
    expect(clear).toHaveBeenCalledOnce();
  });

  it("批量操作忙碌时禁用全部按钮且 Esc 不清空选择", () => {
    const clear = vi.fn();
    render(
      <ModelPriceSelectionToolbar
        selectedCount={2}
        pending
        selectAllDisabled={false}
        onClear={clear}
        onSelectAll={vi.fn()}
        onSync={vi.fn()}
      />,
    );
    for (const button of screen.getAllByRole("button")) expect(button).toBeDisabled();
    fireEvent.keyDown(screen.getByRole("toolbar"), { key: "Escape" });
    expect(clear).not.toHaveBeenCalled();
  });

  it("超过批量上限时显示原因并禁用同步，仍可清空选择", () => {
    render(
      <ModelPriceSelectionToolbar
        selectedCount={1001}
        pending={false}
        selectAllDisabled={false}
        onClear={vi.fn()}
        onSelectAll={vi.fn()}
        onSync={vi.fn()}
      />,
    );
    expect(screen.getByRole("button", { name: "批量同步（1001）" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "批量同步（1001）" })).toHaveAccessibleDescription(
      "每批最多同步 1000 个模型",
    );
    expect(screen.getByRole("status")).toBeVisible();
    expect(screen.getByRole("button", { name: "清空选择" })).toBeEnabled();
  });

  it("筛选无结果时禁用全选，保留已选模型的同步入口", () => {
    render(
      <ModelPriceSelectionToolbar
        selectedCount={1}
        pending={false}
        selectAllDisabled
        onClear={vi.fn()}
        onSelectAll={vi.fn()}
        onSync={vi.fn()}
      />,
    );
    expect(screen.getByRole("button", { name: "全选筛选结果" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "批量同步（1）" })).toBeEnabled();
  });
});
