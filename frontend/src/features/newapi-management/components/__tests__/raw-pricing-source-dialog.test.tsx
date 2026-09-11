import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

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

  it("长正文让内部滚动区占满弹窗正文的可用高度", () => {
    const markup = renderToStaticMarkup(
      <RawPricingSourceContent
        source={{
          source_url: "https://raw.example/model-prices.json",
          content: Array.from({ length: 200 }, (_, index) => `line-${index}`).join("\n"),
          fetched_at: "2026-09-04T12:30:00Z",
          size_bytes: 2048,
          sha256: "abc123",
        }}
        pending={false}
        error=""
      />,
    );

    expect(markup).toMatch(/class="[^"]*h-full[^"]*grid-rows-\[auto_minmax\(0,1fr\)\][^"]*"/);
    expect(markup).toMatch(
      /<pre class="[^"]*min-h-0[^"]*overflow-auto[^"]*" data-slot="raw-pricing-source"/,
    );
  });

  it("读取期间显示明确状态", () => {
    const markup = renderToStaticMarkup(
      <RawPricingSourceContent source={undefined} pending error="" />,
    );

    expect(markup).toContain('role="status"');
    expect(markup).toContain('aria-busy="true"');
    expect(markup).toContain('aria-label="正在读取远程价卡原始文件"');
    expect(markup).toContain("正在读取远程价卡原始文件");
  });
});

it("原始价卡读取失败提供重试且不显示空数据结论", async () => {
  const retry = vi.fn();
  render(<RawPricingSourceContent pending={false} error="读取失败" onRetry={retry} />);
  expect(screen.queryByText("尚未读取到原始价卡")).not.toBeInTheDocument();
  await userEvent.setup().click(screen.getByRole("button", { name: "重新读取" }));
  expect(retry).toHaveBeenCalledOnce();
});
