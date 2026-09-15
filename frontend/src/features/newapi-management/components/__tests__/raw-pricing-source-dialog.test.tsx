import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { Toaster } from "sonner";

import type { RemoteModelPricingSource } from "@/api";
import { RawPricingSourceContent } from "../raw-pricing-source-dialog";

const content =
  '{\n  "gpt-test": {"input_cost_per_token": 1e-6, "output_cost_per_token": 0, "litellm_provider": "openai", "supports_vision": true, "max_input_tokens": 128000, "custom_field": {"region": "test"}},\n  "image-test": {"output_cost_per_image": 0.04, "mode": "image_generation"}\n}\n';
const source: RemoteModelPricingSource = {
  source_url: "https://raw.example/model-prices.json",
  content,
  fetched_at: "2026-09-04T12:30:00Z",
  size_bytes: 512,
  sha256: "abc123",
};

afterEach(() => vi.restoreAllMocks());

describe("远程价卡阅读", () => {
  it("打开价卡时默认显示模型列表，将 Token 单价换算为百万单位且保留零价格", () => {
    render(<RawPricingSourceContent source={source} pending={false} error="" />);
    expect(screen.getByRole("tab", { name: "模型明细" })).toHaveAttribute("aria-selected", "true");
    const row = screen.getByRole("row", { name: /gpt-test/ });
    expect(within(row).getByText("1", { exact: true })).toBeVisible();
    expect(within(row).getByText("0", { exact: true })).toBeVisible();
    expect(screen.queryByText("custom_field")).not.toBeInTheDocument();
  });

  it("按厂商搜索时仅显示匹配模型，无结果后清空搜索可以恢复列表", async () => {
    const user = userEvent.setup();
    render(<RawPricingSourceContent source={source} pending={false} error="" />);
    const search = screen.getByRole("textbox", { name: "搜索模型或厂商" });
    await user.type(search, "OPENAI");
    expect(screen.getByRole("button", { name: "查看 gpt-test 明细" })).toBeVisible();
    expect(screen.queryByRole("button", { name: "查看 image-test 明细" })).not.toBeInTheDocument();
    await user.type(search, "-missing");
    expect(screen.getByText("没有匹配的模型")).toBeVisible();
    expect(screen.queryByRole("navigation", { name: "表格分页" })).not.toBeInTheDocument();
    await user.clear(search);
    expect(screen.getByRole("button", { name: "查看 image-test 明细" })).toBeVisible();
  });

  it("打开模型时显示中文能力和上下文信息，其他字段按需展开且可以返回列表", async () => {
    const user = userEvent.setup();
    render(<RawPricingSourceContent source={source} pending={false} error="" />);
    await user.click(screen.getByRole("button", { name: "查看 gpt-test 明细" }));
    expect(screen.getByText("最大输入 Token")).toBeVisible();
    expect(screen.getByText("128,000")).toBeVisible();
    expect(screen.getByText("视觉输入")).toBeVisible();
    expect(screen.getByText("支持", { exact: true })).toBeVisible();
    await user.click(screen.getByText("其他字段（1）", { exact: true }));
    expect(await screen.findByRole("textbox", { name: "其他字段 JSON" })).toHaveTextContent(
      '"region": "test"',
    );
    await user.click(screen.getByRole("button", { name: "返回模型列表" }));
    expect(screen.getByRole("button", { name: "查看 image-test 明细" })).toBeVisible();
  });

  it("图像模型没有 Token 价格时显示未提供，详情保留每张图像的单位", async () => {
    const user = userEvent.setup();
    render(<RawPricingSourceContent source={source} pending={false} error="" />);
    const row = screen.getByRole("row", { name: /image-test/ });
    expect(within(row).getAllByText("未提供")).toHaveLength(4);
    await user.click(screen.getByRole("button", { name: "查看 image-test 明细" }));
    expect(screen.getByText("图像输出（每张）")).toBeVisible();
    expect(screen.getByText("0.04", { exact: true })).toBeVisible();
  });

  it("模型超过一页时分页，搜索后从第一页展示匹配结果", async () => {
    const user = userEvent.setup();
    const entries = Object.fromEntries(
      Array.from({ length: 25 }, (_, i) => [
        `model-${String(i).padStart(2, "0")}`,
        { input_cost_per_token: 1e-6 },
      ]),
    );
    render(
      <RawPricingSourceContent
        source={{ ...source, content: JSON.stringify(entries) }}
        pending={false}
        error=""
      />,
    );
    expect(screen.getAllByRole("button", { name: /^查看 model-/ })).toHaveLength(20);
    await user.click(screen.getByRole("button", { name: "转到下一页" }));
    expect(screen.getByRole("button", { name: "查看 model-24 明细" })).toBeVisible();
    await user.type(screen.getByRole("textbox", { name: "搜索模型或厂商" }), "model-0");
    expect(screen.getByRole("button", { name: "查看 model-00 明细" })).toBeVisible();
    expect(screen.getByRole("button", { name: "转到上一页" })).toBeDisabled();
  });

  it("键盘切换原始 JSON 时保留文件原文，文件信息默认收起", async () => {
    const user = userEvent.setup();
    render(<RawPricingSourceContent source={source} pending={false} error="" />);
    screen.getByRole("tab", { name: "模型明细" }).focus();
    await user.keyboard("{ArrowRight}");
    expect(screen.getByRole("tab", { name: "原始 JSON" })).toHaveFocus();
    expect(await screen.findByRole("textbox", { name: "原始价卡内容" })).toHaveAttribute(
      "aria-readonly",
      "true",
    );
    const write = vi.spyOn(navigator.clipboard, "writeText").mockResolvedValue();
    await user.click(screen.getByRole("button", { name: "复制内容" }));
    expect(write).toHaveBeenCalledWith(content);
    const metadata = screen.getByText("文件信息").closest("details");
    expect(metadata).not.toHaveAttribute("open");
    await user.click(screen.getByText("文件信息"));
    expect(screen.getByText("abc123")).toBeVisible();
  });

  it("复制原始文件时保留科学计数法和所有空白", async () => {
    const user = userEvent.setup();
    const write = vi.spyOn(navigator.clipboard, "writeText").mockResolvedValue();
    render(<RawPricingSourceContent source={source} pending={false} error="" />);
    await user.click(screen.getByRole("button", { name: "复制原始文件" }));
    expect(write).toHaveBeenCalledWith(content);
  });

  it("剪贴板拒绝写入时显示可操作的失败提示", async () => {
    const user = userEvent.setup();
    vi.spyOn(navigator.clipboard, "writeText").mockRejectedValue(new Error("denied"));
    render(
      <>
        <Toaster />
        <RawPricingSourceContent source={source} pending={false} error="" />
      </>,
    );
    await user.click(screen.getByRole("button", { name: "复制原始文件" }));
    expect(await screen.findByText("复制失败，请允许剪贴板权限或下载原始文件")).toBeVisible();
  });

  it("文件无法解析时自动显示完整原文并禁用模型明细视图", async () => {
    const raw = "invalid JSON\n";
    render(
      <RawPricingSourceContent source={{ ...source, content: raw }} pending={false} error="" />,
    );
    expect(screen.getByRole("tab", { name: "模型明细" })).toBeDisabled();
    expect(await screen.findByRole("textbox", { name: "原始价卡内容" })).toHaveTextContent(
      "invalid JSON",
    );
  });

  it("价卡为空对象时显示空状态并保留原文入口", () => {
    render(
      <RawPricingSourceContent source={{ ...source, content: "{}" }} pending={false} error="" />,
    );
    expect(screen.getByText("价卡中暂无模型")).toBeVisible();
    expect(screen.queryByRole("navigation", { name: "表格分页" })).not.toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "原始 JSON" })).toBeEnabled();
  });

  it("后台刷新时保留正在查看的模型列表", () => {
    render(<RawPricingSourceContent source={source} pending error="" />);
    expect(screen.getByRole("button", { name: "查看 gpt-test 明细" })).toBeVisible();
  });

  it("模型没有字段时展示明确的详情空状态", async () => {
    const user = userEvent.setup();
    render(
      <RawPricingSourceContent
        source={{ ...source, content: '{"empty-model":{}}' }}
        pending={false}
        error=""
      />,
    );
    await user.click(screen.getByRole("button", { name: "查看 empty-model 明细" }));
    expect(screen.getByText("该模型暂无字段")).toBeVisible();
  });

  it("模型含特殊 JSON 键名时保留未知字段和原始模型类型", async () => {
    const user = userEvent.setup();
    render(
      <RawPricingSourceContent
        source={{
          ...source,
          content: '{"test":{"mode":"constructor","__proto__":{"note":"保留内容"}}}',
        }}
        pending={false}
        error=""
      />,
    );
    await user.click(screen.getByRole("button", { name: "查看 test 明细" }));
    expect(screen.getByText("constructor", { exact: true })).toBeVisible();
    await user.click(screen.getByText("其他字段（1）", { exact: true }));
    expect(await screen.findByRole("textbox", { name: "其他字段 JSON" })).toHaveTextContent(
      '"note": "保留内容"',
    );
  });

  it("首次读取期间显示忙碌状态", () => {
    render(<RawPricingSourceContent pending error="" />);
    expect(screen.getByRole("status", { name: "正在读取远程价卡原始文件" })).toHaveAttribute(
      "aria-busy",
      "true",
    );
  });

  it("读取失败时提供重试且不重复展示错误", async () => {
    const user = userEvent.setup();
    const retry = vi.fn();
    render(<RawPricingSourceContent pending={false} error="读取失败" onRetry={retry} />);
    expect(screen.queryByText("读取失败")).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "重新读取" }));
    expect(retry).toHaveBeenCalledOnce();
  });
});
