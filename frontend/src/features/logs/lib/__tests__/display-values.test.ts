import { expect, it } from "vitest";

import {
  formatLogDurationSeconds,
  formatLogValue,
  logDetailLabel,
  logStatusLabel,
  logTitleLabel,
} from "../log-display";

it.each([
  { context: "标题", format: logTitleLabel },
  { context: "状态", format: logStatusLabel },
  { context: "字段名", format: logDetailLabel },
  ...["operation_type", "phase", "skill", "source", "status", "origin", "unknown"].map((field) => ({
    context: field,
    format: (value: string) => formatLogValue(value, field),
  })),
])("日志$context遇到原型同名值时返回可渲染的文本", (fixture) => {
  expect(fixture.format("constructor")).toBe("constructor");
});

it("日志字段名为 __proto__ 时不会读取上下文字典原型", () => {
  expect(formatLogValue("constructor", "__proto__")).toBe("constructor");
});

it.each([
  { seconds: 59.96, expected: "1 分 0 秒" },
  { seconds: 119.96, expected: "2 分 0 秒" },
])("耗时 $seconds 秒在四舍五入达到整分时正确进位", (fixture) => {
  expect(formatLogDurationSeconds(fixture.seconds)).toBe(fixture.expected);
});
