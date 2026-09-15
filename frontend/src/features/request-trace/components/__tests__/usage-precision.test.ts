import { expect, it } from "vitest";
import { readableUsage } from "../system-log-search-panel";

it("日志Token数量超过安全整数范围时仍完整保留每一位数字", () => {
  expect(
    readableUsage({
      usage_available: true,
      input_tokens: "9007199254740993",
      output_tokens: "25",
      cache_read_tokens: null,
      cache_write_tokens: "0",
    }),
  ).toEqual({ input: "9,007,199,254,740,993", output: "25", cacheRead: "未提供", cacheWrite: "0" });
});
