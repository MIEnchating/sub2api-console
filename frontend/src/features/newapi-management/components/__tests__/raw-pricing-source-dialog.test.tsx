import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { RawPricingSourceContent } from "../raw-pricing-source-dialog";

describe("远程价卡原始文件", () => {
  it("原样显示后端返回的正文及校验信息", () => {
    const content = '{\n  "gpt-test": {"input_cost_per_token": 1e-6}\n}\n';
    const markup = renderToStaticMarkup(
      <RawPricingSourceContent
        source={{
          source_url: "https://raw.example/model-prices.json",
          content,
          fetched_at: "2026-09-04T12:30:00Z",
          size_bytes: 56,
          sha256: "abc123",
        }}
        pending={false}
        error=""
      />,
    );

    expect(markup).toContain("后端实际拉取的原始内容");
    expect(markup).toContain("https://raw.example/model-prices.json");
    expect(markup).toContain("abc123");
    expect(markup).toContain("gpt-test");
    expect(markup).toContain("input_cost_per_token");
    expect(markup).not.toContain("JSON.stringify");
  });

  it("读取期间显示明确状态", () => {
    const markup = renderToStaticMarkup(
      <RawPricingSourceContent source={undefined} pending error="" />,
    );

    expect(markup).toContain('role="status"');
    expect(markup).toContain("正在读取远程价卡原始文件");
  });
});
