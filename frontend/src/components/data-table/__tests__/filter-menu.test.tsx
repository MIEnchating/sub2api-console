import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { FilterMenu } from "../filter-menu";

beforeEach(() => {
  // JSDOM 26 recurses on :fullscreen; popups here never enter the top layer.
  const matches = Element.prototype.matches;
  vi.spyOn(Element.prototype, "matches").mockImplementation(function (
    this: Element,
    selector: string,
  ) {
    if ([":fullscreen", ":popover-open", ":modal"].includes(selector)) return false;
    return matches.call(this, selector);
  });
});
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

function FilterFixture() {
  const [value, setValue] = useState<string | null>(null);
  return (
    <FilterMenu
      label="分组"
      options={["默认", "Codex", "Claude"]}
      value={value}
      onValueChange={setValue}
    />
  );
}

describe("FilterMenu keyboard selection", () => {
  it("associates the focused search with its visible keyboard option", async () => {
    const user = userEvent.setup();
    render(<FilterFixture />);
    await user.click(screen.getByRole("button", { name: "分组筛选" }));
    const search = screen.getByRole("combobox", { name: "搜索分组" });
    expect(search).toHaveFocus();
    expect(search).toHaveAttribute("aria-controls", screen.getByRole("listbox").id);
    await user.keyboard("{End}");
    expect(search).toHaveAttribute(
      "aria-activedescendant",
      screen.getByRole("option", { name: "Claude" }).id,
    );
    await user.keyboard("{Enter}");
    expect(screen.getByRole("option", { name: "Claude" })).toHaveAttribute("aria-selected", "true");
    await user.keyboard("{Escape}");
    expect(screen.getByRole("button", { name: "分组筛选" })).toHaveFocus();
  });

  it("选项列表缩短后方向键从当前可见选项继续移动", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    const view = render(
      <FilterMenu
        label="分组"
        options={["A", "B", "C", "D", "E"]}
        value={null}
        onValueChange={onChange}
      />,
    );
    await user.click(screen.getByRole("button", { name: "分组筛选" }));
    await user.keyboard("{End}");
    view.rerender(
      <FilterMenu label="分组" options={["A", "B"]} value={null} onValueChange={onChange} />,
    );
    await user.keyboard("{ArrowDown}{Enter}");
    expect(onChange).toHaveBeenCalledWith("A");
  });

  it("does not select an option while confirming Chinese IME input", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(<FilterMenu label="分组" options={["默认"]} value={null} onValueChange={onChange} />);
    await user.click(screen.getByRole("button", { name: "分组筛选" }));
    fireEvent.keyDown(screen.getByLabelText("搜索分组"), { key: "Enter", isComposing: true });
    expect(onChange).not.toHaveBeenCalled();
  });

  it("clears the active option when search has no matches", async () => {
    const user = userEvent.setup();
    render(<FilterFixture />);
    await user.click(screen.getByRole("button", { name: "分组筛选" }));
    await user.type(screen.getByLabelText("搜索分组"), "missing");
    expect(screen.getByText("没有匹配项")).toBeVisible();
    expect(screen.getByLabelText("搜索分组")).not.toHaveAttribute("aria-activedescendant");
  });

  it("shows and clears a selected option whose stable value is an empty string", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(
      <FilterMenu
        label="分组"
        options={[""]}
        optionLabel={() => "未分组"}
        value=""
        onValueChange={onChange}
      />,
    );
    expect(screen.getByRole("button", { name: "分组筛选" })).toHaveTextContent("未分组");
    await user.click(screen.getByRole("button", { name: "分组筛选" }));
    await user.click(screen.getByRole("button", { name: "清除筛选" }));
    expect(onChange).toHaveBeenCalledWith(null);
  });
});
