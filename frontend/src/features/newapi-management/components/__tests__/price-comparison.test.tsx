import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { Toaster, toast } from "sonner";

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

beforeEach(() => vi.stubGlobal("PointerEvent", MouseEvent));
afterEach(() => toast.dismiss());

describe("New API 价格比对", () => {
  it("上游价卡带有来源名称时展示来源并按稳定地址保留选中项", async () => {
    const user = userEvent.setup();
    const upstream = {
      ...snapshot.upstream_prices![0]!,
      name: "upstream.example.test（渠道价卡）",
    };
    const view = render(
      <NewAPIPriceComparison snapshot={{ ...snapshot, upstream_prices: [upstream] }} />,
    );

    const selector = screen.getByRole("combobox", { name: "比对上游" });
    await user.click(selector);
    await user.click(
      screen.getByRole("option", { name: "upstream.example.test（渠道价卡） · Sub2API" }),
    );
    view.rerender(
      <NewAPIPriceComparison
        snapshot={{
          ...snapshot,
          upstream_prices: [{ ...upstream, name: "更新后的渠道价卡" }],
        }}
      />,
    );

    expect(selector).toHaveTextContent("更新后的渠道价卡 · Sub2API");
    await user.click(screen.getByRole("button", { name: "比对 model-a" }));
    expect(screen.getByRole("dialog", { name: "model-a 价格比对" })).toBeInTheDocument();
    expect(screen.getByText("价格一致")).toBeInTheDocument();
  });

  it.each(["", "   "])("上游名称为 %j 时回退显示地址与平台类型", async (name) => {
    const user = userEvent.setup();
    render(
      <NewAPIPriceComparison
        snapshot={{
          ...snapshot,
          upstream_prices: [{ ...snapshot.upstream_prices![0]!, name }],
        }}
      />,
    );

    await user.click(screen.getByRole("combobox", { name: "比对上游" }));

    expect(
      screen.getByRole("option", { name: "upstream.example.test · Sub2API" }),
    ).toBeInTheDocument();
  });

  it("比对按 Token 计费的 New API 上游时展示输入、输出及缓存价格", async () => {
    const user = userEvent.setup();
    const tokenSnapshot: NewAPIRemoteSnapshot = {
      ...snapshot,
      upstream_prices: [
        {
          host: "newapi.example.test",
          name: "New API 上游",
          upstream_type: "newapi",
          models: [
            {
              model: "model-a",
              input_ratio: "1",
              completion_ratio: "2",
              input_price: "2",
              completion_price: "4",
              cache_read_price: "0.2",
            },
          ],
        },
      ],
    };
    render(<NewAPIPriceComparison snapshot={tokenSnapshot} />);

    await user.click(screen.getByRole("combobox", { name: "比对上游" }));
    await user.click(screen.getByRole("option", { name: "New API 上游 · New API" }));
    await user.click(screen.getByRole("button", { name: "比对 model-a" }));

    const dialog = within(screen.getByRole("dialog", { name: "model-a 价格比对" }));
    for (const [label, expected] of [
      ["计费方式", "按 Token"],
      ["输入价格", "2"],
      ["输出价格", "4"],
      ["缓存读取", "0.2"],
    ]) {
      const row = dialog.getByRole("row", { name: new RegExp(label!) });
      expect(within(row).getAllByRole("cell")[2]).toHaveTextContent(expected!);
    }
  });

  it("部分上游读取失败时悬浮提示原因并保留可用上游的比对操作", async () => {
    const user = userEvent.setup();
    const warning = "failed.example.test：上游鉴权失败（HTTP 401），请恢复该上游鉴权后刷新";
    const failedSnapshot = { ...snapshot, upstream_price_warning: warning };
    render(
      <>
        <Toaster />
        <NewAPIPriceComparison snapshot={failedSnapshot} />
      </>,
    );

    expect(await screen.findByText(warning)).toBeInTheDocument();
    expect(screen.getAllByText(warning)).toHaveLength(1);
    await user.click(screen.getByRole("combobox", { name: "比对上游" }));
    await user.click(screen.getByRole("option", { name: "上游 A · Sub2API" }));
    expect(screen.getByRole("button", { name: "比对 model-a" })).toBeEnabled();
  });

  it("选择上游和模型后执行批量比对并显示逐项结果", async () => {
    const user = userEvent.setup();
    render(<NewAPIPriceComparison snapshot={snapshot} />);

    const batchButton = screen.getByRole("button", { name: "批量比对" });
    expect(batchButton).toBeDisabled();

    await user.click(screen.getByRole("combobox", { name: "比对上游" }));
    await user.click(screen.getByRole("option", { name: "上游 A · Sub2API" }));
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
    await user.click(screen.getByRole("option", { name: "上游 A · Sub2API" }));
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

  it("搜索无匹配模型时显示空状态，同时保留比对概览", async () => {
    const user = userEvent.setup();
    render(<NewAPIPriceComparison snapshot={snapshot} />);

    expect(screen.getByRole("region", { name: "价格比对概览" })).toHaveTextContent("模型总数");
    await user.type(screen.getByRole("textbox", { name: "搜索比对模型" }), "不存在");

    expect(screen.getByText("没有匹配的模型，请调整搜索条件")).toBeInTheDocument();
    expect(screen.getByRole("region", { name: "价格比对概览" })).toBeInTheDocument();
  });

  it("按向下方向键打开上游列表并可通过 Tab 到达首个选项", async () => {
    const user = userEvent.setup();
    render(<NewAPIPriceComparison snapshot={snapshot} />);

    const selector = screen.getByRole("combobox", { name: "比对上游" });
    selector.focus();
    await user.keyboard("{ArrowDown}");

    expect(selector).toHaveAttribute("aria-expanded", "true");
    await user.tab();
    expect(screen.getByRole("option", { name: "上游 A · Sub2API" })).toHaveFocus();
  });
});
