import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import type { Sub2APIModelPrice } from "@/api";
import { RemoteModelPricesTable, remotePriceToNewAPIModelPrice } from "../model-prices";

afterEach(cleanup);
const price: Sub2APIModelPrice = {
  model: "default-tiered",
  source: "sub2api",
  input_price: "0.000002",
  output_price: "0.00001",
  model_ratio: "1",
  completion_ratio: "5",
  price_tiers: [
    {
      label: "标准",
      condition: "len <= 272000",
      input_price: "0.000002",
      output_price: "0.00001",
      cache_write_price: "0.0000025",
      cache_write_1h_price: "0.000004",
      cache_read_price: "0.0000002",
    },
    {
      label: "长上下文",
      condition: "",
      input_price: "0.000004",
      output_price: "0.000015",
      cache_write_price: "0.000005",
      cache_write_1h_price: "0.000008",
      cache_read_price: "0.0000004",
    },
  ],
  billing_expr:
    'len <= 272000 ? tier("标准", p * 2 + c * 10 + cr * 0.2 + cc * 2.5 + cc1h * 4) : tier("长上下文", p * 4 + c * 15 + cr * 0.4 + cc * 5 + cc1h * 8)',
};

it("Sub2API 默认价格含多档时展示所有档位和独立一小时缓存价", () => {
  render(<RemoteModelPricesTable prices={[price]} pending={false} error="" />);
  const row = within(screen.getByRole("row", { name: /default-tiered/ }));
  expect(row.getByText("Sub2API 默认")).toBeVisible();
  expect(row.getByText("阶梯计费 · 2 档")).toBeVisible();
  expect(row.getByText("标准 2.5 / 1 小时 4")).toBeVisible();
  expect(row.getByText("长上下文 5 / 1 小时 8")).toBeVisible();
});

it("同步默认阶梯价格时保留阈值、全部单价和一小时缓存项", () => {
  expect(remotePriceToNewAPIModelPrice(price)).toMatchObject({
    billing_mode: "tiered_expr",
    billing_expr: price.billing_expr,
  });
});

it("阶梯仅有一小时缓存写入单价时仍展示该价格", () => {
  const tiers = price.price_tiers?.map((tier) => ({ ...tier, cache_write_price: undefined }));
  render(
    <RemoteModelPricesTable prices={[{ ...price, price_tiers: tiers }]} pending={false} error="" />,
  );
  expect(screen.getByText("标准 - / 1 小时 4")).toBeVisible();
  expect(screen.getByText("长上下文 - / 1 小时 8")).toBeVisible();
});
