import { expect, it } from "vitest";
import { newAPIPriceComparisonStatus, remotePriceToNewAPIModelPrice } from "../model-prices";

const remote = {
  model: "tiered-model",
  input_price: "0.000005",
  output_price: "0.00003",
  model_ratio: "2.5",
  completion_ratio: "6",
  long_context_threshold: 272000,
  long_context_input_price: "0.00001",
  long_context_output_price: "0.000045",
};

it.each([
  ["附加费用", (expr: string) => `${expr} + 100`],
  ["改变输出运算符", (expr: string) => expr.replace("p * 5 + c * 30", "p * 5 - c * 30")],
  ["额外条件", (expr: string) => expr.replace("len <= 272000", "len <= 272000 && c < 100")],
])("阶梯价格%s时不能只按相同数字判定价格一致", (_label, change) => {
  const configured = remotePriceToNewAPIModelPrice(remote);
  configured.billing_expr = change(configured.billing_expr!);
  expect(newAPIPriceComparisonStatus(configured, [remote])).toBe("mismatched");
});

it("仅金额的等价小数写法不同时仍判定阶梯价格一致", () => {
  const configured = remotePriceToNewAPIModelPrice(remote);
  configured.billing_expr = configured.billing_expr!.replace("p * 5", "p * 5.0");
  expect(newAPIPriceComparisonStatus(configured, [remote])).toBe("matched");
});
