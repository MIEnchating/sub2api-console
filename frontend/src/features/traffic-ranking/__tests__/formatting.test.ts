import { describe, expect, it } from "vitest";

import {
  formatTrafficCachePercent,
  formatTrafficLatency,
  formatTrafficTokens,
  trafficAccountMatches,
  trafficStabilityLabel,
} from "../lib/traffic-ranking";
import { rankingFixture } from "./fixtures";

describe("流量指标展示", () => {
  it("Token 用量统一使用 M 并保留两位小数", () => {
    expect(formatTrafficTokens(1_250_000)).toBe("1.25 M");
    expect(formatTrafficTokens(120_000)).toBe("0.12 M");
    expect(formatTrafficTokens(45_000)).toBe("0.05 M");
    expect(formatTrafficTokens(0)).toBe("0.00 M");
  });

  it("微量 Token 不舍入为零，缺失用量保留未提供", () => {
    expect(formatTrafficTokens(1)).toBe("<0.01 M");
    expect(formatTrafficTokens(9_999)).toBe("<0.01 M");
    expect(formatTrafficTokens(10_000)).toBe("0.01 M");
    expect(formatTrafficTokens(null)).toBe("未提供");
  });

  it("缓存分别按输入侧用量计算占比，不受输出用量影响", () => {
    const usage = {
      input_tokens: 60,
      cache_read_tokens: 30,
      cache_write_tokens: 10,
      output_tokens: 500,
    };
    expect(formatTrafficCachePercent(usage, "cache_read_tokens")).toBe("30.00%");
    expect(formatTrafficCachePercent(usage, "cache_write_tokens")).toBe("10.00%");
  });

  it("输入为零但有缓存时仍可计算缓存占比", () => {
    const usage = { input_tokens: 0, cache_read_tokens: 80, cache_write_tokens: 20 };
    expect(formatTrafficCachePercent(usage, "cache_read_tokens")).toBe("80.00%");
    expect(formatTrafficCachePercent(usage, "cache_write_tokens")).toBe("20.00%");
  });

  it("明确无缓存且存在输入时显示零占比，总量为零时显示横线", () => {
    const usage = { input_tokens: 100, cache_read_tokens: 0, cache_write_tokens: 0 };
    expect(formatTrafficCachePercent(usage, "cache_read_tokens")).toBe("0.00%");
    expect(formatTrafficCachePercent({ ...usage, input_tokens: 0 }, "cache_read_tokens")).toBe("-");
  });

  it.each(["input_tokens", "cache_read_tokens", "cache_write_tokens"] as const)(
    "%s 缺失时不补零计算缓存百分比",
    (field) => {
      const usage = {
        input_tokens: 60,
        cache_read_tokens: 30,
        cache_write_tokens: 10,
        [field]: null,
      };
      expect(formatTrafficCachePercent(usage, "cache_read_tokens")).toBe("未提供");
      expect(formatTrafficCachePercent(usage, "cache_write_tokens")).toBe("未提供");
    },
  );

  it("按评分区间展示稳定性状态，缺失样本不记为不稳定", () => {
    expect(trafficStabilityLabel(95)).toEqual({ label: "稳定", variant: "secondary" });
    expect(trafficStabilityLabel(75)).toEqual({ label: "观察", variant: "warning" });
    expect(trafficStabilityLabel(40)).toEqual({ label: "不稳定", variant: "destructive" });
    expect(trafficStabilityLabel(null)).toEqual({ label: "无样本", variant: "outline" });
  });

  it("输入账号身份或分组关键词时忽略大小写匹配", () => {
    expect(trafficAccountMatches(rankingFixture.accounts[0], "API.EXAMPLE")).toBe(true);
    expect(trafficAccountMatches(rankingFixture.accounts[0], "codex")).toBe(true);
    expect(trafficAccountMatches(rankingFixture.accounts[0], "missing")).toBe(false);
  });

  it("延迟超过一秒时使用秒单位，缺失值保留为空", () => {
    expect(formatTrafficLatency(1450)).toBe("1.45 s");
    expect(formatTrafficLatency(82.25)).toBe("82.3 ms");
    expect(formatTrafficLatency(null)).toBe("-");
  });
});
