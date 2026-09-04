import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import type { NewAPIRemoteSnapshot } from "@/api";
import {
  compareConfiguredWithModelPlaza,
  modelPriceColumnValues,
  NewAPIModelPrices,
  NewAPIPriceDifferences,
} from "../model-prices";

describe("New API 价格差异", () => {
  it("按次模型显示 ModelPrice 并保持其他价格列为空", () => {
    expect(
      modelPriceColumnValues({
        model: "gpt-image-2",
        model_price: "0.06",
        input_ratio: "",
        completion_ratio: "",
        billing_mode: "per-request",
      }),
    ).toEqual({
      input: "0.06",
      output: "",
      cacheCreate: "",
      cacheCreate1h: "",
      cacheRead: "",
      imageInput: "",
      imageOutput: "",
      audioInput: "",
      audioOutput: "",
    });
  });

  it("价格差异表回显按次价格", () => {
    const model = {
      model: "gpt-image-2",
      model_price: "0.06",
      input_ratio: "",
      completion_ratio: "",
      billing_mode: "per-request",
    };
    const snapshot: NewAPIRemoteSnapshot = {
      groups: [],
      models: [model],
      references: [model],
      unset_models: [],
      tool_prices: [],
      differences: [],
      fetched_at: "2026-09-04T00:00:00Z",
    };

    const markup = renderToStaticMarkup(<NewAPIPriceDifferences snapshot={snapshot} />);

    expect(markup).toContain("gpt-image-2");
    expect(markup).toContain("按次");
    expect(markup).toContain(">0.06</div>");
  });

  it("从表达式基础档位读取输入输出和缓存价格", () => {
    expect(
      modelPriceColumnValues({
        model: "claude-fable-5",
        input_ratio: "",
        completion_ratio: "",
        billing_mode: "tiered_expr",
        billing_expr: 'tier("base", p * 10 + c * 50 + cr * 1 + cc * 12.5 + cc1h * 20)',
      }),
    ).toEqual({
      input: "10",
      output: "50",
      cacheCreate: "12.5",
      cacheCreate1h: "20",
      cacheRead: "1",
      imageInput: "",
      imageOutput: "",
      audioInput: "",
      audioOutput: "",
    });
  });

  it("模型价格主表回显表达式中的输入输出和缓存价格", () => {
    const markup = renderToStaticMarkup(
      <NewAPIModelPrices
        models={[
          {
            model: "claude-fable-5",
            input_ratio: "",
            completion_ratio: "",
            billing_mode: "tiered_expr",
            billing_expr: 'tier("base", p * 10 + c * 50 + cr * 1 + cc * 12.5 + cc1h * 20)',
          },
        ]}
      />,
    );

    expect(markup).toContain('aria-label="claude-fable-5 输入价格：10"');
    expect(markup).toContain('aria-label="claude-fable-5 输出价格：50"');
    expect(markup).toContain('aria-label="claude-fable-5 缓存创建价格：普通 12.5，1 小时 20"');
    expect(markup).toContain('aria-label="claude-fable-5 缓存读取价格：1"');
  });

  it("空倍率不显示为零", () => {
    expect(
      modelPriceColumnValues({
        model: "unset-model",
        input_ratio: "",
        completion_ratio: "",
      }),
    ).toEqual({
      input: "",
      output: "",
      cacheCreate: "",
      cacheCreate1h: "",
      cacheRead: "",
      imageInput: "",
      imageOutput: "",
      audioInput: "",
      audioOutput: "",
    });
  });

  it("价格换算结果四舍五入并去除浮点尾差", () => {
    expect(
      modelPriceColumnValues({
        model: "glm-5.1",
        input_ratio: "0.7",
        completion_ratio: "1",
        completion_price: "4.399999999998",
        cache_create_price: "1.000000000004",
        cache_read_price: "0.279999999999997",
      }),
    ).toMatchObject({
      input: "1.4",
      output: "4.4",
      cacheCreate: "1",
      cacheRead: "0.28",
    });
  });

  it("多档表达式按档位顺序显示不同价格", () => {
    expect(
      modelPriceColumnValues({
        model: "tiered-model",
        input_ratio: "",
        completion_ratio: "",
        billing_mode: "tiered_expr",
        billing_expr:
          'len <= 1000 ? tier("short", p * 2.5 + c * 15 + cr * 0.25) : tier("long", p * 5 + c * 22.5 + cr * 0.5)',
      }),
    ).toEqual({
      input: "2.5 / 5",
      output: "15 / 22.5",
      cacheCreate: "",
      cacheCreate1h: "",
      cacheRead: "0.25 / 0.5",
      imageInput: "",
      imageOutput: "",
      audioInput: "",
      audioOutput: "",
    });
  });

  it("价格差异表为多档价格显示对应档位名称", () => {
    const expression =
      'len <= 272000 ? tier("standard", p * 0.2 + c * 1.2 + cr * 0.02 + cc * 0.25) : tier("long_context", p * 0.4 + c * 1.8 + cr * 0.04 + cc * 0.5)';
    const model = {
      model: "gpt-5.6-luna",
      input_ratio: "",
      completion_ratio: "",
      billing_mode: "tiered_expr",
      billing_expr: expression,
    };
    const snapshot: NewAPIRemoteSnapshot = {
      groups: [],
      models: [model],
      references: [model],
      unset_models: [],
      tool_prices: [],
      differences: [],
      fetched_at: "2026-09-04T00:00:00Z",
    };

    const markup = renderToStaticMarkup(<NewAPIPriceDifferences snapshot={snapshot} />);

    expect(markup).toContain("阶梯计费 · 2 档");
    expect(markup).toContain("standard");
    expect(markup).toContain("long_context");
    expect(markup.match(/data-slot="tier-labels"/g)).toHaveLength(1);
    expect(markup.match(/data-slot="tier-price-values"/g)).toHaveLength(4);
    expect(markup.match(/<div[^>]*data-slot="tooltip-trigger"[^>]*>standard<\/div>/g)).toHaveLength(
      1,
    );
    expect(
      markup.match(/<div[^>]*data-slot="tooltip-trigger"[^>]*>long_context<\/div>/g),
    ).toHaveLength(1);
    expect(markup).not.toContain(">普通</span> 0.25");
    expect(markup).not.toContain("0.2 / 0.4");
  });

  it("价格差异表不显示原始表达式并区分普通和一小时缓存写入价格", () => {
    const expression = 'tier("base", p * 10 + c * 50 + cr * 1 + cc * 12.5 + cc1h * 20)';
    const model = {
      model: "claude-fable-5",
      input_ratio: "",
      completion_ratio: "",
      billing_mode: "tiered_expr",
      billing_expr: expression,
    };
    const snapshot: NewAPIRemoteSnapshot = {
      groups: [],
      models: [model],
      references: [model],
      unset_models: [],
      tool_prices: [],
      differences: [],
      fetched_at: "2026-09-04T00:00:00Z",
    };

    const markup = renderToStaticMarkup(<NewAPIPriceDifferences snapshot={snapshot} />);

    expect(markup).toContain("阶梯");
    expect(markup).toContain('aria-label="claude-fable-5 缓存创建价格：普通 12.5，1 小时 20"');
    expect(markup).toContain(">普通</span> 12.5");
    expect(markup).toContain(">1 小时</span> 20");
    expect(markup).not.toContain(expression);
  });

  it("按 New API 规则把媒体倍率换算成价格", () => {
    expect(
      modelPriceColumnValues({
        model: "media-model",
        input_ratio: "2",
        completion_ratio: "3",
        image_ratio: "0.5",
        audio_ratio: "2",
        audio_completion_ratio: "4",
      }),
    ).toEqual({
      input: "4",
      output: "12",
      cacheCreate: "",
      cacheCreate1h: "",
      cacheRead: "",
      imageInput: "2",
      imageOutput: "",
      audioInput: "8",
      audioOutput: "32",
    });
  });

  it("只展示本平台配置与本平台价格参考的差异", () => {
    const snapshot: NewAPIRemoteSnapshot = {
      groups: [],
      models: [
        {
          model: "gpt-5",
          input_ratio: "0.8",
          completion_ratio: "2",
          input_price: "0.8",
          completion_price: "2",
          cache_create_price: "0.2",
          cache_read_price: "0.1",
          cache_ratio: "0.1",
          create_cache_ratio: "0.2",
          create_cache_1h_ratio: "0.3",
          image_ratio: "0.4",
          audio_ratio: "0.5",
          audio_completion_ratio: "0.6",
        },
      ],
      unset_models: [],
      tool_prices: [],
      references: [
        {
          model: "gpt-5",
          input_ratio: "0.9",
          completion_ratio: "2",
          input_price: "0.9",
          completion_price: "2",
          cache_create_price: "0.3",
          cache_read_price: "0.2",
          cache_ratio: "0.2",
          create_cache_ratio: "0.3",
          create_cache_1h_ratio: "0.4",
          image_ratio: "0.5",
          audio_ratio: "0.6",
          audio_completion_ratio: "0.7",
        },
      ],
      newapi_models: [
        {
          model: "gpt-5",
          input_ratio: "0.8",
          completion_ratio: "2",
          cache_ratio: "0.1",
          create_cache_ratio: "1.25",
        },
      ],
      upstream_prices: [
        {
          host: "upstream.example",
          name: "Sub2API",
          upstream_type: "sub2api",
          models: [
            {
              model: "gpt-5",
              input_ratio: "0.9",
              completion_ratio: "2",
              cache_ratio: "0.2",
              create_cache_ratio: "1.25",
            },
          ],
        },
      ],
      differences: [
        {
          model: "gpt-5",
          kind: "ratio_mismatch",
          configured: {
            model: "gpt-5",
            input_ratio: "0.8",
            completion_ratio: "2",
            cache_ratio: "0.1",
            create_cache_ratio: "0.2",
            create_cache_1h_ratio: "0.3",
            image_ratio: "0.4",
            audio_ratio: "0.5",
            audio_completion_ratio: "0.6",
          },
          reference: {
            model: "gpt-5",
            input_ratio: "0.9",
            completion_ratio: "2",
            cache_ratio: "0.2",
            create_cache_ratio: "0.3",
            create_cache_1h_ratio: "0.4",
            image_ratio: "0.5",
            audio_ratio: "0.6",
            audio_completion_ratio: "0.7",
          },
        },
      ],
      fetched_at: "2026-09-03T00:00:00Z",
    };

    const markup = renderToStaticMarkup(<NewAPIPriceDifferences snapshot={snapshot} />);

    expect(markup).toContain("输入价格");
    expect(markup).toContain("输出价格");
    expect(markup).toContain("缓存创建");
    expect(markup).toContain("缓存读取");
    expect(markup).not.toContain("计费方式 / 表达式");
    expect(markup).toContain("按 Token");
    expect(markup).not.toContain("选择比较上游");
    expect(markup).not.toContain("Sub2API 上游输入");
    expect(markup).not.toContain("需要处理");
    expect(markup).not.toContain("已同步");
    expect(markup).not.toContain("倍率不同");
    expect(markup).toContain("0.8");
    expect(markup).not.toContain("0.9");
  });

  it("只返回本平台已有模型并标记缓存倍率差异", () => {
    const differences = compareConfiguredWithModelPlaza(
      [{ model: "in-platform", input_ratio: "1", completion_ratio: "2", cache_ratio: "0.1" }],
      [
        { model: "in-platform", input_ratio: "1", completion_ratio: "2", cache_ratio: "0.2" },
        { model: "model-plaza-only", input_ratio: "9", completion_ratio: "9" },
      ],
    );
    expect(differences).toHaveLength(1);
    expect(differences[0].model).toBe("in-platform");
  });

  it("平台配置有 33 个模型时严格按配置显示 33 项", () => {
    const models = Array.from({ length: 33 }, (_, index) => ({
      model: `model-${String(index + 1).padStart(2, "0")}`,
      input_ratio: "1",
      completion_ratio: "2",
    }));
    const snapshot: NewAPIRemoteSnapshot = {
      groups: [],
      models,
      references: models.slice(0, 31),
      unset_models: [],
      tool_prices: [],
      differences: [],
      fetched_at: "2026-09-04T00:00:00Z",
    };
    const markup = renderToStaticMarkup(<NewAPIPriceDifferences snapshot={snapshot} />);

    expect(markup).toContain(">33</span>");
    expect(markup).toContain("model-01");
    expect(markup).toContain("model-20");
    expect(markup).not.toContain("model-21");
    expect(markup).toContain('aria-label="转到下一页"');
  });
});
