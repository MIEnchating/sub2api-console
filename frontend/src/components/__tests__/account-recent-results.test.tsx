import { render, screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import type { AccountRecentResult } from "@/api";
import { AccountRecentResults } from "../account-recent-results";

const probe: AccountRecentResult = {
  result: "通过",
  event_type: "healthy",
  score: 100,
  observed_at: "2026-08-26T11:00:00Z",
  latency_ms: 320,
  failure_reason: null,
  source: "active-probe",
};
const traffic: AccountRecentResult = {
  result: "失败",
  event_type: "gateway_error",
  score: 25,
  observed_at: "2026-08-26T12:00:00Z",
  latency_ms: null,
  duration_ms: 2795,
  failure_reason: "上游网关错误",
  source: "traffic",
};
const placeholder: AccountRecentResult = {
  result: "未取到日志",
  observed_at: null,
  latency_ms: null,
  failure_reason: null,
  source: "account-state",
};

describe("AccountRecentResults", () => {
  it("追加请求时最旧色块移出，保留色块不会因位置变化被重建", () => {
    const first = { ...traffic, id: "1", observed_at: "2026-09-09T08:00:01Z" };
    const second = { ...traffic, id: "2", observed_at: "2026-09-09T08:00:02Z", duration_ms: 2000 };
    const third = { ...traffic, id: "3", observed_at: "2026-09-09T08:00:03Z", duration_ms: 3000 };
    const view = render(<AccountRecentResults results={[second, first]} limit={2} />);
    const retained = screen.getByLabelText(/总耗时 2000ms/);
    view.rerender(<AccountRecentResults results={[third, second, first]} limit={2} />);
    expect(screen.queryByLabelText(/总耗时 2795ms/)).not.toBeInTheDocument();
    expect(screen.getByLabelText(/总耗时 2000ms/)).toBe(retained);
    expect(retained).toHaveAttribute("tabindex", "0");
    const blocks = within(screen.getByRole("group", { name: "真实流量结果" })).getAllByLabelText(
      /网关错误/,
    );
    expect(blocks[0]).toHaveAccessibleName(/总耗时 2000ms/);
    expect(blocks[1]).toHaveAccessibleName(/总耗时 3000ms/);
  });

  it("混合来源分成真实流量与探针两行，均以实心色块保留事件颜色", () => {
    render(<AccountRecentResults results={[traffic, probe]} />);
    const trafficRow = screen.getByRole("group", { name: "真实流量结果" });
    const probeRow = screen.getByRole("group", { name: "探针结果" });
    expect(within(trafficRow).getByLabelText(/网关错误 · 25 分/)).toHaveClass("bg-orange-500");
    expect(within(probeRow).getByLabelText(/探测通过 · 100 分/)).toHaveClass("bg-success");
    expect(within(probeRow).getByLabelText(/探测通过/)).not.toHaveClass("bg-transparent");
    expect(screen.queryByLabelText("结果来源图例")).not.toBeInTheDocument();
  });

  it("仅有探针时真实流量显示空色条，不伪造流量结果", () => {
    render(<AccountRecentResults results={[probe]} />);
    expect(screen.getByRole("img", { name: "真实流量无结果" })).toBeVisible();
    expect(
      within(screen.getByRole("group", { name: "探针结果" })).getByLabelText(/探测通过/),
    ).toBeVisible();
  });

  it("结果评分只显示整数，计时仍保留原有精度", () => {
    render(<AccountRecentResults results={[{ ...probe, score: 70.8, latency_ms: 320.5 }]} />);
    expect(screen.getByLabelText(/探测通过/)).toHaveAccessibleName(/71 分.*首字 320.5ms/);
  });

  it("色块提供完整错误、计时和来源的可访问名称，并可键盘聚焦", () => {
    const reason = "上游网关错误，等待恢复。".repeat(30);
    render(<AccountRecentResults results={[{ ...traffic, failure_reason: reason }]} />);
    const block = screen.getByLabelText(/网关错误 · 25 分/);
    expect(block).toHaveAttribute("tabindex", "0");
    expect(block).toHaveAccessibleName(expect.stringContaining(reason));
  });

  it.each([{ results: [] }, { results: [placeholder] }])(
    "没有真实结果时保留两行空色条，不显示暂无文案：%j",
    (fixture) => {
      render(<AccountRecentResults results={fixture.results} />);
      expect(screen.getByRole("img", { name: "真实流量无结果" })).toBeVisible();
      expect(screen.getByRole("img", { name: "探针无结果" })).toBeVisible();
      expect(screen.queryByText(/暂无/)).not.toBeInTheDocument();
      expect(
        screen.getByRole("group", { name: "最近结果" }).querySelectorAll('[tabindex="0"]'),
      ).toHaveLength(0);
    },
  );

  it("过滤账号状态占位记录后保留真实探测结果", () => {
    render(<AccountRecentResults results={[placeholder, probe]} />);
    expect(screen.getByLabelText(/探测通过 · 100 分/)).toBeVisible();
    expect(screen.queryByLabelText(/未取到日志/)).not.toBeInTheDocument();
  });

  it("慢响应探针使用实心慢响应色块", () => {
    render(<AccountRecentResults results={[{ ...probe, event_type: "slow" }]} />);
    expect(screen.getByLabelText(/响应慢/)).toHaveClass("bg-lime-500");
  });

  it("超过十条时仅显示最新十条，不包含更老结果", () => {
    const results = Array.from({ length: 12 }, (_, index) => ({
      ...probe,
      latency_ms: 100 + index,
      observed_at: `2026-08-26T12:${String(59 - index).padStart(2, "0")}:00Z`,
    }));
    render(<AccountRecentResults results={results} sampleCount={60} />);
    const blocks = within(screen.getByRole("group", { name: "探针结果" })).getAllByLabelText(
      /探测通过/,
    );
    expect(blocks).toHaveLength(10);
    expect(blocks[0]).toHaveAccessibleName(/首字 109ms/);
    expect(blocks[9]).toHaveAccessibleName(/首字 100ms/);
    expect(screen.queryByLabelText(/首字 110ms/)).not.toBeInTheDocument();
    expect(screen.getByText("有效样本 60")).toBeVisible();
  });

  it("旧来源别名归入对应的探针和流量行", () => {
    render(
      <AccountRecentResults
        results={[
          { ...probe, source: "active_probe" },
          { ...traffic, source: "logs" },
        ]}
      />,
    );
    expect(screen.getByLabelText(/探测通过 · 100 分/)).toHaveClass("bg-success");
    expect(screen.getByLabelText(/网关错误 · 25 分/)).toHaveAccessibleName(/真实流量/);
  });
});
