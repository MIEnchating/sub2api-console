import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { NewAPIModelPrice, Sub2APIModelPrice } from "@/api";
import {
  modelPriceColumnValues,
  newAPIPriceComparisonStatus,
  NewAPIModelPrices,
  RemoteModelPricesTable,
  remotePriceToNewAPIModelPrice,
} from "../model-prices";

beforeEach(() => vi.stubGlobal("PointerEvent", MouseEvent));
afterEach(() => vi.unstubAllGlobals());

function officialPrice(): Sub2APIModelPrice {
  return {
    source: "official",
    source_url: "https://api-docs.deepseek.com/zh-cn/quick_start/pricing",
    model: "deepseek-flash",
    input_price: "0.000001",
    output_price: "0.000004",
    cache_read_price: "0.00000002",
    model_ratio: "0.5",
    completion_ratio: "4",
    cache_ratio: "0.02",
    time_pricing: {
      timezone: "Asia/Shanghai",
      weekdays_only: true,
      periods: [
        { start_time: "09:00", end_time: "12:00" },
        { start_time: "14:00", end_time: "18:00" },
      ],
      peak: {
        input_price: "0.000002",
        output_price: "0.000008",
        cache_read_price: "0.00000004",
      },
    },
  };
}

describe("官方峰谷价格", () => {
  it("官方返回两档时展示来源、两档单价和带时区的工作日时段", () => {
    render(<RemoteModelPricesTable prices={[officialPrice()]} pending={false} error="" />);
    expect(screen.getByRole("link", { name: "官方价格" })).toHaveAttribute(
      "href",
      "https://api-docs.deepseek.com/zh-cn/quick_start/pricing",
    );
    expect(
      screen.getByText("周一至周五 09:00–12:00、14:00–18:00（Asia/Shanghai），其余时间为空闲"),
    ).toBeVisible();
    expect(screen.getByText("高峰 2")).toBeVisible();
    expect(screen.getByText("空闲 1")).toBeVisible();
    expect(screen.getByText("高峰 8")).toBeVisible();
    expect(screen.getByText("空闲 4")).toBeVisible();
    expect(screen.getByText("高峰 0.04")).toBeVisible();
    expect(screen.getByText("空闲 0.02")).toBeVisible();
  });

  it("同步两档价格时生成包含工作日和半开时段的计费表达式", () => {
    const result = remotePriceToNewAPIModelPrice(officialPrice());
    expect(result.billing_mode).toBe("tiered_expr");
    expect(result.billing_expr).toBe(
      '(weekday("Asia/Shanghai") >= 1 && weekday("Asia/Shanghai") <= 5) && ((hour("Asia/Shanghai") * 60 + minute("Asia/Shanghai") >= 540 && hour("Asia/Shanghai") * 60 + minute("Asia/Shanghai") < 720) || (hour("Asia/Shanghai") * 60 + minute("Asia/Shanghai") >= 840 && hour("Asia/Shanghai") * 60 + minute("Asia/Shanghai") < 1080)) ? tier("高峰", p * 2 + c * 8 + cr * 0.04) : tier("空闲", p * 1 + c * 4 + cr * 0.02)',
    );
    expect(modelPriceColumnValues(result).input).toBe("2 / 1");
    expect(newAPIPriceComparisonStatus(result, [officialPrice()])).toBe("matched");
  });

  it("单独改变高峰输出价格后同步原值，不套用统一折扣倍数", () => {
    const price = officialPrice();
    price.time_pricing!.peak.output_price = "0.0000017";
    const result = remotePriceToNewAPIModelPrice(price);
    expect(result.billing_expr).toContain('tier("高峰", p * 2 + c * 1.7 + cr * 0.04)');
    expect(result.billing_expr).toContain('tier("空闲", p * 1 + c * 4 + cr * 0.02)');
  });

  it.each([
    ["高峰价格", "c * 8", "c * 1.3"],
    ["时段边界", "< 720", "< 721"],
    ["工作日范围", "<= 5", "<= 6"],
  ])("读回的%s变化时标记为不一致", (_label, before, after) => {
    const result = remotePriceToNewAPIModelPrice(officialPrice());
    result.billing_expr = result.billing_expr?.replace(before, after);
    expect(newAPIPriceComparisonStatus(result, [officialPrice()])).toBe("mismatched");
  });

  it("批量同步预览两档价格和时段，确认后写入完整表达式", async () => {
    const write = vi.fn().mockImplementation(async (models: NewAPIModelPrice[]) => ({ models }));
    render(
      <NewAPIModelPrices
        models={[{ model: "deepseek-flash", input_ratio: "0.22", completion_ratio: "3" }]}
        onLoadManagementPrices={async () => ({ models: [officialPrice()] })}
        onWriteModelPrices={write}
      />,
    );
    fireEvent.click(screen.getByRole("checkbox", { name: "选择本页模型" }));
    fireEvent.click(screen.getByRole("button", { name: "批量同步（1）" }));
    const dialog = await screen.findByRole("dialog");
    expect(within(dialog).getByText(/周一至周五 09:00–12:00/)).toBeVisible();
    fireEvent.click(within(dialog).getByRole("button", { name: "确认同步 1 个模型" }));
    await waitFor(() =>
      expect(write).toHaveBeenCalledWith([remotePriceToNewAPIModelPrice(officialPrice())]),
    );
    expect(await screen.findByText("同步成功并已读回")).toBeVisible();
  });
});
