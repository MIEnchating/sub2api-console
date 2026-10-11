import { describe, expect, it } from "vitest";

import { formatTaskDate } from "../format-task-date";

describe("formatTaskDate", () => {
  it("有效任务时间保留本地年份、日期、秒和24小时制", () => {
    const value = "2026-10-09T16:07:38Z";

    expect(formatTaskDate(value)).toBe(new Date(value).toLocaleString("zh-CN", { hour12: false }));
  });

  it("无效或缺失任务时间保留原文", () => {
    expect(formatTaskDate("无法读取时间")).toBe("无法读取时间");
    expect(formatTaskDate("")).toBe("");
  });

  it("连续格式化不同年份不会复用前一次时间", () => {
    const values = ["2025-01-02T00:00:00Z", "2026-12-31T23:59:59Z"];

    for (const value of values) {
      expect(formatTaskDate(value)).toBe(
        new Date(value).toLocaleString("zh-CN", { hour12: false }),
      );
    }
  });
});
