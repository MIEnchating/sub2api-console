import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { NewAPIRemoteSnapshot } from "@/api";
import { NewAPIModelPrices } from "../model-prices";
import userEvent from "@testing-library/user-event";

beforeEach(() => vi.stubGlobal("PointerEvent", MouseEvent));
afterEach(() => vi.unstubAllGlobals());

const snapshot: NewAPIRemoteSnapshot = {
  groups: [],
  models: [],
  unset_models: [],
  references: [],
  tool_prices: [],
  differences: [],
  fetched_at: "2026-09-07T00:00:00Z",
};
const prices = [
  {
    model: "model-a",
    input_price: "0.000001",
    output_price: "0.000004",
    model_ratio: "0.5",
    completion_ratio: "4",
  },
];
const models = [
  { model: "model-a", input_ratio: "1", completion_ratio: "4" },
  { model: "model-missing", input_ratio: "1", completion_ratio: "4" },
];

describe("批量模型价格同步", () => {
  it("勾选后先预览，确认时一次写入有效模型并跳过缺失价格", async () => {
    const write = vi.fn().mockResolvedValue({
      ...snapshot,
      models: [{ model: "model-a", input_ratio: "0.5", completion_ratio: "4" }],
    });
    render(
      <NewAPIModelPrices
        models={models}
        onLoadManagementPrices={async () => ({ models: prices })}
        onWriteModelPrices={write}
      />,
    );
    fireEvent.click(screen.getByRole("checkbox", { name: "选择本页模型" }));
    fireEvent.click(screen.getByRole("button", { name: "批量同步（2）" }));
    const confirm = await screen.findByRole("button", { name: "确认同步 1 个模型" });
    expect(write).not.toHaveBeenCalled();
    expect(screen.getByText("跳过：参考价未找到")).toBeVisible();
    fireEvent.click(confirm);
    await waitFor(() =>
      expect(write).toHaveBeenCalledWith([
        { model: "model-a", input_ratio: "0.5", completion_ratio: "4" },
      ]),
    );
    expect(await screen.findByText("同步成功并已读回")).toBeVisible();
  });
  it("筛选和翻页后保留选择，取消预览不写入平台", async () => {
    const write = vi.fn();
    const many = Array.from({ length: 21 }, (_, index) => ({
      model: `model-${String(index).padStart(2, "0")}`,
      input_ratio: "1",
      completion_ratio: "4",
    }));
    render(
      <NewAPIModelPrices
        models={many}
        onLoadManagementPrices={async () => ({ models: [] })}
        onWriteModelPrices={write}
      />,
    );
    fireEvent.click(screen.getByRole("checkbox", { name: "选择模型 model-00" }));
    fireEvent.click(screen.getByRole("button", { name: "转到下一页" }));
    fireEvent.click(screen.getByRole("checkbox", { name: "选择模型 model-20" }));
    fireEvent.change(screen.getByRole("textbox", { name: "搜索模型" }), {
      target: { value: "model-20" },
    });
    expect(screen.getByRole("checkbox", { name: "选择模型 model-20" })).toBeChecked();
    fireEvent.click(screen.getByRole("button", { name: "批量同步（2）" }));
    const dialog = await screen.findByRole("dialog");
    await waitFor(() => expect(within(dialog).getByText("model-00")).toBeVisible());
    expect(within(dialog).getByText("model-20")).toBeVisible();
    fireEvent.click(within(dialog).getByRole("button", { name: "取消" }));
    expect(write).not.toHaveBeenCalled();
    expect(screen.getByRole("button", { name: "批量同步（2）" })).toBeEnabled();
  });
  it("键盘选择一行后表头显示部分选中，全选筛选结果只增加匹配行", async () => {
    const user = userEvent.setup();
    render(
      <NewAPIModelPrices
        models={models}
        onLoadManagementPrices={async () => ({ models: prices })}
        onWriteModelPrices={vi.fn()}
      />,
    );
    const checkbox = screen.getByRole("checkbox", { name: "选择模型 model-a" });
    checkbox.focus();
    await user.keyboard(" ");
    expect(checkbox).toBeChecked();
    expect(screen.getByRole("checkbox", { name: "选择本页模型" })).toBePartiallyChecked();
    fireEvent.change(screen.getByRole("textbox", { name: "搜索模型" }), {
      target: { value: "missing" },
    });
    fireEvent.click(screen.getByRole("button", { name: "全选筛选结果" }));
    expect(screen.getByRole("button", { name: "批量同步（2）" })).toBeEnabled();
  });
  it("读回内容与目标价格不同则保留选择并提示核对", async () => {
    render(
      <NewAPIModelPrices
        models={models}
        onLoadManagementPrices={async () => ({ models: prices })}
        onWriteModelPrices={async () => ({ ...snapshot, models })}
      />,
    );
    fireEvent.click(screen.getByRole("checkbox", { name: "选择模型 model-a" }));
    fireEvent.click(screen.getByRole("button", { name: "批量同步（1）" }));
    fireEvent.click(await screen.findByRole("button", { name: "确认同步 1 个模型" }));
    expect(await screen.findByText("已提交，读回价格未匹配，请核对")).toBeVisible();
    fireEvent.click(
      within(screen.getByRole("dialog")).getAllByRole("button", { name: "关闭" })[0]!,
    );
    expect(screen.getByRole("checkbox", { name: "选择模型 model-a" })).toBeChecked();
  });
  it("远程模型页允许多选，切换分类后清空原分类的选择", () => {
    render(
      <NewAPIModelPrices
        models={models}
        managementPrices={prices}
        onLoadManagementPrices={async () => ({ models: prices })}
        onWriteModelPrices={vi.fn()}
      />,
    );
    fireEvent.click(screen.getByRole("tab", { name: "远程模型价格" }));
    fireEvent.click(screen.getByRole("checkbox", { name: "选择本页参考模型" }));
    expect(screen.getByRole("button", { name: "批量同步（1）" })).toBeEnabled();
    fireEvent.click(screen.getByRole("tab", { name: "模型价格" }));
    expect(screen.queryByRole("toolbar", { name: /个模型的批量操作/ })).not.toBeInTheDocument();
  });
  it("加载参考价失败时显示原因且禁止确认", async () => {
    render(
      <NewAPIModelPrices
        models={models}
        onLoadManagementPrices={async () => {
          throw new Error("价格读取失败");
        }}
        onWriteModelPrices={vi.fn()}
      />,
    );
    fireEvent.click(screen.getByRole("checkbox", { name: "选择本页模型" }));
    fireEvent.click(screen.getByRole("button", { name: "批量同步（2）" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("价格读取失败");
    expect(screen.getByRole("button", { name: "确认同步 0 个模型" })).toBeDisabled();
  });
  it("参考价已过期时禁止批量写入并引导刷新", async () => {
    const write = vi.fn();
    render(
      <NewAPIModelPrices
        models={models}
        onLoadManagementPrices={async () => ({ models: prices, stale: true })}
        onWriteModelPrices={write}
      />,
    );
    fireEvent.click(screen.getByRole("checkbox", { name: "选择模型 model-a" }));
    fireEvent.click(screen.getByRole("button", { name: "批量同步（1）" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("请先强制刷新参考价格");
    expect(screen.getByRole("button", { name: "确认同步 0 个模型" })).toBeDisabled();
    expect(write).not.toHaveBeenCalled();
  });
  it("不支持的计费格式只展示跳过原因", async () => {
    render(
      <NewAPIModelPrices
        models={models}
        onLoadManagementPrices={async () => ({
          models: [{ ...prices[0]!, model_ratio: "", completion_ratio: "" }],
        })}
        onWriteModelPrices={vi.fn()}
      />,
    );
    fireEvent.click(screen.getByRole("checkbox", { name: "选择模型 model-a" }));
    fireEvent.click(screen.getByRole("button", { name: "批量同步（1）" }));
    expect(await screen.findByText("跳过：不支持此计费格式")).toBeVisible();
    expect(screen.getByRole("button", { name: "确认同步 0 个模型" })).toBeDisabled();
  });
  it("写入失败时保留预览和选择并允许重试", async () => {
    const write = vi.fn().mockRejectedValue(new Error("平台写入失败"));
    render(
      <NewAPIModelPrices
        models={models}
        onLoadManagementPrices={async () => ({ models: prices })}
        onWriteModelPrices={write}
      />,
    );
    fireEvent.click(screen.getByRole("checkbox", { name: "选择模型 model-a" }));
    fireEvent.click(screen.getByRole("button", { name: "批量同步（1）" }));
    const confirm = await screen.findByRole("button", { name: "确认同步 1 个模型" });
    fireEvent.click(confirm);
    expect(await screen.findByRole("alert")).toHaveTextContent("平台写入失败");
    expect(confirm).toBeEnabled();
    expect(screen.queryByText("同步成功并已读回")).not.toBeInTheDocument();
  });
});
