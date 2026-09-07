import { fireEvent, render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { NewAPIModelPrices } from "../model-prices";

describe("模型参考价缓存", () => {
  it("手动刷新时触发拉取，刷新期间禁用按钮并保留已有价格", () => {
    const refresh = vi.fn();
    const props = {
      models: [],
      managementPrices: [
        {
          model: "kimi-k3",
          source: "sub2api" as const,
          input_price: "0.000001",
          output_price: "0.000004",
          model_ratio: "0.5",
          completion_ratio: "4",
        },
      ],
      onRefreshManagementPrices: refresh,
    };
    const view = render(<NewAPIModelPrices {...props} />);
    fireEvent.click(screen.getByRole("tab", { name: "远程模型价格" }));
    const pagination = screen.getByRole("button", { name: "转到下一页" });
    fireEvent.click(screen.getByRole("button", { name: "强制刷新参考价格" }));
    expect(refresh).toHaveBeenCalledOnce();
    view.rerender(<NewAPIModelPrices {...props} managementPricesPending />);
    expect(screen.getByRole("button", { name: "强制刷新参考价格" })).toBeDisabled();
    const row = screen.getByRole("row", { name: /kimi-k3/ });
    expect(within(row).getByText("Sub2API 默认")).toBeVisible();
    expect(screen.getByRole("button", { name: "转到下一页" })).toBe(pagination);
  });

  it("已有比较结果时后台刷新不替换状态，失败后保留结果并提示缓存", () => {
    const props = {
      models: [{ model: "model-a", input_ratio: "0.5", completion_ratio: "4" }],
      managementPrices: [
        {
          model: "model-a",
          input_price: "0.000001",
          output_price: "0.000004",
          model_ratio: "0.5",
          completion_ratio: "4",
        },
      ],
      onCompareManagementPrices: vi.fn(),
    };
    const view = render(<NewAPIModelPrices {...props} />);
    fireEvent.click(screen.getByRole("button", { name: "比较模型价格" }));
    const status = screen.getByText("一致", { exact: true });
    view.rerender(<NewAPIModelPrices {...props} managementPricesPending />);
    expect(screen.getByText("一致", { exact: true })).toBe(status);
    expect(screen.queryByText("比较中", { exact: true })).not.toBeInTheDocument();
    view.rerender(<NewAPIModelPrices {...props} managementPricesError="连接失败" />);
    expect(screen.getByText("一致", { exact: true })).toBe(status);
    expect(screen.getByText(/连接失败/)).toBeVisible();
  });

  it("刷新失败时展示过期提示且已有缓存仍可查看", () => {
    render(
      <NewAPIModelPrices
        models={[]}
        managementPrices={[
          {
            model: "cached-model",
            source: "remote",
            input_price: "0.000001",
            output_price: "0.000004",
            model_ratio: "0.5",
            completion_ratio: "4",
          },
        ]}
        managementPricesStale
        managementPricesWarning="刷新失败，请稍后刷新"
        managementPricesError="连接失败"
      />,
    );
    fireEvent.click(screen.getByRole("tab", { name: "远程模型价格" }));
    expect(screen.getByText("缓存过期或刷新不完整")).toBeVisible();
    expect(screen.getByText("刷新失败，请稍后刷新")).toBeVisible();
    expect(screen.getByRole("row", { name: /cached-model/ })).toBeVisible();
  });
});
