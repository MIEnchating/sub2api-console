import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { AccountHealthScore } from "../account-health-score";

describe("AccountHealthScore", () => {
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
