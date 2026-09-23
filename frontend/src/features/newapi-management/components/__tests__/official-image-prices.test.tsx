import { render, screen, within } from "@testing-library/react";
import { expect, it, vi } from "vitest";
import { RemoteModelPricesTable } from "../model-prices";

it("官方图片参考价分别按图片和 Token 展示，保留来源并禁用自动写入", () => {
  render(
    <RemoteModelPricesTable
      pending={false}
      error=""
      onWritePrice={vi.fn()}
      prices={[
        {
          model: "grok-imagine-image-2.0",
          source: "official",
          source_url: "https://docs.x.ai/developers/models",
          source_scope: "USD；每张图片 0.04",
          input_price: "",
          output_price: "",
          model_ratio: "",
          completion_ratio: "",
          image_output_price: "0.04",
          image_output_unit: "image",
          sync_error: "该模型按图片数量计费，暂不支持自动同步",
        },
        {
          model: "gpt-image-2",
          source: "official",
          source_url: "https://developers.openai.com/api/docs/pricing",
          source_scope: "USD／百万 Token；Standard",
          input_price: "0.000005",
          output_price: "",
          model_ratio: "",
          completion_ratio: "",
          image_input_price: "0.000008",
          image_output_price: "0.00003",
          image_output_unit: "token",
          sync_error: "该模型按文本和图片分别计价，暂不支持自动同步",
        },
      ]}
    />,
  );
  const grok = screen.getByRole("row", { name: /grok-imagine-image-2.0/ });
  expect(within(grok).getByText("输出 0.04 / 张")).toBeVisible();
  expect(within(grok).getByRole("link", { name: "官方价格" })).toHaveAttribute(
    "href",
    "https://docs.x.ai/developers/models",
  );
  const openai = screen.getByRole("row", { name: /gpt-image-2/ });
  expect(within(openai).getByText("输入 8，输出 30 / 百万 Token")).toBeVisible();
  expect(within(openai).getByRole("button", { name: "写入平台 gpt-image-2" })).toBeDisabled();
});
