import { fireEvent, render, screen, within } from "@testing-library/react";
import { expect, it } from "vitest";
import { GroupAllocationContent } from "../group-allocation-dialog";
import { allocationFixture } from "./allocation-fixture";

it("大分组只挂载当前页，翻页保留全组汇总并回到表格顶部", () => {
  render(<GroupAllocationContent allocation={allocationFixture(41)} />);
  const table = screen.getByRole("table");
  expect(within(table).getAllByRole("row")).toHaveLength(21);
  expect(screen.getByText("账号 6-1")).toBeVisible();
  expect(screen.queryByText("账号 6-21")).not.toBeInTheDocument();
  expect(screen.getByText("41 / 41")).toBeVisible();
  const scroll = table.parentElement!;
  scroll.scrollTop = 300;
  scroll.scrollLeft = 100;
  fireEvent.click(screen.getByRole("button", { name: "转到下一页" }));
  expect(screen.getByText("账号 6-21")).toBeVisible();
  expect(screen.queryByText("账号 6-1")).not.toBeInTheDocument();
  expect(screen.getByText("41 / 41")).toBeVisible();
  expect(scroll.scrollTop).toBe(0);
  expect(scroll.scrollLeft).toBe(100);
  fireEvent.click(screen.getByRole("button", { name: "转到下一页" }));
  expect(within(table).getAllByRole("row")).toHaveLength(2);
  expect(screen.getByRole("button", { name: "转到下一页" })).toBeDisabled();
});

it("同分组刷新保留当前页，数据缩减后收敛到有效页且清空后显示空状态", () => {
  const view = render(<GroupAllocationContent allocation={allocationFixture(41)} />);
  fireEvent.click(screen.getByRole("button", { name: "转到下一页" }));
  const scroll = screen.getByRole("table").parentElement!;
  scroll.scrollTop = 200;
  const refreshed = allocationFixture(41);
  refreshed.channels[20].account_name = "刷新后的账号";
  view.rerender(<GroupAllocationContent allocation={refreshed} />);
  expect(screen.getByText("刷新后的账号")).toBeVisible();
  expect(scroll.scrollTop).toBe(200);
  view.rerender(<GroupAllocationContent allocation={allocationFixture(1)} />);
  expect(screen.getByText("账号 6-1")).toBeVisible();
  expect(screen.getByRole("button", { name: "转到上一页" })).toBeDisabled();
  view.rerender(<GroupAllocationContent allocation={allocationFixture(0)} />);
  expect(screen.getByText(/该分组暂无账号/)).toBeVisible();
  expect(screen.queryByRole("navigation", { name: "表格分页" })).not.toBeInTheDocument();
});

it("切换稳定分组ID时重新从第一页展示", () => {
  const view = render(<GroupAllocationContent allocation={allocationFixture(41)} />);
  fireEvent.click(screen.getByRole("button", { name: "转到下一页" }));
  view.rerender(<GroupAllocationContent allocation={allocationFixture(41, "7")} />);
  expect(screen.getByText("账号 7-1")).toBeVisible();
  expect(screen.queryByText("账号 7-21")).not.toBeInTheDocument();
});

it("后续页有更高权重时按全组最大值绘制比例，刷新后同步更新", () => {
  const allocation = allocationFixture(40);
  allocation.channels[0].weight = 20;
  const view = render(<GroupAllocationContent allocation={allocation} />);
  const row = screen.getByRole("row", { name: /账号 6-1 ID/ });
  const weightBar = within(row).getByText("最终权重 20").previousElementSibling?.firstElementChild;
  expect(weightBar).toHaveStyle({ width: "50%" });
  const refreshed = {
    ...allocation,
    channels: allocation.channels.map((channel) => ({ ...channel })),
  };
  refreshed.channels[39].weight = 80;
  view.rerender(<GroupAllocationContent allocation={refreshed} />);
  expect(weightBar).toHaveStyle({ width: "25%" });
});
