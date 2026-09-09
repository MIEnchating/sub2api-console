import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { matchingRemoteModelPrice, NewAPIModelPrices } from "../model-prices";

describe("模型价格行操作", () => {
  it("远程同步只接受模型名称完全匹配的价格", () => {
    const price = {
      model: "gpt-5",
      input_price: "0.000001",
      output_price: "0.000008",
      model_ratio: "0.5",
      completion_ratio: "8",
    };

    expect(matchingRemoteModelPrice([price], "gpt-5")).toBe(price);
    expect(matchingRemoteModelPrice([price], "GPT-5")).toBeNull();
    expect(matchingRemoteModelPrice([price], "gpt-5-mini")).toBeNull();
  });

  it("Gemini 思考等级变体继承基础模型价格并保留目标模型名", () => {
    const price = {
      model: "gemini-3.8-flash",
      input_price: "0.00000075",
      output_price: "0.00000375",
      model_ratio: "0.375",
      completion_ratio: "5",
    };

    for (const suffix of ["high", "low", "medium", "tiered"]) {
      const model = `${price.model}-${suffix}`;
      expect(matchingRemoteModelPrice([price], model)).toEqual({ ...price, model });
    }
    const exactVariant = {
      ...price,
      model: `${price.model}-high`,
      model_ratio: "0.5",
    };
    expect(matchingRemoteModelPrice([price, exactVariant], exactVariant.model)).toBe(exactVariant);
    expect(matchingRemoteModelPrice([price], `${price.model}-image`)).toBeNull();
  });

  it("使用图标 Tooltip 提供上调、下调、还原和远程同步", () => {
    render(
      <NewAPIModelPrices
        models={[{ model: "gpt-5", input_ratio: "1", completion_ratio: "8" }]}
        onWriteModelPrice={vi.fn().mockResolvedValue(true)}
        onSyncModelPrice={vi.fn().mockResolvedValue(true)}
      />,
    );

    expect(screen.getByRole("button", { name: "上调 gpt-5 价格" })).toBeEnabled();
    expect(screen.getByRole("button", { name: "下调 gpt-5 价格" })).toBeEnabled();
    expect(screen.getByRole("button", { name: "还原 gpt-5 价格" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "同步 gpt-5 远程模型价格" })).toBeEnabled();
  });

  it("输入百分比上调并可还原首次调价前的价格", async () => {
    const user = userEvent.setup();
    const write = vi.fn().mockResolvedValue(true);
    render(
      <NewAPIModelPrices
        models={[{ model: "gpt-5", input_ratio: "1", completion_ratio: "8" }]}
        onWriteModelPrice={write}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "上调 gpt-5 价格" }));
    expect(screen.getByRole("dialog", { name: "gpt-5 价格上调" })).toBeVisible();
    expect(screen.getByLabelText("调整百分比")).toHaveValue(10);
    fireEvent.change(screen.getByLabelText("调整百分比"), { target: { value: "15" } });
    await user.click(screen.getByRole("button", { name: "确认上调" }));

    await waitFor(() =>
      expect(write).toHaveBeenCalledWith(
        { model: "gpt-5", input_ratio: "1.15", completion_ratio: "8" },
        "上调 15%",
      ),
    );
    fireEvent.click(screen.getByRole("button", { name: "还原 gpt-5 价格" }));
    await waitFor(() =>
      expect(write).toHaveBeenLastCalledWith(
        { model: "gpt-5", input_ratio: "1", completion_ratio: "8" },
        "还原",
      ),
    );
  });

  it("输入百分比下调时按当前价格计算且保持相对倍率", async () => {
    const user = userEvent.setup();
    const write = vi.fn().mockResolvedValue(true);
    render(
      <NewAPIModelPrices
        models={[
          {
            model: "gpt-5-mini",
            input_ratio: "0.25",
            completion_ratio: "8",
            cache_ratio: "0.1",
          },
        ]}
        onWriteModelPrice={write}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "下调 gpt-5-mini 价格" }));
    fireEvent.change(screen.getByLabelText("调整百分比"), { target: { value: "20" } });
    await user.click(screen.getByRole("button", { name: "确认下调" }));

    await waitFor(() =>
      expect(write).toHaveBeenCalledWith(
        {
          model: "gpt-5-mini",
          input_ratio: "0.2",
          completion_ratio: "8",
          cache_ratio: "0.1",
        },
        "下调 20%",
      ),
    );
  });

  it("同步远程价格前保存当前价格作为还原基线", async () => {
    const sync = vi.fn().mockResolvedValue(true);
    const write = vi.fn().mockResolvedValue(true);
    render(
      <NewAPIModelPrices
        models={[{ model: "claude", input_ratio: "2", completion_ratio: "5" }]}
        onWriteModelPrice={write}
        onSyncModelPrice={sync}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "同步 claude 远程模型价格" }));
    await waitFor(() => expect(sync).toHaveBeenCalledWith("claude"));
    expect(screen.getByRole("button", { name: "还原 claude 价格" })).toBeEnabled();
  });
});
