import { render, screen, cleanup } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { MultiSelect } from "../../multi-select";

beforeEach(() => {
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

it("选择器打开后变为禁用时不能通过清空按钮修改选中值", async () => {
  const user = userEvent.setup();
  const onChange = vi.fn();
  const props = {
    options: [{ value: "one", label: "一组" }],
    selected: ["one"],
    onChange,
    title: "分组",
  };
  const view = render(<MultiSelect {...props} />);
  await user.click(screen.getByRole("combobox", { name: "分组" }));
  expect(screen.getByRole("button", { name: "清空筛选" })).toBeVisible();
  view.rerender(<MultiSelect {...props} disabled />);
  const clear = screen.queryByRole("button", { name: "清空筛选" });
  if (clear) await user.click(clear);
  expect(onChange).not.toHaveBeenCalled();
  expect(screen.getByRole("combobox", { name: "分组" })).toHaveAttribute("aria-disabled", "true");
});
