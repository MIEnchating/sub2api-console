import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { SortableItem, SortableList } from "../../sortable-list";

function List(props: { disabled?: boolean }) {
  return (
    <SortableList items={[{ id: "openai", label: "OpenAI" }]} onMove={() => undefined}>
      <SortableItem id="openai" label="OpenAI" disabled={props.disabled}>
        {(item) => (
          <div ref={item.ref} style={item.style}>
            {item.handle}
            <span>OpenAI</span>
          </div>
        )}
      </SortableItem>
    </SortableList>
  );
}

describe("排序拖动手柄", () => {
  it("键盘聚焦手柄时显示共享提示，空格启动拖动且 Escape 取消", async () => {
    const user = userEvent.setup();
    render(<List />);

    await user.tab();
    const handle = screen.getByRole("button", { name: "拖动OpenAI" });
    expect(handle).toHaveFocus();
    expect(await screen.findByText("拖动OpenAI")).toBeVisible();
    expect(handle).not.toHaveAttribute("title");

    await user.keyboard(" ");
    expect(handle).toHaveAttribute("aria-pressed", "true");
    await user.keyboard("{Escape}");
    expect(handle).toHaveAttribute("aria-pressed", "false");
  });

  it("禁用手柄时不进入键盘焦点顺序且保持未拖动状态", async () => {
    render(<List disabled />);
    const handle = screen.getByRole("button", { name: "拖动OpenAI" });
    expect(handle).toBeDisabled();
    await userEvent.setup().tab();
    expect(handle).not.toHaveFocus();
    expect(handle).toHaveAttribute("aria-pressed", "false");
  });
});
