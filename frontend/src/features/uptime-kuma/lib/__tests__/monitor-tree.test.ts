import { describe, expect, it } from "vitest";
import { buildMonitorTree, filterMonitorTree } from "../monitor-tree";
import { monitor } from "../../components/__tests__/fixtures";

const root = { ...monitor, id: 1, key: "id:1", name: "openai", type: "group" };
const nested = { ...root, id: 2, key: "id:2", name: "生产", parent: 1 };
const child = { ...monitor, id: 3, key: "id:3", name: "codex", parent: 2, status: 0 };
const options = {
  search: "",
  status: "all",
  group: "all",
  management: true,
  collapsed: new Set<string>(),
};

describe("监控分组层级", () => {
  it("乱序返回多层分组时按父子顺序展示并保留完整归属", () => {
    const rows = buildMonitorTree([child, nested, root]);
    expect(rows.map((row) => row.monitor.id)).toEqual([1, 2, 3]);
    expect(rows[2]?.parentLabel).toBe("openai / 生产");
  });
  it("同名分组通过稳定 ID 筛选，包含嵌套子组且不混入另一同名分组", () => {
    const other = { ...root, id: 4, key: "id:4" };
    const rows = buildMonitorTree([other, child, nested, root]);
    expect(
      filterMonitorTree(rows, { ...options, group: "1" }).map((row) => row.monitor.id),
    ).toEqual([1, 2, 3]);
  });
  it("状态筛选匹配子项时保留不匹配的父组作为层级上下文", () => {
    const rows = buildMonitorTree([{ ...root, status: 1 }, nested, child]);
    expect(
      filterMonitorTree(rows, { ...options, status: "故障" }).map((row) => row.monitor.id),
    ).toEqual([1, 2, 3]);
  });
  it("搜索分组名时展示该组及全部后代", () => {
    expect(
      filterMonitorTree(buildMonitorTree([root, nested, child]), {
        ...options,
        search: "openai",
      }).map((row) => row.monitor.id),
    ).toEqual([1, 2, 3]);
  });
  it("未分组筛选仅展示没有父组的普通监控", () => {
    const rows = buildMonitorTree([root, nested, child, monitor]);
    expect(
      filterMonitorTree(rows, { ...options, group: "ungrouped" }).map((row) => row.monitor.key),
    ).toEqual([monitor.key]);
  });
  it("父组缺失或循环时仍展示全部监控，不按名称补造关联", () => {
    const rows = buildMonitorTree([{ ...root, parent: 2 }, nested, { ...child, parent: 99 }]);
    expect(new Set(rows.map((row) => row.monitor.id))).toEqual(new Set([1, 2, 3]));
    expect(rows.find((row) => row.monitor.id === 3)?.parentLabel).toBe("分组 #99");
  });
  it("空列表和无稳定 ID 的指标不会被去重丢失", () => {
    expect(buildMonitorTree([])).toEqual([]);
    expect(
      buildMonitorTree([
        { ...monitor, id: 0, key: "a" },
        { ...monitor, id: 0, key: "b" },
      ]),
    ).toHaveLength(2);
  });
});
