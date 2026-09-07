import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import type { AccountStatus } from "@/api";
import { AccountLatencyCell } from "../account-pool-cells";

const account: Pick<AccountStatus, "ttfb_p50_ms" | "ttfb_p95_ms"> = {
  ttfb_p50_ms: 320,
  ttfb_p95_ms: 1250,
};

describe("账号流量首字延迟", () => {
  it("有亚秒和秒级延迟时保留精度，避免较快请求显示为零秒", () => {
    render(<AccountLatencyCell account={account} />);
    expect(screen.getByText("320ms")).toBeVisible();
    expect(screen.getByText("1.25s")).toBeVisible();
    expect(screen.queryByText(/0s/)).not.toBeInTheDocument();
  });

  it("没有统计值时显示缺失提示，并提供可聚焦的说明入口", () => {
    render(<AccountLatencyCell account={{ ...account, ttfb_p50_ms: null, ttfb_p95_ms: null }} />);
    expect(screen.getByText("暂无首字数据")).toBeVisible();
    expect(screen.getByLabelText("真实流量首字延迟说明")).toHaveAttribute("tabindex", "0");
  });

  it("仅一个分位值可用时保留该值，缺失项不补零", () => {
    render(<AccountLatencyCell account={{ ...account, ttfb_p50_ms: null }} />);
    expect(screen.getByText("1.25s")).toBeVisible();
    expect(screen.getByText("—")).toBeVisible();
    expect(screen.queryByText("暂无首字数据")).not.toBeInTheDocument();
  });

  it("非有限或非正延迟不显示为有效测量值", () => {
    render(
      <AccountLatencyCell account={{ ...account, ttfb_p50_ms: 0, ttfb_p95_ms: Number.NaN }} />,
    );
    expect(screen.getByText("暂无首字数据")).toBeVisible();
    expect(screen.getAllByText("—")).toHaveLength(2);
  });
});
