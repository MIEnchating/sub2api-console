import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterAll, beforeAll, describe, expect, it, vi } from "vitest";

import type { NewAPIRemoteSnapshot } from "@/api";
import { NewAPIPriceComparison, comparePlatformModelPrice } from "../price-comparison";

const snapshot: NewAPIRemoteSnapshot = {
  groups: [],
  models: [
    { model: "model-a", input_ratio: "1", completion_ratio: "2" },
    { model: "model-b", input_ratio: "2", completion_ratio: "4" },
  ],
  unset_models: [],
  tool_prices: [],
  references: [],
  upstream_prices: [
    {
      host: "upstream.example.test",
      name: "上游 A",
      upstream_type: "sub2api",
      models: [
        { model: "model-a", input_ratio: "1", completion_ratio: "2" },
        { model: "model-b", input_ratio: "3", completion_ratio: "4" },
      ],
    },
  ],
  differences: [],
  fetched_at: "2026-09-05T00:00:00Z",
};

beforeAll(() => vi.stubGlobal("PointerEvent", MouseEvent));
afterAll(() => vi.unstubAllGlobals());

describe("New API 价格比对", () => {
  it("选择上游和模型后执行批量比对并显示逐项结果", async () => {
    const user = userEvent.setup();
    render(<NewAPIPriceComparison snapshot={snapshot} />);

    const batchButton = screen.getByRole("button", { name: "批量比对" });
    expect(batchButton).toBeDisabled();

    await user.click(screen.getByRole("combobox", { name: "比对上游" }));
    await user.click(screen.getByRole("option", { name: /上游 A/ }));
    await user.click(screen.getByRole("checkbox", { name: "选择 model-a" }));
    await user.click(screen.getByRole("checkbox", { name: "选择 model-b" }));
    await user.click(batchButton);

    expect(screen.getByLabelText("model-a 比对结果")).toHaveTextContent("一致");
    expect(screen.getByLabelText("model-b 比对结果")).toHaveTextContent("不一致");
  });

  it("选定上游后可以对单个模型发起比对", async () => {
    const user = userEvent.setup();
    render(<NewAPIPriceComparison snapshot={snapshot} />);

    await user.click(screen.getByRole("combobox", { name: "比对上游" }));
    await user.click(screen.getByRole("option", { name: /上游 A/ }));
    await user.click(screen.getByRole("button", { name: "比对 model-b" }));

    expect(screen.getByRole("dialog", { name: "model-b 价格比对" })).toBeInTheDocument();
    expect(screen.getByText("价格不一致")).toBeInTheDocument();
    expect(screen.getByText("当前平台价格与 上游 A 价卡的逐项结果。")).toBeInTheDocument();
  });

  it("按价格字段区分一致、不同和上游缺失", () => {
    expect(
      comparePlatformModelPrice(snapshot.models[0]!, snapshot.upstream_prices![0]!.models),
    ).toBe("matched");
    expect(
      comparePlatformModelPrice(snapshot.models[1]!, snapshot.upstream_prices![0]!.models),
    ).toBe("mismatched");
    expect(
      comparePlatformModelPrice(
        { model: "missing", input_ratio: "1", completion_ratio: "2" },
        snapshot.upstream_prices![0]!.models,
      ),
    ).toBe("missing");
  });

  it("没有可用上游时保持比对操作禁用并显示原因", () => {
    render(<NewAPIPriceComparison snapshot={{ ...snapshot, upstream_prices: [] }} />);

    expect(screen.getByRole("button", { name: "批量比对" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "比对 model-a" })).toBeDisabled();
    expect(screen.getByText("尚未读取到可比对的上游价卡")).toBeInTheDocument();
  });

  it("按向下方向键打开上游列表并可通过 Tab 到达首个选项", async () => {
    const user = userEvent.setup();
    render(<NewAPIPriceComparison snapshot={snapshot} />);

    const selector = screen.getByRole("combobox", { name: "比对上游" });
    selector.focus();
    await user.keyboard("{ArrowDown}");

    expect(selector).toHaveAttribute("aria-expanded", "true");
    await user.tab();
    expect(screen.getByRole("option", { name: /上游 A/ })).toHaveFocus();
  });
});
