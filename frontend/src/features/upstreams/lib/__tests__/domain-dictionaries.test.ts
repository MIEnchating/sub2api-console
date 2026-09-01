import { describe, expect, it } from "vitest";

import {
  configurableUpstreamTypeOptions,
  upstreamAuthStatusIsReady,
  upstreamAuthStatusMeta,
  upstreamAuthStatusOptions,
  upstreamTypeLabel,
  upstreamTypeOptions,
} from "@/lib/domain-dictionaries";

describe("upstream domain dictionaries", () => {
  it("keeps upstream types available without deriving them from loaded hosts", () => {
    expect(upstreamTypeOptions.map((option) => option.value)).toEqual([
      "sub2api",
      "newapi",
      "oneapi",
      "custom",
      "apikey",
    ]);
  });

  it("keeps configurable upstream types as a fixed subset of the global dictionary", () => {
    expect(configurableUpstreamTypeOptions.map((option) => option.value)).toEqual([
      "sub2api",
      "newapi",
      "oneapi",
      "custom",
    ]);
  });

  it("keeps every canonical authentication status available without loaded hosts", () => {
    expect(upstreamAuthStatusOptions.map((option) => option.value)).toEqual([
      "已鉴权",
      "已恢复",
      "待验证",
      "未确认",
      "恢复暂时失败",
      "鉴权失效",
      "配置错误",
    ]);
  });

  it("does not present an unknown authentication status as successful", () => {
    expect(upstreamAuthStatusMeta("上游新增状态")).toEqual({
      label: "未知状态（上游新增状态）",
      tone: "neutral",
    });
  });

  it("uses one fixed readiness dictionary for canonical and legacy auth statuses", () => {
    expect(upstreamAuthStatusIsReady("已鉴权")).toBe(true);
    expect(upstreamAuthStatusIsReady("已恢复")).toBe(true);
    expect(upstreamAuthStatusIsReady("已发现鉴权记录")).toBe(true);
    expect(upstreamAuthStatusIsReady("authenticated")).toBe(true);
    expect(upstreamAuthStatusIsReady("待验证")).toBe(false);
    expect(upstreamAuthStatusIsReady("上游新增状态")).toBe(false);
  });

  it("preserves unknown upstream types in an explicit fallback label", () => {
    expect(upstreamTypeLabel("future-api")).toBe("未知类型（future-api）");
  });
});
