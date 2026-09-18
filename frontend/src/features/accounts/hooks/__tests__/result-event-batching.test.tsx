import { act, renderHook } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import type { AccountStatus } from "@/api";
import { useAccountResultEvents } from "../use-account-result-events";

class EventStream extends EventTarget {
  static instances: EventStream[] = [];
  constructor() {
    super();
    EventStream.instances.push(this);
  }
  close(): void {}
  emit(id: string, resultID = "1"): void {
    this.dispatchEvent(
      new MessageEvent("result", {
        data: JSON.stringify({
          account_id: id,
          result: {
            id: resultID,
            result: "成功",
            observed_at: "2026-09-18T08:00:00Z",
            latency_ms: 100,
            failure_reason: null,
            source: "traffic",
          },
        }),
      }),
    );
  }
}
let client: QueryClient;
beforeEach(() => {
  vi.useFakeTimers();
  EventStream.instances = [];
  vi.stubGlobal("EventSource", EventStream);
  client = new QueryClient();
  client.setQueryData(
    ["accounts"],
    [
      { id: "1", recent_results: [] },
      { id: "2", recent_results: [] },
    ],
  );
});
afterEach(() => {
  client.clear();
  vi.useRealTimers();
  vi.unstubAllGlobals();
});
function Wrapper(props: { children: ReactNode }) {
  return <QueryClientProvider client={client}>{props.children}</QueryClientProvider>;
}

it("离开页面后不再发布尚未刷新的结果", () => {
  const view = renderHook(() => useAccountResultEvents(["1"]), { wrapper: Wrapper });
  act(() => EventStream.instances[0].emit("1"));
  view.unmount();
  act(() => vi.advanceTimersByTime(100));
  expect(client.getQueryData<AccountStatus[]>(["accounts"])?.[0].recent_results).toEqual([]);
});

it("连续到达多个账号结果时只发布一次缓存更新，并保留每个账号的全部结果", () => {
  renderHook(() => useAccountResultEvents(["1", "2"]), { wrapper: Wrapper });
  const notifications: unknown[] = [];
  const unsubscribe = client.getQueryCache().subscribe((event) => {
    if (event.type === "updated" && event.action.type === "success")
      notifications.push(event.query.state.data);
  });
  act(() => {
    EventStream.instances[0].emit("1");
    EventStream.instances[0].emit("2");
    EventStream.instances[0].emit("1", "2");
    vi.advanceTimersByTime(100);
  });
  expect(notifications).toHaveLength(1);
  expect(
    client
      .getQueryData<AccountStatus[]>(["accounts"])
      ?.map((account) => account.recent_results.map((result) => result.id)),
  ).toEqual([["2", "1"], ["1"]]);
  unsubscribe();
});

it("持续收到事件时按固定窗口发布，不因新事件一直推迟刷新", () => {
  renderHook(() => useAccountResultEvents(["1"]), { wrapper: Wrapper });
  act(() => {
    EventStream.instances[0].emit("1");
    vi.advanceTimersByTime(90);
    EventStream.instances[0].emit("1", "2");
    vi.advanceTimersByTime(10);
  });
  expect(client.getQueryData<AccountStatus[]>(["accounts"])?.[0].recent_results).toHaveLength(2);
});

it("翻页后取消旧连接尚未发布的更新，旧连接迟到事件不会污染当前缓存", () => {
  const view = renderHook((props: { ids: string[] }) => useAccountResultEvents(props.ids), {
    initialProps: { ids: ["1"] },
    wrapper: Wrapper,
  });
  act(() => EventStream.instances[0].emit("1"));
  view.rerender({ ids: ["2"] });
  act(() => {
    EventStream.instances[0].emit("1", "2");
    EventStream.instances[1].emit("2");
    vi.advanceTimersByTime(100);
  });
  expect(
    client
      .getQueryData<AccountStatus[]>(["accounts"])
      ?.map((account) => account.recent_results.length),
  ).toEqual([0, 1]);
});
