import { describe, expect, it } from "vitest";

import { adjustNewAPIModelPrice } from "../model-price-adjustment";

describe("模型价格百分比调整", () => {
  it("上调普通 Token 价格时只缩放输入基价并保持相对倍率", () => {
    expect(
      adjustNewAPIModelPrice(
        {
          model: "gpt-5",
          input_ratio: "1",
          completion_ratio: "8",
          cache_ratio: "0.1",
          create_cache_ratio: "1.25",
        },
        "increase",
        10,
      ),
    ).toEqual({
      model: "gpt-5",
      input_ratio: "1.1",
      completion_ratio: "8",
      cache_ratio: "0.1",
      create_cache_ratio: "1.25",
    });
  });

  it("下调固定价格时缩放固定价格且不引入倍率字段", () => {
    expect(
      adjustNewAPIModelPrice(
        { model: "image-model", model_price: "0.04", input_ratio: "", completion_ratio: "" },
        "decrease",
        25,
      ),
    ).toEqual({
      model: "image-model",
      model_price: "0.03",
      input_ratio: "",
      completion_ratio: "",
    });
  });

  it("调整阶梯价格时缩放价格项但保留长上下文阈值", () => {
    const adjusted = adjustNewAPIModelPrice(
      {
        model: "tiered-model",
        input_ratio: "2.5",
        completion_ratio: "6",
        billing_mode: "tiered_expr",
        billing_expr:
          'len <= 272000 ? tier("standard", p * 5 + c * 30 + cr * 0.5) : tier("long", p * 10 + c * 45 + cr * 1)',
      },
      "increase",
      20,
    );

    expect(adjusted.input_ratio).toBe("3");
    expect(adjusted.billing_expr).toBe(
      'len <= 272000 ? tier("standard", p * 6 + c * 36 + cr * 0.6) : tier("long", p * 12 + c * 54 + cr * 1.2)',
    );
  });
});
