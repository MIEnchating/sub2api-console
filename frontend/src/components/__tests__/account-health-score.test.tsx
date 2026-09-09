import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { AccountHealthScore } from "../account-health-score";

describe("AccountHealthScore", () => {
  it.each([0, 2, 10000])("有效样本为 %i 时短长期标签与分数保持紧凑间距", (sampleCount) => {
    render(
      <AccountHealthScore score={98} shortScore={98} longScore={96} sampleCount={sampleCount} />,
    );

    for (const label of [/短期评分/, /长期评分/]) {
      const row = screen.getByLabelText(label);
      expect(row).toHaveClass("flex", "gap-2");
      expect(row).not.toHaveClass("justify-between");
      expect(row).not.toHaveClass("min-w-16");
    }
  });

  it("有有效样本时同时展示综合分、短长期分和样本数", () => {
    render(<AccountHealthScore score={72.5} shortScore={62.4} longScore={90.5} sampleCount={2} />);
    expect(screen.getByLabelText("健康分 73")).toBeVisible();
    expect(screen.getByLabelText("健康分 73")).toHaveTextContent("73");
    expect(screen.getByLabelText("短期评分 62")).toBeVisible();
    expect(screen.getByLabelText("长期评分 91")).toBeVisible();
    expect(screen.getByText("有效样本 2")).toBeVisible();
  });

  it("没有有效样本时显示零条样本并隐藏残留评分", () => {
    render(<AccountHealthScore score={72.5} shortScore={68} longScore={83} sampleCount={0} />);
    expect(screen.getByLabelText("暂无健康分")).toBeVisible();
    expect(screen.getByLabelText("短期评分 —")).toBeVisible();
    expect(screen.getByLabelText("长期评分 —")).toBeVisible();
    expect(screen.getByText("有效样本 0")).toBeVisible();
    expect(screen.queryByLabelText("健康分 73")).not.toBeInTheDocument();
  });
});
