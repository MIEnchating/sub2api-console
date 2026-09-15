import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { NewAPIModelPrices } from "../model-prices";

beforeEach(() => vi.stubGlobal("PointerEvent", MouseEvent));
afterEach(() => vi.unstubAllGlobals());

it.each(["constructor", "__proto__"])(
  "模型名为%s时首次不能还原，调价成功后还原原始价格",
  async (model) => {
    const write = vi.fn().mockResolvedValue(true);
    const price = { model, input_ratio: "1", completion_ratio: "4" };
    render(<NewAPIModelPrices models={[price]} onWriteModelPrice={write} />);
    expect(screen.getByRole("button", { name: `还原 ${model} 价格` })).toBeDisabled();
    fireEvent.click(screen.getByRole("button", { name: `上调 ${model} 价格` }));
    fireEvent.click(screen.getByRole("button", { name: "确认上调" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    fireEvent.click(screen.getByRole("button", { name: `还原 ${model} 价格` }));
    await waitFor(() => expect(write).toHaveBeenLastCalledWith(price, "还原"));
  },
);

it("远程价格缓存已过期时禁用直接写入，刷新成功后恢复", () => {
  const props = {
    models: [],
    managementPrices: [
      {
        model: "model-a",
        input_price: "0.000001",
        output_price: "0.000004",
        model_ratio: "0.5",
        completion_ratio: "4",
      },
    ],
    onWriteManagementPrice: vi.fn(),
  };
  const view = render(<NewAPIModelPrices {...props} managementPricesStale />);
  fireEvent.click(screen.getByRole("tab", { name: "远程模型价格" }));
  expect(screen.getByRole("button", { name: "写入平台 model-a" })).toBeDisabled();
  view.rerender(<NewAPIModelPrices {...props} managementPricesStale={false} />);
  expect(screen.getByRole("button", { name: "写入平台 model-a" })).toBeEnabled();
});

it("批量同步名为__proto__的模型后正常展示该模型的读回结果", async () => {
  const model = { model: "__proto__", input_ratio: "1", completion_ratio: "4" };
  const reference = {
    model: "__proto__",
    input_price: "0.000001",
    output_price: "0.000004",
    model_ratio: "0.5",
    completion_ratio: "4",
  };
  render(
    <NewAPIModelPrices
      models={[model]}
      onLoadManagementPrices={async () => ({ models: [reference] })}
      onWriteModelPrices={async (models) => ({
        groups: [],
        models,
        unset_models: [],
        references: [],
        tool_prices: [],
        differences: [],
        fetched_at: "2026-09-14T00:00:00Z",
      })}
    />,
  );
  fireEvent.click(screen.getByRole("checkbox", { name: "选择本页模型" }));
  fireEvent.click(screen.getByRole("button", { name: "批量同步（1）" }));
  fireEvent.click(await screen.findByRole("button", { name: "确认同步 1 个模型" }));
  expect(await screen.findByText("同步成功并已读回")).toBeVisible();
});
