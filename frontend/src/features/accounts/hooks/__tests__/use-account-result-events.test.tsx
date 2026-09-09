import { act, renderHook } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { AccountStatus } from "@/api";
import { useAccountResultEvents } from "../use-account-result-events";

class FakeEventSource extends EventTarget {
  static instances: FakeEventSource[] = [];
  onerror: (() => void) | null = null;
  close = vi.fn();
  constructor(
    public url: string,
    public options: EventSourceInit,
  ) {
    super();
    FakeEventSource.instances.push(this);
  }
  emit(type: string, data: unknown): void {
    this.dispatchEvent(new MessageEvent(type, { data: JSON.stringify(data) }));
  }
}
function sample(id: string, latency: number) {
  return {
    id,
    result: "通过",
    event_type: "healthy",
    score: 100,
    observed_at: `2026-09-09T08:00:${id.padStart(2, "0")}Z`,
    latency_ms: latency,
    duration_ms: null,
    failure_reason: null,
    source: "traffic",
  };
}
function fixture() {
  FakeEventSource.instances = [];
  vi.stubGlobal("EventSource", FakeEventSource);
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const account = { id: "41", recent_results: [sample("1", 101)] } as AccountStatus;
  client.setQueryData(["accounts"], [account]);
  function Wrapper(props: { children: ReactNode }) {
    return <QueryClientProvider client={client}>{props.children}</QueryClientProvider>;
  }
  return { client, Wrapper };
}
afterEach(() => {
  vi.unstubAllGlobals();
});

describe("账号实时请求追加", () => {
  it("收到单条新请求立即追加，重复推送只更新原记录", () => {
    const setup = fixture();
    const view = renderHook(() => useAccountResultEvents(["41"]), { wrapper: setup.Wrapper });
    const source = FakeEventSource.instances[0]!;
    expect(source.options.withCredentials).toBe(true);
    act(() => {
      source.emit("result", { account_id: "41", result: sample("2", 102) });
    });
    expect(
      setup.client
        .getQueryData<AccountStatus[]>(["accounts"])?.[0]
        .recent_results.map((item) => item.id),
    ).toEqual(["2", "1"]);
    act(() => {
      source.emit("result", { account_id: "41", result: sample("2", 202) });
    });
    expect(
      setup.client.getQueryData<AccountStatus[]>(["accounts"])?.[0].recent_results,
    ).toHaveLength(2);
    expect(
      setup.client.getQueryData<AccountStatus[]>(["accounts"])?.[0].recent_results[0].latency_ms,
    ).toBe(202);
    view.unmount();
    setup.client.clear();
  });

  it("翻页关闭旧订阅，旧连接迟到事件和非订阅账号数据不会更新缓存", () => {
    const setup = fixture();
    const view = renderHook((props: { ids: string[] }) => useAccountResultEvents(props.ids), {
      initialProps: { ids: ["41"] },
      wrapper: setup.Wrapper,
    });
    const old = FakeEventSource.instances[0]!;
    view.rerender({ ids: ["42"] });
    expect(old.close).toHaveBeenCalledOnce();
    act(() => {
      old.emit("result", { account_id: "41", result: sample("2", 102) });
      FakeEventSource.instances[1]!.emit("result", { account_id: "41", result: sample("3", 103) });
    });
    expect(
      setup.client.getQueryData<AccountStatus[]>(["accounts"])?.[0].recent_results,
    ).toHaveLength(1);
    view.unmount();
    setup.client.clear();
  });

  it("断线重连的历史快照按请求 ID 合并，错误数据不清空已有结果", () => {
    const setup = fixture();
    const view = renderHook(() => useAccountResultEvents(["41"]), { wrapper: setup.Wrapper });
    const source = FakeEventSource.instances[0]!;
    act(() => {
      source.emit("snapshot", { account_id: "41", results: [sample("2", 102), sample("1", 101)] });
    });
    act(() => {
      source.emit("snapshot", { account_id: "41", results: [sample("2", 102), sample("1", 101)] });
      source.emit("result", { account_id: "41", result: { id: "bad" } });
    });
    expect(
      setup.client
        .getQueryData<AccountStatus[]>(["accounts"])?.[0]
        .recent_results.map((item) => item.id),
    ).toEqual(["2", "1"]);
    view.unmount();
    setup.client.clear();
  });
  it("同一时间的两个请求分别保留，迟到旧记录按发生时间插入", () => {
    const setup = fixture();
    const view = renderHook(() => useAccountResultEvents(["41"]), { wrapper: setup.Wrapper });
    const source = FakeEventSource.instances[0]!;
    act(() => {
      source.emit("result", { account_id: "41", result: sample("3", 103) });
      source.emit("result", {
        account_id: "41",
        result: { ...sample("2", 102), observed_at: sample("1", 101).observed_at },
      });
    });
    expect(
      setup.client
        .getQueryData<AccountStatus[]>(["accounts"])?.[0]
        .recent_results.map((item) => item.id),
    ).toEqual(["3", "2", "1"]);
    view.unmount();
    setup.client.clear();
  });

  it("采集失败和连接中断提供重试状态，恢复采集后显示已连接", () => {
    const setup = fixture();
    const view = renderHook(() => useAccountResultEvents(["41"]), { wrapper: setup.Wrapper });
    const source = FakeEventSource.instances[0]!;
    act(() => {
      source.emit("collection", { account_id: "41", state: "retrying" });
    });
    expect(view.result.current).toBe("retrying");
    act(() => {
      source.onerror?.();
    });
    expect(view.result.current).toBe("reconnecting");
    act(() => {
      source.emit("collection", { account_id: "41", state: "connected" });
    });
    expect(view.result.current).toBe("connected");
    view.unmount();
    setup.client.clear();
  });
});
