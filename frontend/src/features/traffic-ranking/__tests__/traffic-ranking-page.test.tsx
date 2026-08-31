import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import type { TrafficRanking } from "@/api";

import { TrafficRankingPage } from "../components/traffic-ranking-page";
import {
  formatTrafficLatency,
  trafficAccountMatches,
  trafficStabilityLabel,
} from "../lib/traffic-ranking";

const ranking: TrafficRanking = {
  start_at: "2026-08-30T12:00:00Z",
  end_at: "2026-08-31T12:00:00Z",
  group_name: "",
  sort_by: "traffic",
  bucket: "hour",
  total_requests: 1250,
  accounts_with_traffic: 1,
  accounts: [
    {
      rank: 1,
      account_id: "41",
      account_name: "稳定主账号",
      upstream_host: "api.example",
      platform: "anthropic",
      groups: ["codex"],
      requests: 1250,
      successful: 1245,
      failed: 5,
      traffic_share: 100,
      success_rate: 99.6,
      stability_score: 99.08,
      average_latency_ms: 820,
      p95_latency_ms: 1450,
      active_buckets: 22,
      total_buckets: 24,
      latest_at: "2026-08-31T11:58:00Z",
    },
    {
      rank: 2,
      account_id: "42",
      account_name: "待接入账号",
      upstream_host: "",
      platform: "openai",
      groups: ["codex"],
      requests: 0,
      successful: 0,
      failed: 0,
      traffic_share: null,
      success_rate: null,
      stability_score: null,
      average_latency_ms: null,
      p95_latency_ms: null,
      active_buckets: 0,
      total_buckets: 24,
      latest_at: null,
    },
  ],
};

describe("流量排行页面", () => {
  it("展示账号流量、可靠性、延迟和时间覆盖，并保留无流量账号", () => {
    const queryClient = new QueryClient();
    queryClient.setQueryData(["groups"], []);
    queryClient.setQueryData(["traffic-ranking", "24h", "all", "traffic"], ranking);

    const markup = renderToStaticMarkup(
      <QueryClientProvider client={queryClient}>
        <TrafficRankingPage />
      </QueryClientProvider>,
    );

    expect(markup).toContain("流量排行");
    expect(markup).not.toContain("请求总量");
    expect(markup).toContain("1,250 / 100.00%");
    expect(markup).toContain("稳定主账号");
    expect(markup).toContain("#41 · codex · api.example");
    expect(markup).not.toContain(">分组</th>");
    expect(markup).toContain("99.08%");
    expect(markup).toContain("820 ms / 1.45 s");
    expect(markup).toContain("22 / 24");
    expect(markup).toContain("待接入账号");
    expect(markup).toContain("无样本");
    expect(markup).toContain('aria-label="时间范围"');
    expect(markup).toContain('aria-label="账号分组"');
    expect(markup).toContain('aria-label="排行维度"');
    expect(markup).toContain("最近 24 小时");
    expect(markup).toContain("全部分组");
    expect(markup).toContain("按流量");
    expect(markup).toContain("共</span><span");
    expect(markup).toContain("min-w-[980px]");
    expect(markup).toContain("overflow-auto");
  });

  it("根据统计置信度映射稳定性状态", () => {
    expect(trafficStabilityLabel(95)).toEqual({ label: "稳定", variant: "secondary" });
    expect(trafficStabilityLabel(75)).toEqual({ label: "观察", variant: "warning" });
    expect(trafficStabilityLabel(40)).toEqual({ label: "不稳定", variant: "destructive" });
    expect(trafficStabilityLabel(null)).toEqual({ label: "无样本", variant: "outline" });
  });

  it("支持按账号身份与分组搜索并统一格式化延迟", () => {
    expect(trafficAccountMatches(ranking.accounts[0], "API.EXAMPLE")).toBe(true);
    expect(trafficAccountMatches(ranking.accounts[0], "codex")).toBe(true);
    expect(trafficAccountMatches(ranking.accounts[0], "missing")).toBe(false);
    expect(formatTrafficLatency(1450)).toBe("1.45 s");
    expect(formatTrafficLatency(82.25)).toBe("82.3 ms");
    expect(formatTrafficLatency(null)).toBe("-");
  });
});
