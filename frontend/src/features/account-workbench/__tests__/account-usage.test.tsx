import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { AccountUsageCell } from "../components/account-usage-cell";

afterEach(cleanup);

it("用量为零与部分使用时分别显示实际百分比和窗口重置时间", () => {
  render(
    <AccountUsageCell
      usage={{
        five_hour: { used_percent: 0, reset_at: "2099-01-01T00:00:00Z" },
        seven_day: { used_percent: 62.5 },
        updated_at: "2026-10-12T00:00:00Z",
      }}
    />,
  );
  expect(screen.getByRole("meter", { name: "5h 已用" })).toHaveAttribute("aria-valuenow", "0");
  expect(screen.getByRole("meter", { name: "7d 已用" })).toHaveAttribute("aria-valuenow", "62.5");
  expect(screen.getByText("0%")).toBeVisible();
  expect(screen.getByText("62.5%")).toBeVisible();
  expect(screen.getByText(/重置：/)).toBeVisible();
  expect(screen.getByText(/更新于/)).toBeVisible();
});

it("缺失用量不补零且保留两个窗口名称", () => {
  render(<AccountUsageCell />);
  expect(screen.getByText("5h")).toBeVisible();
  expect(screen.getByText("7d")).toBeVisible();
  expect(screen.getAllByText("暂无数据")).toHaveLength(2);
  expect(screen.queryByRole("meter")).not.toBeInTheDocument();
});

it("窗口已过重置时间时显示待刷新，保留上次用量且不伪造归零", () => {
  render(
    <AccountUsageCell
      usage={{ five_hour: { used_percent: 100, reset_at: "2000-01-01T00:00:00Z" } }}
    />,
  );
  expect(screen.getByText("待刷新（上次 100%）")).toBeVisible();
  expect(screen.queryByRole("meter")).not.toBeInTheDocument();
});
