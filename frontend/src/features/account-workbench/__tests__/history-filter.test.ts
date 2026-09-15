import { describe, expect, it } from "vitest";
import type { Task } from "@/api";
import { filterHistory } from "../lib/history-filter";

const base: Task = {
  id: "import-1",
  skill: "account-workbench",
  operation: "account-workbench-import",
  status: "failed",
  message: "等待核对",
  progress: 100,
  created_at: "2026-09-14T00:00:00Z",
  updated_at: "2026-09-14T00:00:00Z",
  result: { items: [{ email: "owner@example.com" }] },
};
describe("工作台记录筛选", () => {
  it("状态与类型组合筛选同时排除不匹配记录", () => {
    const other: Task = { ...base, id: "oauth-1", operation: "account-workbench-oauth" };
    const succeeded: Task = { ...base, id: "import-2", status: "succeeded" };
    expect(
      filterHistory([base, other, succeeded], "", "failed", "account-workbench-import"),
    ).toEqual([base]);
  });
  it("长搜索内容和空记录返回空结果", () => {
    expect(filterHistory([base], "a".repeat(2000), "", "")).toEqual([]);
    expect(filterHistory([], "owner", "", "")).toEqual([]);
  });
});
