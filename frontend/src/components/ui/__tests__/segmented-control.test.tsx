import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { describe, expect, it } from "vitest";

import { SegmentedControl, SegmentedControlItem } from "../segmented-control";

function TabsFixture() {
  const [selected, setSelected] = useState("details");
  return (
    <>
      <SegmentedControl role="tablist" aria-label="视图">
        {[
          ["details", "明细"],
          ["summary", "统计"],
          ["issues", "问题"],
        ].map(([id, label]) => (
          <SegmentedControlItem
            key={id}
            id={`tab-${id}`}
            role="tab"
            aria-controls={`panel-${id}`}
            selected={selected === id}
            onClick={() => setSelected(id)}
          >
            {label}
          </SegmentedControlItem>
        ))}
      </SegmentedControl>
      <div id={`panel-${selected}`} role="tabpanel" aria-labelledby={`tab-${selected}`}>
        {selected}
      </div>
    </>
  );
}

describe("SegmentedControl", () => {
  it("exposes one selected tab and its controlled panel", () => {
    render(<TabsFixture />);

    const details = screen.getByRole("tab", { name: "明细" });
    const summary = screen.getByRole("tab", { name: "统计" });
    expect(details).toHaveAttribute("aria-selected", "true");
    expect(details).not.toHaveAttribute("aria-pressed");
    expect(details).toHaveAttribute("tabindex", "0");
    expect(summary).toHaveAttribute("tabindex", "-1");
    expect(screen.getByRole("tabpanel")).toHaveAttribute("aria-labelledby", "tab-details");
  });

  it("supports arrow, Home, and End keyboard navigation with roving tabindex", async () => {
    const user = userEvent.setup();
    render(<TabsFixture />);
    const details = screen.getByRole("tab", { name: "明细" });
    details.focus();

    await user.keyboard("{ArrowRight}");
    expect(screen.getByRole("tab", { name: "统计" })).toHaveFocus();
    expect(screen.getByRole("tab", { name: "统计" })).toHaveAttribute("aria-selected", "true");
    await user.keyboard("{End}");
    expect(screen.getByRole("tab", { name: "问题" })).toHaveFocus();
    await user.keyboard("{Home}");
    expect(details).toHaveFocus();
    await user.keyboard("{ArrowLeft}");
    expect(screen.getByRole("tab", { name: "问题" })).toHaveFocus();
  });
});
