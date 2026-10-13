import { act, renderHook } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import type { AnimationResult } from "@/api";
import { useTodayResults } from "../use-today-results";

afterEach(() => vi.useRealTimers());

function result(model: string, completed_at: string): AnimationResult {
  return {
    account_id: "41",
    account_name: "账号",
    model,
    completed_at,
    request_id: model,
    status: "failed",
    duration_ms: 10,
  };
}

it("北京时间日期边界只保留当天结果，排除昨天、明天和无效时间", () => {
  vi.useFakeTimers();
  vi.setSystemTime(new Date("2026-10-13T08:00:00Z"));
  const values = [
    result("yesterday", "2026-10-12T15:59:59Z"),
    result("start", "2026-10-12T16:00:00Z"),
    result("end", "2026-10-13T15:59:59Z"),
    result("tomorrow", "2026-10-13T16:00:00Z"),
    result("invalid", "invalid"),
  ];
  const results = new Map(values.map((value) => [value.model, value]));
  const models = new Map([["41", values]]);
  const view = renderHook(() => useTodayResults(results, models));
  expect([...view.result.current.results.keys()]).toEqual(["start", "end"]);
  expect(view.result.current.modelResults.get("41")?.map((value) => value.model)).toEqual([
    "start",
    "end",
  ]);
  view.unmount();
});

it("页面停留跨过北京时间午夜后自动清除前一天结果", () => {
  vi.useFakeTimers();
  vi.setSystemTime(new Date("2026-10-13T15:59:59Z"));
  const value = result("model-a", "2026-10-13T08:00:00Z");
  const results = new Map([["41", value]]);
  const models = new Map([["41", [value]]]);
  const view = renderHook(() => useTodayResults(results, models));
  expect(view.result.current.results.size).toBe(1);
  act(() => vi.advanceTimersByTime(1000));
  expect(view.result.current.results.size).toBe(0);
  expect(view.result.current.modelResults.get("41")).toEqual([]);
  view.unmount();
  expect(vi.getTimerCount()).toBe(0);
});

it("休眠跨日后窗口重新获得焦点会更新日期", () => {
  vi.useFakeTimers();
  vi.setSystemTime(new Date("2026-10-13T08:00:00Z"));
  const results = new Map([["41", result("model-a", "2026-10-13T08:00:00Z")]]);
  const view = renderHook(() => useTodayResults(results));
  vi.setSystemTime(new Date("2026-10-14T08:00:00Z"));
  act(() => window.dispatchEvent(new Event("focus")));
  expect(view.result.current.results.size).toBe(0);
  view.unmount();
});
