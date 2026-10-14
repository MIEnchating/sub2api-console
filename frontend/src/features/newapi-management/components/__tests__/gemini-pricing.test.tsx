import { render, screen } from "@testing-library/react";
import { expect, it, vi } from "vitest";

import { RemoteModelPricesTable } from "../model-prices";

it("Gemini 日期价显示前后两档及官网来源，仅禁用日期价写入", () => {
  render(
    <RemoteModelPricesTable
      prices={[
        {
          model: "gemini-4-argon",
          source: "official",
          source_url: "https://ai.google.dev/gemini-api/docs/pricing",
          source_scope: "USD；Gemini API Standard 付费档；切换时区未注明",
          input_price: "0.000002",
          output_price: "0.00001",
          model_ratio: "",
          completion_ratio: "",
          sync_error: "官网价格包含日期切换且未注明时区，请核对后手动设置",
          price_tiers: [
            {
              label: "截至 2026-11-30",
              condition: "through 2026-11-30",
              input_price: "0.000002",
              output_price: "0.00001",
              cache_read_price: "0.0000001",
            },
            {
              label: "2026-12-01 起",
              condition: "starting 2026-12-01",
              input_price: "0.000004",
              output_price: "0.00002",
              cache_read_price: "0.0000002",
            },
          ],
        },
        {
          model: "gemini-3.5-flash-lite",
          input_price: "0.0000003",
          output_price: "0.0000025",
          model_ratio: "0.15",
          completion_ratio: "8.33333333",
        },
      ]}
      pending={false}
      error=""
      onWritePrice={vi.fn()}
    />,
  );

  expect(screen.getByRole("link", { name: "官方价格" })).toHaveAttribute(
    "href",
    "https://ai.google.dev/gemini-api/docs/pricing",
  );
  expect(screen.getByText("截至 2026-11-30 2")).toBeVisible();
  expect(screen.getByText("2026-12-01 起 20")).toBeVisible();
  expect(screen.getByText("截至 2026-11-30 0.1")).toBeVisible();
  expect(screen.getByText("2026-12-01 起 0.2")).toBeVisible();
  expect(screen.getByRole("button", { name: "写入平台 gemini-4-argon" })).toBeDisabled();
  expect(screen.getByRole("button", { name: "写入平台 gemini-3.5-flash-lite" })).toBeEnabled();
});
