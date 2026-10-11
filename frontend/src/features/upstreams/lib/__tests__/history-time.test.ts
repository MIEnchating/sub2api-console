import { expect, it } from "vitest";
import { groupHistoryTime } from "../group-history";

it.each(["2026-01-02T00:00:00Z", "2025-12-31T23:59:59Z", "2026-10-09T12:34:56+08:00"])(
  "分组历史时间 %s 保留本地年份、日期、秒和24小时制",
  (value) => {
    expect(groupHistoryTime(value)).toBe(
      new Date(value).toLocaleString("zh-CN", { hour12: false }),
    );
  },
);

it.each(["", "未提供时间"])("无效历史时间 %s 保留原文", (value) => {
  expect(groupHistoryTime(value)).toBe(value);
});
