import { expect, it } from "vitest";

import { adjustNewAPIModelPrice } from "../model-price-adjustment";

it("同一乘法链在变量两侧都有系数时只应用一次涨幅", () => {
  const result = adjustNewAPIModelPrice(
    {
      model: "tiered-model",
      input_ratio: "1",
      completion_ratio: "1",
      billing_mode: "tiered_expr",
      billing_expr: 'tier("base", 2 * p * 3 + c * 10)',
    },
    "increase",
    10,
  );

  expect(result.billing_expr).toBe('tier("base", 2.2 * p * 3 + c * 11)');
});

it("相邻计费变量共用一个系数时只应用一次降幅", () => {
  const result = adjustNewAPIModelPrice(
    {
      model: "tiered-model",
      input_ratio: "1",
      completion_ratio: "1",
      billing_mode: "tiered_expr",
      billing_expr: 'tier("base", p * 2 * c)',
    },
    "decrease",
    10,
  );

  expect(result.billing_expr).toBe('tier("base", p * 1.8 * c)');
});

it("多个变量的乘法链只调整一个系数", () => {
  const result = adjustNewAPIModelPrice(
    {
      model: "tiered-model",
      input_ratio: "1",
      completion_ratio: "1",
      billing_mode: "tiered_expr",
      billing_expr: 'tier("base", p * 2 * c * 3)',
    },
    "increase",
    10,
  );

  expect(result.billing_expr).toBe('tier("base", p * 2.2 * c * 3)');
});

it("调整实际价格时保留包含乘法文本的阶梯名称", () => {
  const result = adjustNewAPIModelPrice(
    {
      model: "tiered-model",
      input_ratio: "1",
      completion_ratio: "1",
      billing_mode: "tiered_expr",
      billing_expr: 'tier("p * 2", p * 2)',
    },
    "increase",
    10,
  );

  expect(result.billing_expr).toBe('tier("p * 2", p * 2.2)');
});
