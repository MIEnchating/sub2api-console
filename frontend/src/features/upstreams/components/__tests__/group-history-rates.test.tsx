import { act, cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it } from "vitest";
import type { UpstreamGroupChange } from "@/api";
import { UpstreamGroupHistory } from "../upstream-group-history";

afterEach(cleanup);

const changes: UpstreamGroupChange[] = [
  {
    id: 1,
    upstream_id: "one",
    group_id: "7",
    group_name: "标准组",
    change_type: "added",
    changed_at: "2026-09-18T01:00:00Z",
    effective_rate: "0.1234567890123456789",
  },
  {
    id: 2,
    upstream_id: "one",
    group_id: "8",
    group_name: "免费组",
    change_type: "added",
    changed_at: "2026-09-18T02:00:00Z",
    effective_rate: "0",
  },
  {
    id: 3,
    upstream_id: "one",
    group_id: "9",
    group_name: "缺失倍率组",
    change_type: "added",
    changed_at: "2026-09-18T03:00:00Z",
    effective_rate: null,
  },
  {
    id: 4,
    upstream_id: "one",
    group_id: "10",
    group_name: "已删除组",
    change_type: "removed",
    changed_at: "2026-09-18T04:00:00Z",
    effective_rate: "9.99",
  },
];

it("展开新增分组变化时显示换算后倍率，保留精度、零值及缺失提示", async () => {
  render(<UpstreamGroupHistory upstreams={[]} rows={changes.slice(0, 3)} />);
  await userEvent.setup().click(screen.getByRole("button", { name: "展开 one 的变化明细" }));
  const items = screen.getAllByRole("listitem");
  expect(items[0]).toHaveTextContent("缺失倍率组");
  expect(items[0]).toHaveTextContent("账号成本（已换算）：未计算");
  expect(items[1]).toHaveTextContent("免费组");
  expect(items[1]).toHaveTextContent("账号成本（已换算）：0");
  expect(items[2]).toHaveTextContent("标准组");
  expect(items[2]).toHaveTextContent("账号成本（已换算）：0.1234567890123456789");
  await userEvent.keyboard("{Tab}");
  act(() => screen.getByText("账号成本（已换算）：0.1234567890123456789").focus());
  expect(await screen.findByRole("tooltip")).toHaveTextContent(
    "最近同步并按充值比例换算后的分组倍率",
  );
});

it("展开删除分组变化时保留记录但不显示倍率或缺失提示", async () => {
  render(<UpstreamGroupHistory upstreams={[]} rows={[changes[3]]} />);
  await userEvent.setup().click(screen.getByRole("button", { name: "展开 one 的变化明细" }));
  const item = screen.getByRole("listitem");
  expect(item).toHaveTextContent("已删除组");
  expect(item).not.toHaveTextContent("账号成本");
  expect(item).not.toHaveTextContent("倍率");
  expect(item).not.toHaveTextContent("9.99");
  expect(item).not.toHaveTextContent("未计算");
  expect(item).not.toHaveTextContent("未读取");
});

it("单个上游历史显示新增分组换算后倍率且删除记录的倍率单元格留空", () => {
  render(<UpstreamGroupHistory rows={changes} />);
  expect(screen.getByRole("columnheader", { name: "账号成本（已换算）" })).toBeVisible();
  const rows = screen.getAllByRole("row");
  expect(within(rows[1]).getByRole("cell", { name: "0.1234567890123456789" })).toBeVisible();
  expect(within(rows[2]).getByRole("cell", { name: "0" })).toBeVisible();
  expect(within(rows[3]).getByRole("cell", { name: "未计算" })).toBeVisible();
  expect(within(rows[4]).getAllByRole("cell")[3]).toBeEmptyDOMElement();
});
