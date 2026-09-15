import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";

import { NewAPIModelPrices } from "../model-prices";

const fetchedAt = "2026-09-14T07:05:35Z";
const props = {
  models: [{ model: "model-a", input_ratio: "0.5", completion_ratio: "4" }],
  unsetModels: [{ model: "model-b", input_ratio: "", completion_ratio: "" }],
  managementPrices: [
    {
      model: "model-a",
      input_price: "0.000001",
      output_price: "0.000004",
      model_ratio: "0.5",
      completion_ratio: "4",
    },
  ],
  managementPricesFetchedAt: fetchedAt,
};

describe("模型价格缓存时间位置", () => {
  it("各价格分类都在表格分页下方显示更新时间", async () => {
    const user = userEvent.setup();
    render(<NewAPIModelPrices {...props} />);
    for (const label of ["模型价格", "未设置模型价格", "远程模型价格"]) {
      await user.click(screen.getByRole("tab", { name: label }));
      const timestamp = screen.getByRole("status", { name: "价格缓存更新时间" });
      const pagination = screen.getByRole("navigation", { name: "表格分页" });
      expect(
        pagination.compareDocumentPosition(timestamp) & Node.DOCUMENT_POSITION_FOLLOWING,
      ).toBeTruthy();
      expect(
        within(timestamp).getByText(new Date(fetchedAt).toLocaleString("zh-CN")),
      ).toHaveAttribute("datetime", fetchedAt);
      expect(timestamp).toHaveClass("shrink-0", "text-right", "text-xs");
      expect(screen.getAllByText(/价格缓存更新/)).toHaveLength(1);
    }
  });

  it("没有缓存时间时不显示时间占位", () => {
    render(<NewAPIModelPrices models={[]} />);
    expect(screen.queryByRole("status", { name: "价格缓存更新时间" })).not.toBeInTheDocument();
  });

  it("模型为空且后台刷新时仍保留底部已有更新时间", () => {
    render(
      <NewAPIModelPrices
        models={[]}
        managementPricesFetchedAt={fetchedAt}
        managementPricesPending
      />,
    );
    const empty = screen.getByText("尚未读取到模型价格");
    const timestamp = screen.getByRole("status", { name: "价格缓存更新时间" });
    expect(
      empty.compareDocumentPosition(timestamp) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
  });

  it("缓存过期时警告保持在分类上方，时间仍显示在底部", () => {
    render(
      <NewAPIModelPrices
        {...props}
        managementPricesStale
        managementPricesWarning="官网价格读取失败，请稍后刷新"
      />,
    );
    const tabs = screen.getByRole("tablist", { name: "价格分类" });
    const warning = screen.getByText("缓存过期或刷新不完整");
    expect(warning.compareDocumentPosition(tabs) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(screen.getByRole("status", { name: "价格缓存更新时间" })).not.toContainElement(warning);
  });
});
