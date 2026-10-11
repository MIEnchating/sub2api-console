import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactElement } from "react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { FilterMenu } from "@/components/data-table/filter-menu";
import { MultiSelect } from "@/components/multi-select";
import { Select, SelectContent, SelectItem, SelectTrigger } from "../select";

const options = ["默认", "Codex", "Claude", "Gemini", "Kimi", "DeepSeek"];

beforeEach(() => {
  // JSDOM has no top layer or native select layout; use the browser boundary shim.
  const matches = Element.prototype.matches;
  vi.spyOn(Element.prototype, "matches").mockImplementation(function (
    this: Element,
    selector: string,
  ) {
    if ([":fullscreen", ":popover-open", ":modal"].includes(selector)) return false;
    return matches.call(this, selector);
  });
  const getComputedStyle = window.getComputedStyle;
  vi.spyOn(window, "getComputedStyle").mockImplementation((element, pseudoElement) => {
    if (element instanceof HTMLSelectElement) {
      const style = document.createElement("div").style;
      style.display = "none";
      return style;
    }
    return getComputedStyle(element, pseudoElement);
  });
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const cases: {
  name: string;
  triggerRole: "button" | "combobox";
  triggerName: string;
  searchName: string;
  render: () => ReactElement;
}[] = [
  {
    name: "单选菜单",
    triggerRole: "combobox",
    triggerName: "分组",
    searchName: "搜索选项",
    render: () => (
      <Select>
        <SelectTrigger aria-label="分组">选择分组</SelectTrigger>
        <SelectContent>
          {options.map((option) => (
            <SelectItem key={option} value={option}>
              {option}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    ),
  },
  {
    name: "多选菜单",
    triggerRole: "combobox",
    triggerName: "分组",
    searchName: "搜索分组",
    render: () => (
      <MultiSelect
        title="分组"
        options={options.map((value) => ({ value, label: value }))}
        selected={[]}
        onChange={() => undefined}
      />
    ),
  },
  {
    name: "筛选菜单",
    triggerRole: "button",
    triggerName: "分组筛选",
    searchName: "搜索分组",
    render: () => (
      <FilterMenu label="分组" options={options} value={null} onValueChange={() => undefined} />
    ),
  },
];

it.each(cases)("$name 打开后搜索框获得焦点，可直接输入且不增加焦点光环", async (scenario) => {
  const user = userEvent.setup();
  render(scenario.render());
  await user.click(screen.getByRole(scenario.triggerRole, { name: scenario.triggerName }));
  const search = screen.getByLabelText(scenario.searchName);
  expect(search).toHaveFocus();
  expect(search).toHaveClass("outline-none", "focus-visible:ring-0");
  await user.keyboard("Codex");
  expect(search).toHaveValue("Codex");
  expect(screen.getByRole("option", { name: "Codex" })).toBeVisible();
  expect(screen.queryByRole("option", { name: "Claude" })).not.toBeInTheDocument();
});
