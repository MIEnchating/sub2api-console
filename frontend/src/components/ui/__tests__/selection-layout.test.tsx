import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { MultiSelect } from "@/components/multi-select";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "../select";
import userEvent from "@testing-library/user-event";

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
  // The hidden native select has no layout; JSDOM's UA picker selectors recurse.
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

describe("selection popup layout", () => {
  it("keeps long single-select labels inside the available popup width", () => {
    render(
      <Select open value="account">
        <SelectTrigger aria-label="账号">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="account">{"account-".repeat(40)}</SelectItem>
        </SelectContent>
      </Select>,
    );

    const option = screen.getByRole("option");
    expect(option.querySelector('[data-slot="select-item-text"]')).toHaveClass(
      "min-w-0",
      "whitespace-normal",
      "[overflow-wrap:anywhere]",
    );
    expect(document.querySelector('[data-slot="select-content"]')).toHaveClass(
      "max-w-(--available-width)",
    );
  });

  it("allows the multi-select list to shrink when the popup height is constrained", async () => {
    const user = userEvent.setup();
    render(
      <MultiSelect
        title="分组"
        options={[{ value: "codex", label: "Codex" }]}
        selected={[]}
        onChange={() => undefined}
      />,
    );
    await user.click(screen.getByRole("combobox", { name: "分组" }));

    expect(document.querySelector('[data-slot="combobox-content"]')).toHaveClass(
      "flex",
      "flex-col",
    );
    expect(screen.getByRole("listbox")).toHaveClass("min-h-0", "overflow-y-auto");
  });
});
