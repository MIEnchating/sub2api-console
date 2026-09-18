import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it } from "vitest";
import type { UpstreamGroupChange } from "@/api";
import { UpstreamGroupHistory } from "../upstream-group-history";

afterEach(cleanup);

it("点击上游名称、统计数字及行空白均切换明细，点击明细内容不收起", async () => {
  render(
    <UpstreamGroupHistory
      upstreams={[{ upstream_id: "one", name: "上游甲", host: "a.example.test" }]}
      rows={[change(1, "one")]}
    />,
  );
  const user = userEvent.setup();
  const row = screen.getAllByRole("row")[1];
  await user.click(within(row).getByText("上游甲"));
  const details = screen.getByRole("region", { name: "上游甲 的变化明细" });
  expect(screen.getByRole("button", { name: "收起 上游甲 的变化明细" })).toHaveAttribute(
    "aria-expanded",
    "true",
  );
  await user.click(within(details).getByText("分组 1"));
  expect(details).toBeVisible();
  await user.click(within(row).getByRole("cell", { name: "新增 1 次" }));
  expect(screen.queryByRole("region", { name: "上游甲 的变化明细" })).not.toBeInTheDocument();
  await user.click(row);
  expect(screen.getByRole("region", { name: "上游甲 的变化明细" })).toBeVisible();
  await user.click(within(row).getByText("a.example.test"));
  expect(screen.queryByRole("region", { name: "上游甲 的变化明细" })).not.toBeInTheDocument();
});

it("点击箭头每次只切换一次，不被行点击重复触发", async () => {
  render(<UpstreamGroupHistory upstreams={[]} rows={[change(1, "one")]} />);
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "展开 one 的变化明细" }));
  expect(screen.getByRole("region", { name: "one 的变化明细" })).toBeVisible();
  await user.click(screen.getByRole("button", { name: "收起 one 的变化明细" }));
  expect(screen.queryByRole("region", { name: "one 的变化明细" })).not.toBeInTheDocument();
});

function change(
  id: number,
  upstreamID: string,
  type: "added" | "removed" = "added",
): UpstreamGroupChange {
  return {
    id,
    upstream_id: upstreamID,
    group_id: String(id),
    group_name: `分组 ${id}`,
    change_type: type,
    changed_at: `2026-09-16T${String(id % 24).padStart(2, "0")}:00:00Z`,
  };
}

it("同一稳定上游的交错记录聚合为一行，按最近变化排序并展开全部明细", async () => {
  render(
    <UpstreamGroupHistory
      upstreams={[
        { upstream_id: "one", name: "上游甲", host: "a.example.test" },
        { upstream_id: "two", name: "上游乙", host: "b.example.test" },
      ]}
      rows={[change(1, "one"), change(2, "two"), change(3, "one", "removed"), change(4, "one")]}
    />,
  );
  const table = screen.getByRole("table", { name: "按上游汇总分组变化" });
  const rows = within(table).getAllByRole("row");
  expect(rows).toHaveLength(3);
  expect(rows[1]).toHaveTextContent("上游甲");
  expect(within(rows[1]).getByRole("cell", { name: "新增 2 次" })).toBeVisible();
  expect(within(rows[1]).getByRole("cell", { name: "删除 1 次" })).toBeVisible();
  expect(screen.queryByText("分组 1")).not.toBeInTheDocument();
  const user = userEvent.setup();
  const expand = within(rows[1]).getByRole("button", { name: "展开 上游甲 的变化明细" });
  expand.focus();
  await user.keyboard("{Enter}");
  expect(expand).toHaveAttribute("aria-expanded", "true");
  const details = screen.getByRole("region", { name: "上游甲 的变化明细" });
  expect(
    within(details)
      .getAllByRole("listitem")
      .map((item) => item.textContent),
  ).toEqual([
    expect.stringContaining("分组 4"),
    expect.stringContaining("分组 3"),
    expect.stringContaining("分组 1"),
  ]);
  expect(within(details).queryByText("分组 2")).not.toBeInTheDocument();
  await user.keyboard(" ");
  expect(expand).toHaveAttribute("aria-expanded", "false");
  expect(screen.queryByRole("region", { name: "上游甲 的变化明细" })).not.toBeInTheDocument();
});

it("同名上游按稳定 ID 分开，已删除上游仍以 ID 展示历史", () => {
  render(
    <UpstreamGroupHistory
      upstreams={[
        { upstream_id: "one", name: "同名上游", host: "a.example.test" },
        { upstream_id: "two", name: "同名上游", host: "b.example.test" },
      ]}
      rows={[change(1, "one"), change(2, "two"), change(3, "deleted-upstream")]}
    />,
  );
  const table = screen.getByRole("table", { name: "按上游汇总分组变化" });
  expect(within(table).getAllByRole("row")).toHaveLength(4);
  expect(within(table).getAllByText("同名上游")).toHaveLength(2);
  expect(within(table).getByText("deleted-upstream")).toBeVisible();
});

it("超过一页的同一上游变化先聚合再分页，记录不拆散", async () => {
  render(
    <UpstreamGroupHistory
      upstreams={[]}
      rows={Array.from({ length: 21 }, (_, index) => change(index + 1, "one"))}
    />,
  );
  expect(screen.getAllByRole("row")).toHaveLength(2);
  expect(screen.getByRole("button", { name: "转到下一页" })).toBeDisabled();
  await userEvent.setup().click(screen.getByRole("button", { name: "展开 one 的变化明细" }));
  expect(screen.getAllByRole("listitem")).toHaveLength(21);
});

it("后台记录刷新后重新汇总数量，空数据保留明确空状态", async () => {
  const view = render(<UpstreamGroupHistory upstreams={[]} rows={[change(1, "one")]} />);
  await userEvent.setup().click(screen.getByRole("button", { name: "展开 one 的变化明细" }));
  view.rerender(
    <UpstreamGroupHistory upstreams={[]} rows={[change(1, "one"), change(2, "one", "removed")]} />,
  );
  expect(screen.getByRole("cell", { name: "新增 1 次" })).toBeVisible();
  expect(screen.getByRole("cell", { name: "删除 1 次" })).toBeVisible();
  expect(screen.getAllByRole("listitem")).toHaveLength(2);
  view.rerender(<UpstreamGroupHistory upstreams={[]} rows={[]} />);
  expect(screen.getByText("暂无上游分组变化记录")).toBeVisible();
  expect(screen.queryByRole("navigation", { name: "表格分页" })).not.toBeInTheDocument();
});

it("超过二十个上游时按上游翻页且总数不使用变化条数", async () => {
  const rows = Array.from({ length: 21 }, (_, index) => change(index + 1, `upstream-${index + 1}`));
  rows.push(change(22, "upstream-21"));
  render(<UpstreamGroupHistory upstreams={[]} rows={rows} />);
  const table = screen.getByRole("table", { name: "按上游汇总分组变化" });
  expect(within(table).getAllByRole("row")).toHaveLength(21);
  expect(screen.getByText("最近 22 条变化 · 21 个上游")).toBeVisible();
  await userEvent.setup().click(screen.getByRole("button", { name: "转到下一页" }));
  expect(within(table).getAllByRole("row")).toHaveLength(2);
  expect(within(table).getByText("upstream-1")).toBeVisible();
  expect(screen.getByRole("button", { name: "转到下一页" })).toBeDisabled();
});
