import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { AccountRecentResults } from "../account-recent-results";

describe("AccountRecentResults", () => {
  it("shows results in chronological order with shared tones and details", () => {
    const markup = renderToStaticMarkup(
      <AccountRecentResults
        results={[
          {
            result: "失败",
            event_type: "gateway_error",
            score: 25,
            observed_at: "2026-08-26T12:00:00Z",
            latency_ms: 1250,
            failure_reason: "上游网关错误",
            source: "traffic",
          },
          {
            result: "通过",
            event_type: "healthy",
            score: 100,
            observed_at: "2026-08-26T11:00:00Z",
            latency_ms: 320,
            failure_reason: null,
            source: "active-probe",
          },
        ]}
        sampleCount={12}
        showCount
      />,
    );

    expect(markup).toContain('data-slot="account-recent-results"');
    expect(markup).toContain("bg-success");
    expect(markup).toContain("bg-orange-500");
    expect(markup).toContain("主动探测");
    expect(markup).toContain("真实流量");
    expect(markup).toContain("网关错误 · 25 分");
    expect(markup).toContain("完美健康 · 100 分");
    expect(markup.indexOf("主动探测")).toBeLessThan(markup.indexOf("真实流量"));
    expect(markup).toContain("首字 1250ms");
    expect(markup).toContain("上游网关错误");
    expect(markup).toContain("12 条样本");
  });

  it("uses the shared empty state when no evidence exists", () => {
    const markup = renderToStaticMarkup(<AccountRecentResults results={[]} />);

    expect(markup).toContain('data-slot="account-recent-results"');
    expect(markup).toContain("暂无样本");
  });

  it("hides account-state placeholders instead of rendering a gray result", () => {
    const markup = renderToStaticMarkup(
      <AccountRecentResults
        results={[
          {
            result: "未取到日志",
            observed_at: null,
            latency_ms: null,
            failure_reason: "最近1分钟无账号使用日志，未调用官方测试接口",
            source: "account-state",
          },
        ]}
      />,
    );

    expect(markup).toContain("暂无样本");
    expect(markup).not.toContain("未取到日志");
    expect(markup).not.toContain("未调用官方测试接口");
    expect(markup).not.toContain("account-state");
    expect(markup).not.toContain("首字");
  });

  it("keeps real samples while removing an account-state placeholder", () => {
    const markup = renderToStaticMarkup(
      <AccountRecentResults
        results={[
          {
            result: "未取到日志",
            observed_at: null,
            latency_ms: null,
            failure_reason: "最近1分钟无账号使用日志",
            source: "account-state",
          },
          {
            result: "通过",
            observed_at: "2026-08-26T11:00:00Z",
            latency_ms: 320,
            failure_reason: null,
            source: "active-probe",
          },
        ]}
      />,
    );

    expect(markup).toContain("bg-success");
    expect(markup).not.toContain("bg-muted-foreground/35");
    expect(markup).not.toContain("未取到日志");
  });

  it("draws at most ten recent cells while preserving the scoring sample count", () => {
    const results = Array.from({ length: 12 }, (_, index) => ({
      result: "通过",
      observed_at: `2026-08-26T${String(index).padStart(2, "0")}:00:00Z`,
      latency_ms: 100 + index,
      failure_reason: null,
      source: "traffic",
    }));
    const markup = renderToStaticMarkup(
      <AccountRecentResults results={results} sampleCount={60} showCount />,
    );

    expect(markup.match(/tabindex="0"/g)).toHaveLength(10);
    expect(markup).toContain("60 条样本");
  });

  it("shows the actual sample count when the long window is not full", () => {
    const results = Array.from({ length: 8 }, (_, index) => ({
      result: "通过",
      observed_at: `2026-08-26T${String(index).padStart(2, "0")}:00:00Z`,
      latency_ms: 100 + index,
      failure_reason: null,
      source: "traffic",
    }));
    const markup = renderToStaticMarkup(
      <AccountRecentResults results={results} sampleCount={8} showCount />,
    );

    expect(markup.match(/tabindex="0"/g)).toHaveLength(8);
    expect(markup).toContain("8 条样本");
  });
});
