import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import type { Sub2APIModelPrice } from "@/api";
import {
  RemoteModelPricesTable,
  remotePriceToNewAPIModelPrice,
  newAPIPriceComparisonStatus,
} from "../model-prices";

function price(): Sub2APIModelPrice {
  return {
    model: "GLM-4.7",
    source: "official",
    source_url: "https://docs.bigmodel.cn/cn/guide/start/pricing",
    source_scope: "实时调用",
    model_ratio: "1",
    completion_ratio: "4",
    input_price: "0.000002",
    output_price: "0.000008",
    price_tiers: [
      {
        label: "短输入、短输出",
        condition: "len < 32000 && c < 200",
        input_price: "0.000002",
        output_price: "0.000008",
      },
      {
        label: "短输入、长输出",
        condition: "len < 32000 && c >= 200",
        input_price: "0.000003",
        output_price: "0.000014",
      },
      {
        label: "长输入",
        condition: "len >= 32000",
        input_price: "0.000004",
        output_price: "0.000016",
      },
    ],
    billing_expr:
      'len < 32000 && c < 200 ? tier("短输入、短输出", p * 2 + c * 8) : len < 32000 && c >= 200 ? tier("短输入、长输出", p * 3 + c * 14) : tier("长输入", p * 4 + c * 16)',
  };
}
describe("官方多级价格", () => {
  it("官网有三个档位时展示每档输入输出及来源范围", () => {
    render(<RemoteModelPricesTable prices={[price()]} pending={false} error="" />);
    expect(screen.getByRole("link", { name: "官方价格" })).toHaveAttribute(
      "href",
      price().source_url,
    );
    expect(screen.getByText("实时调用")).toBeVisible();
    expect(screen.getByText("短输入、短输出 2")).toBeVisible();
    expect(screen.getByText("短输入、长输出 14")).toBeVisible();
    expect(screen.getByText("长输入 16")).toBeVisible();
  });
  it("同步官方阶梯时保留后端的完整条件和单价", () => {
    const result = remotePriceToNewAPIModelPrice(price());
    expect(result.billing_expr).toBe(price().billing_expr);
    expect(result.billing_mode).toBe("tiered_expr");
    expect(newAPIPriceComparisonStatus(result, [price()])).toBe("matched");
  });
  it("只修改第二个输出长度条件时仍识别为价格不一致", () => {
    const result = remotePriceToNewAPIModelPrice(price());
    result.billing_expr = price().billing_expr?.replace("c >= 200", "c >= 201");
    expect(newAPIPriceComparisonStatus(result, [price()])).toBe("mismatched");
  });
});
