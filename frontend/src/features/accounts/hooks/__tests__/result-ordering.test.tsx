import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import type { AccountRecentResult, AccountStatus } from "@/api";
import { useAccountResultEvents } from "../use-account-result-events";

class ResultStream extends EventTarget {
  static current: ResultStream;
  constructor() {
    super();
    ResultStream.current = this;
  }
  close(): void {}
}
let client: QueryClient;
beforeEach(() => {
  vi.useFakeTimers();
  vi.stubGlobal("EventSource", ResultStream);
  client = new QueryClient();
  client.setQueryData(["accounts"], [{ id: "41", recent_results: [] }]);
  renderHook(() => useAccountResultEvents(["41"]), {
    wrapper: (props: { children: ReactNode }) => (
      <QueryClientProvider client={client}>{props.children}</QueryClientProvider>
    ),
  });
});
afterEach(() => {
  cleanup();
  client.clear();
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

function send(id: string, observedAt: string | null): void {
  const result: AccountRecentResult = {
    id,
    observed_at: observedAt,
    result: "通过",
    latency_ms: 100,
    failure_reason: null,
    source: "traffic",
  };
  ResultStream.current.dispatchEvent(
    new MessageEvent("result", { data: JSON.stringify({ account_id: "41", result }) }),
  );
}
function resultIDs(): Array<string | undefined> {
  return client.getQueryData<AccountStatus[]>(["accounts"])![0].recent_results.map((row) => row.id);
}

it("同一秒内整秒和不同精度的时间按真实纳秒倒序排列", () => {
  act(() => {
    send("4", "2026-10-09T08:00:00Z");
    send("3", "2026-10-09T08:00:00.1Z");
    send("2", "2026-10-09T08:00:00.100000001Z");
    send("1", "2026-10-09T08:00:00.100000002Z");
    vi.advanceTimersByTime(100);
  });
  expect(resultIDs()).toEqual(["1", "2", "3", "4"]);
});

it("相同时间的不同精度表示按完整请求 ID 排序，缺失时间放在末尾", () => {
  act(() => {
    send("9007199254740992", "2026-10-09T08:00:00.100000000Z");
    send("9007199254740993", "2026-10-09T08:00:00.1Z");
    send("10", null);
    send("9", null);
    vi.advanceTimersByTime(100);
  });
  expect(resultIDs()).toEqual(["9007199254740993", "9007199254740992", "10", "9"]);
});

it("超过一百条时保留真实最新记录，整秒旧记录不会挤掉纳秒新记录", () => {
  act(() => {
    for (let id = 1; id <= 100; id++) send(String(id), "2026-10-09T08:00:00Z");
    send("101", "2026-10-09T08:00:00.000000001Z");
    vi.advanceTimersByTime(100);
  });
  expect(resultIDs()).toHaveLength(100);
  expect(resultIDs()[0]).toBe("101");
  expect(resultIDs()).not.toContain("1");
});
