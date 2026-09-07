import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { NumberRangeFilter } from "../number-range-filter";

function NumberRangeFilterHarness() {
  const [minimumValue, setMinimumValue] = useState("");
  const [maximumValue, setMaximumValue] = useState("");

  return (
    <NumberRangeFilter
      label="余额"
      minimumValue={minimumValue}
      maximumValue={maximumValue}
      onMinimumValueChange={setMinimumValue}
      onMaximumValueChange={setMaximumValue}
      min={0}
      step="any"
    />
  );
}

describe("NumberRangeFilter", () => {
  it("横向排列标签、两个固定宽度输入框和分隔文本", () => {
    const markup = renderToStaticMarkup(
      <NumberRangeFilter
        label="余额"
        minimumValue=""
        maximumValue="20"
        onMinimumValueChange={() => undefined}
        onMaximumValueChange={() => undefined}
      />,
    );

    expect(markup).toContain('data-slot="number-range-filter"');
    expect(markup).toContain('role="group"');
    expect(markup).toContain('aria-label="余额范围"');
    expect(markup.match(/w-28/g)).toHaveLength(2);
    expect(markup).toContain(">至</span>");
  });

  it("输入上下限时分别更新对应的受控值", async () => {
    const user = userEvent.setup();
    render(<NumberRangeFilterHarness />);

    const minimumInput = screen.getByRole("spinbutton", { name: "最低余额" });
    const maximumInput = screen.getByRole("spinbutton", { name: "最高余额" });
    await user.type(minimumInput, "5.5");
    await user.type(maximumInput, "20");

    expect(minimumInput).toHaveValue(5.5);
    expect(maximumInput).toHaveValue(20);
  });

  it("向两个输入框传递相同的数值边界和禁用状态", () => {
    render(
      <NumberRangeFilter
        label="次数"
        minimumValue="1"
        maximumValue="10"
        onMinimumValueChange={() => undefined}
        onMaximumValueChange={() => undefined}
        min={0}
        max={100}
        step={1}
        disabled
      />,
    );

    const inputs = screen.getAllByRole("spinbutton");
    for (const input of inputs) {
      expect(input).toHaveAttribute("min", "0");
      expect(input).toHaveAttribute("max", "100");
      expect(input).toHaveAttribute("step", "1");
      expect(input).toBeDisabled();
    }
  });
});
