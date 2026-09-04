import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

import {
  filterRemoteModelPrices,
  modelPriceDifferenceRows,
  NewAPIModelPrices,
  newAPIPriceComparisonStatus,
  RemoteModelPricesTable,
  remotePriceToNewAPIModelPrice,
  WrittenModelPriceResult,
} from "../model-prices";

describe("远程模型价格", () => {
  it("作为顶级页签展示且不再使用弹窗入口", () => {
    const markup = renderToStaticMarkup(
      <NewAPIModelPrices models={[]} onViewManagementPrices={vi.fn()} />,
    );

    expect(markup).toContain(">远程模型价格</button>");
    expect(markup).not.toContain("查看远程模型价格");
    expect(markup).not.toContain("查看管理平台价格");
  });

  it("默认每页显示十条并提供翻页控件", () => {
    const prices = Array.from({ length: 11 }, (_, index) => ({
      model: `remote-model-${String(index + 1).padStart(2, "0")}`,
      input_price: "0.000001",
      output_price: "0.000002",
      model_ratio: "0.5",
      completion_ratio: "2",
    }));
    const markup = renderToStaticMarkup(
      <RemoteModelPricesTable prices={prices} pending={false} error="" />,
    );

    expect(markup).toContain("remote-model-10");
    expect(markup).not.toContain("remote-model-11");
    expect(markup).toContain('aria-label="转到下一页"');
    expect(markup).toContain(">11</span>");
  });

  it("支持按主表模型名称查询远程价格", () => {
    const prices = [
      {
        model: "gpt-5",
        input_price: "0.000001",
        output_price: "0.000002",
        model_ratio: "0.5",
        completion_ratio: "2",
      },
      {
        model: "claude-sonnet-4",
        input_price: "0.000003",
        output_price: "0.000015",
        model_ratio: "1.5",
        completion_ratio: "5",
      },
    ];

    expect(filterRemoteModelPrices(prices, " GPT-5 ")).toEqual([prices[0]]);
  });

  it("为远程价格提供写入 New API 操作", () => {
    const markup = renderToStaticMarkup(
      <RemoteModelPricesTable
        prices={[
          {
            model: "gpt-5",
            input_price: "0.000001",
            output_price: "0.000008",
            model_ratio: "0.5",
            completion_ratio: "8",
          },
        ]}
        pending={false}
        error=""
        onWritePrice={vi.fn()}
      />,
    );

    expect(markup).toContain("操作");
    expect(markup).toContain("写入 New API");
  });

  it("没有 Token 输入基价时不提供错误的写入操作", () => {
    const markup = renderToStaticMarkup(
      <RemoteModelPricesTable
        prices={[
          {
            model: "image-per-request-only",
            input_price: "",
            output_price: "",
            image_output_price: "0.04",
            model_ratio: "",
            completion_ratio: "",
          },
        ]}
        pending={false}
        error=""
        onWritePrice={vi.fn()}
      />,
    );

    expect(markup).toContain("暂不支持写入");
    expect(markup).toContain("disabled");
    expect(markup).not.toContain(">写入 New API</button>");
  });

  it("将远程价格完整转换为 New API 倍率配置", () => {
    expect(
      remotePriceToNewAPIModelPrice({
        model: "claude-sonnet-4",
        input_price: "0.000003",
        output_price: "0.000015",
        image_input_price: "0.000006",
        cache_write_price: "0.00000375",
        cache_write_1h_price: "0.000006",
        cache_read_price: "3e-7",
        model_ratio: "1.5",
        completion_ratio: "5",
        cache_ratio: "0.1",
        create_cache_ratio: "1.25",
        create_cache_1h_ratio: "2",
        image_ratio: "2",
      }),
    ).toEqual({
      model: "claude-sonnet-4",
      input_ratio: "1.5",
      completion_ratio: "5",
      cache_ratio: "0.1",
      create_cache_ratio: "1.25",
      image_ratio: "2",
      billing_mode: "tiered_expr",
      billing_expr: 'tier("base", p * 3 + c * 15 + cr * 0.3 + cc * 3.75 + cc1h * 6 + img * 6)',
    });
  });

  it("将远程长上下文价格转换为 New API 阶梯表达式", () => {
    const remotePrice = {
      model: "gpt-5.6-sol",
      input_price: "0.000005",
      output_price: "0.00003",
      cache_write_price: "0.00000625",
      cache_read_price: "0.0000005",
      model_ratio: "2.5",
      completion_ratio: "6",
      long_context_threshold: 272000,
      long_context_input_price: "0.00001",
      long_context_output_price: "0.000045",
      long_context_cache_write_price: "0.0000125",
      long_context_cache_read_price: "0.000001",
    };

    const converted = remotePriceToNewAPIModelPrice(remotePrice);
    expect(converted.billing_mode).toBe("tiered_expr");
    expect(converted.billing_expr).toBe(
      'len <= 272000 ? tier("standard", p * 5 + c * 30 + cr * 0.5 + cc * 6.25) : tier("long_context", p * 10 + c * 45 + cr * 1 + cc * 12.5)',
    );
    const configuredPrice = {
      model: "gpt-5.6-sol",
      input_ratio: "2.5",
      completion_ratio: "6",
      billing_mode: "tiered_expr",
      billing_expr:
        'len <= 272000 ? tier("standard", p * 5 + c * 30 + cr * 0.5 + cc * 6.25) : tier("long_context", p * 10 + c * 45 + cr * 1 + cc * 12.5)',
    };
    expect(newAPIPriceComparisonStatus(configuredPrice, [remotePrice])).toBe("matched");
    expect(modelPriceDifferenceRows(configuredPrice, remotePrice)).toEqual(
      expect.arrayContaining([
        {
          label: "输入价格（$/百万 Token）",
          configured: "5 / 10",
          remote: "5 / 10",
          matched: true,
        },
        {
          label: "输出价格（$/百万 Token）",
          configured: "30 / 45",
          remote: "30 / 45",
          matched: true,
        },
        {
          label: "缓存写入（$/百万 Token）",
          configured: "6.25 / 12.5",
          remote: "6.25 / 12.5",
          matched: true,
        },
        {
          label: "缓存读取（$/百万 Token）",
          configured: "0.5 / 1",
          remote: "0.5 / 1",
          matched: true,
        },
      ]),
    );
  });

  it("xAI 达到长上下文阈值时进入高档", () => {
    const remotePrice = {
      model: "grok-tier",
      input_price: "0.000002",
      output_price: "0.000006",
      model_ratio: "1",
      completion_ratio: "3",
      long_context_threshold: 200000,
      long_context_threshold_inclusive: true,
      long_context_input_price: "0.000004",
      long_context_output_price: "0.000012",
    };
    expect(remotePriceToNewAPIModelPrice(remotePrice).billing_expr).toBe(
      'len < 200000 ? tier("standard", p * 2 + c * 6) : tier("long_context", p * 4 + c * 12)',
    );
    const wrongBoundary = {
      model: "grok-tier",
      input_ratio: "1",
      completion_ratio: "3",
      billing_mode: "tiered_expr",
      billing_expr:
        'len <= 200000 ? tier("standard", p * 2 + c * 6) : tier("long_context", p * 4 + c * 12)',
    };
    expect(newAPIPriceComparisonStatus(wrongBoundary, [remotePrice])).toBe("mismatched");
    expect(modelPriceDifferenceRows(wrongBoundary, remotePrice)).toContainEqual({
      label: "阶梯条件",
      configured: "len <= 200000",
      remote: "len < 200000",
      matched: false,
    });
  });

  it("在远程价格列表显示长上下文的两档价格", () => {
    const markup = renderToStaticMarkup(
      <RemoteModelPricesTable
        prices={[
          {
            model: "gpt-5.6-sol",
            input_price: "0.000005",
            output_price: "0.00003",
            cache_write_price: "0.00000625",
            cache_read_price: "0.0000005",
            model_ratio: "2.5",
            completion_ratio: "6",
            long_context_threshold: 272000,
            long_context_input_price: "0.00001",
            long_context_output_price: "0.000045",
            long_context_cache_write_price: "0.0000125",
            long_context_cache_read_price: "0.000001",
          },
        ]}
        pending={false}
        error=""
      />,
    );

    expect(markup).toContain("272K");
    expect(markup).toContain("5 / 10");
    expect(markup).toContain("30 / 45");
    expect(markup).toContain("6.25 / 12.5");
    expect(markup).toContain("0.5 / 1");
  });

  it("写入后回显 New API 实际读回的完整价格", () => {
    const expression = 'tier("base", p * 10 + c * 50 + cr * 0.25 + cc * 12.5 + cc1h * 20)';
    const markup = renderToStaticMarkup(
      <WrittenModelPriceResult
        price={{
          model: "claude-fable-5-1",
          input_ratio: "5",
          completion_ratio: "5",
          cache_ratio: "0.025",
          create_cache_ratio: "1.25",
          billing_mode: "tiered_expr",
          billing_expr: expression,
        }}
      />,
    );

    expect(markup).toContain("New API 读回结果");
    expect(markup).toContain("输入价格");
    expect(markup).toContain(">10</");
    expect(markup).toContain(">50</");
    expect(markup).toContain(">0.25</");
    expect(markup).toContain(">12.5</");
    expect(markup).toContain(">20</");
    expect(markup).toContain(expression.replaceAll('"', "&quot;"));
  });

  it("claude-fable-5 使用自身的缓存读取价格并比较一小时缓存写入", () => {
    const remotePrice = {
      model: "claude-fable-5",
      input_price: "0.00001",
      output_price: "0.00005",
      cache_read_price: "0.000001",
      cache_write_price: "0.0000125",
      cache_write_1h_price: "0.00002",
      model_ratio: "5",
      completion_ratio: "5",
      cache_ratio: "0.1",
      create_cache_ratio: "1.25",
      create_cache_1h_ratio: "2",
    };

    const configuredPrice = remotePriceToNewAPIModelPrice(remotePrice);
    expect(configuredPrice.billing_expr).toBe(
      'tier("base", p * 10 + c * 50 + cr * 1 + cc * 12.5 + cc1h * 20)',
    );
    expect(newAPIPriceComparisonStatus(configuredPrice, [remotePrice])).toBe("matched");
    expect(modelPriceDifferenceRows(configuredPrice, remotePrice)).toEqual(
      expect.arrayContaining([
        { label: "计费方式", configured: "阶梯", remote: "阶梯", matched: true },
        {
          label: "缓存读取（$/百万 Token）",
          configured: "1",
          remote: "1",
          matched: true,
        },
        {
          label: "缓存写入（1h）（$/百万 Token）",
          configured: "20",
          remote: "20",
          matched: true,
        },
      ]),
    );
  });

  it("比较 New API 与远程价卡支持一致、不一致和远程缺失状态", () => {
    const remotePrices = [
      {
        model: "gpt-5",
        input_price: "0.000001",
        output_price: "0.000008",
        model_ratio: "0.5",
        completion_ratio: "8",
        cache_ratio: "0.1",
      },
      {
        model: "claude-sonnet-4",
        input_price: "0.000003",
        output_price: "0.000015",
        model_ratio: "1.5",
        completion_ratio: "5",
      },
    ];

    expect(
      newAPIPriceComparisonStatus(
        {
          model: "gpt-5",
          input_ratio: "0.50",
          completion_ratio: "8.0",
          cache_ratio: "0.10",
        },
        remotePrices,
      ),
    ).toBe("matched");
    expect(
      newAPIPriceComparisonStatus(
        { model: "claude-sonnet-4", input_ratio: "1.5", completion_ratio: "4" },
        remotePrices,
      ),
    ).toBe("mismatched");
    expect(
      newAPIPriceComparisonStatus(
        { model: "missing-model", input_ratio: "1", completion_ratio: "1" },
        remotePrices,
      ),
    ).toBe("missing");

    const tieredRemote = {
      model: "claude-fable-5-1",
      input_price: "0.00001",
      output_price: "0.00005",
      cache_read_price: "0.00000025",
      cache_write_price: "0.0000125",
      cache_write_1h_price: "0.00002",
      model_ratio: "5",
      completion_ratio: "5",
      cache_ratio: "0.025",
      create_cache_ratio: "1.25",
      create_cache_1h_ratio: "2",
    };
    expect(
      newAPIPriceComparisonStatus(remotePriceToNewAPIModelPrice(tieredRemote), [tieredRemote]),
    ).toBe("matched");
  });

  it("比较时忽略倍率换算产生的价格尾差", () => {
    const remotePrice = {
      model: "glm-5.1",
      input_price: "0.0000014",
      output_price: "0.000004399999999998",
      model_ratio: "0.7",
      completion_ratio: "3.142857142855714",
    };

    expect(
      newAPIPriceComparisonStatus(
        {
          model: "glm-5.1",
          input_ratio: "0.7",
          completion_ratio: "3.142857142857143",
        },
        [remotePrice],
      ),
    ).toBe("matched");
    expect(
      modelPriceDifferenceRows(remotePriceToNewAPIModelPrice(remotePrice), remotePrice),
    ).toEqual(
      expect.arrayContaining([
        {
          label: "输出价格（$/百万 Token）",
          configured: "4.4",
          remote: "4.4",
          matched: true,
        },
      ]),
    );
  });

  it("生成当前 New API 与远程价格的逐项差异", () => {
    const rows = modelPriceDifferenceRows(
      {
        model: "claude-sonnet-4",
        input_ratio: "1.5",
        completion_ratio: "4",
        cache_ratio: "0.1",
      },
      {
        model: "claude-sonnet-4",
        input_price: "0.000003",
        output_price: "0.000015",
        model_ratio: "1.5",
        completion_ratio: "5",
        cache_ratio: "0.1",
      },
    );

    expect(rows.find((row) => row.label.startsWith("输入价格"))).toMatchObject({
      configured: "3",
      remote: "3",
      matched: true,
    });
    expect(rows.find((row) => row.label.startsWith("输出价格"))).toMatchObject({
      configured: "12",
      remote: "15",
      matched: false,
    });
  });
});
