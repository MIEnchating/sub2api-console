import { describe, expect, it } from "vitest";
import { formatDate, formatDateTime } from "../date-format";

describe("共享日期展示", () => {
  const localDate = new Date(2026, 9, 9, 12, 34, 56);

  it("ISO 时间按浏览器本地时间展示，可选择保留秒", () => {
    expect(formatDate(localDate.toISOString())).toBe("10/09 12:34");
    expect(formatDate(localDate.toISOString(), true)).toBe("10/09 12:34:56");
  });

  it.each([
    ["秒", String(localDate.getTime() / 1000)],
    ["毫秒", String(localDate.getTime())],
  ])("%s 时间戳与 ISO 时间保持一致", (_unit, value) => {
    expect(formatDate(value, true)).toBe("10/09 12:34:56");
  });

  it.each([null, undefined, ""])("缺少时间 %s 时明确显示未知时间", (value) => {
    expect(formatDate(value)).toBe("未知时间");
  });

  it("无法解析的历史时间原样保留", () => {
    expect(formatDate("历史记录未提供时间")).toBe("历史记录未提供时间");
  });
});

describe("完整日期展示", () => {
  it.each(["2026-01-02T00:00:00Z", "2025-12-31T23:59:59Z", "2026-10-09T12:34:56+08:00"])(
    "时间 %s 与原有中文本地日期、秒和24小时制格式一致",
    (value) => {
      expect(formatDateTime(value)).toBe(
        new Date(value).toLocaleString("zh-CN", { hour12: false }),
      );
    },
  );

  it.each(["", "未提供时间", "1791504000000"])("无效完整日期 %s 原样保留", (value) => {
    expect(formatDateTime(value)).toBe(value);
  });
});
