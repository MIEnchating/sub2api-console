import { expect, it } from "vitest";
import { formatUsageAmount } from "../usage-totals";

it("格式化大额消费时保留整数和小数精度", () => {
  expect(formatUsageAmount("9007199254740993.000010")).toBe("9,007,199,254,740,993.00001");
});
it("小于显示精度的消费显示下界提示而非零", () => {
  expect(formatUsageAmount("0.0000001")).toBe("<0.000001");
  expect(formatUsageAmount("0.0000000")).toBe("0");
});
