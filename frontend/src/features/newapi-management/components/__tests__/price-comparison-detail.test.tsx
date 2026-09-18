import { fireEvent, render, screen, within } from "@testing-library/react";
import { expect, it } from "vitest";

import { NewAPIModelPrices, remotePriceToNewAPIModelPrice } from "../model-prices";

const remote = {
  model: "claude-sonnet-4-6-thinking",
  input_price: "0.000003",
  output_price: "0.000015",
  cache_write_price: "0.00000375",
  cache_write_1h_price: "0.000006",
  cache_read_price: "0.0000003",
  model_ratio: "1.5",
  completion_ratio: "5",
};

it("单价相同但有附加费用时打开差异弹窗可见不一致的表达式", async () => {
  const configured = remotePriceToNewAPIModelPrice(remote);
  const expression = `${configured.billing_expr} + 100`;
  configured.billing_expr = expression;
  render(
    <NewAPIModelPrices
      models={[configured]}
      managementPrices={[remote]}
      onCompareManagementPrices={() => undefined}
    />,
  );
  fireEvent.click(screen.getByRole("button", { name: "比较模型价格" }));
  expect(screen.getByRole("row", { name: /claude-sonnet-4-6-thinking/ })).toHaveTextContent(
    "不一致",
  );
  fireEvent.click(screen.getByRole("button", { name: `查看 ${remote.model} 价格差异` }));

  const dialog = await screen.findByRole("dialog");
  const expressionRow = within(dialog).getByRole("row", { name: /计费表达式/ });
  expect(within(expressionRow).getByText(expression)).toBeVisible();
  expect(within(expressionRow).getByText("不一致")).toBeVisible();
});

it("单价和条件相同且加法项换序时列表显示一致", () => {
  const configured = remotePriceToNewAPIModelPrice(remote);
  configured.billing_expr = configured.billing_expr!.replace("p * 3 + c * 15", "c * 15 + p * 3");
  render(
    <NewAPIModelPrices
      models={[configured]}
      managementPrices={[remote]}
      onCompareManagementPrices={() => undefined}
    />,
  );
  fireEvent.click(screen.getByRole("button", { name: "比较模型价格" }));

  const row = screen.getByRole("row", { name: /claude-sonnet-4-6-thinking/ });
  expect(within(row).getByText("一致")).toBeVisible();
  expect(within(row).queryByRole("button", { name: /价格差异/ })).not.toBeInTheDocument();
});
