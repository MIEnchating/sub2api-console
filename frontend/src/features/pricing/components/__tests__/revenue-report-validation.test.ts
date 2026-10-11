import { expect, it } from "vitest";

import { revenueReportFromTask } from "../revenue-analysis-page";
import { revenueReport, revenueTask } from "./revenue-fixtures";

it.each([null, "失败", { host: {}, reason: "超时" }, { host: "api.example.test", reason: [] }])(
  "读取问题包含无效明细 %j 时拒绝报告，避免切换视图崩溃",
  (issue) => {
    const task = revenueTask();
    task.result = { ...revenueReport, issues: [issue] };
    expect(revenueReportFromTask(task)).toBeNull();
  },
);

it("读取问题包含有效中文原因时接受报告", () => {
  const report = { ...revenueReport, issues: [{ host: "api.example.test", reason: "上游超时" }] };
  expect(revenueReportFromTask(revenueTask(report))).toEqual(report);
});
