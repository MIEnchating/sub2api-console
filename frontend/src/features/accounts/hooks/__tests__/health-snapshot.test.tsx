import { act, renderHook } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactElement, ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { AccountRecentResult, AccountStatus } from "@/api";
import { useAccountResultEvents } from "../use-account-result-events";

class FakeEventSource extends EventTarget {
  static instances: FakeEventSource[] = [];
  close(): void {}
  constructor() {
    super();
    FakeEventSource.instances.push(this);
  }
  emit(type: string, data: unknown): void {
    this.dispatchEvent(new MessageEvent(type, { data: JSON.stringify(data) }));
  }
}

const healthyResult: AccountRecentResult = {
  id: "1",
  result: "通过",
  event_type: "healthy",
  score: 100,
  observed_at: "2026-09-17T08:00:01Z",
  latency_ms: 100,
  failure_reason: null,
  source: "traffic",
};
const failedResult: AccountRecentResult = {
  ...healthyResult,
  id: "2",
  result: "失败",
  event_type: "gateway_error",
  score: 25,
  observed_at: "2026-09-17T08:00:02Z",
  latency_ms: null,
  failure_reason: "上游网关错误",
};
const latestHealth = {
  health_score: 62.5,
  short_score: 25,
  long_score: 62.5,
  sample_count: 2,
  short_sample_count: 1,
  long_sample_count: 2,
  ttfb_p50_ms: 120,
  ttfb_p95_ms: 140,
  health_evaluated_at: "2026-09-17T08:00:03.123456789Z",
  health_evidence_at: failedResult.observed_at,
};
const clients: QueryClient[] = [];

function fixture(): { client: QueryClient; source: FakeEventSource; account: AccountStatus } {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  clients.push(client);
  const account = {
    id: "41",
    health_score: 100,
    short_score: 100,
    long_score: 100,
    sample_count: 1,
    short_sample_count: 1,
    long_sample_count: 1,
    ttfb_p50_ms: 100,
    ttfb_p95_ms: 100,
    recent_results: [healthyResult],
    health: "healthy",
    routing_state: "active",
    weight: 80,
    failure_streak: 0,
    recovery_pass_streak: 3,
  } as AccountStatus;
  client.setQueryData(["accounts"], [account]);
  function Wrapper(props: { children: ReactNode }): ReactElement {
    return <QueryClientProvider client={client}>{props.children}</QueryClientProvider>;
  }
  renderHook(() => useAccountResultEvents(["41"]), { wrapper: Wrapper });
  return { client, account, source: FakeEventSource.instances[0]! };
}

beforeEach(() => {
  vi.useFakeTimers();
  FakeEventSource.instances = [];
  vi.stubGlobal("EventSource", FakeEventSource);
});
afterEach(() => {
  vi.useRealTimers();
  for (const client of clients.splice(0)) client.clear();
  vi.unstubAllGlobals();
});

describe("实时健康快照", () => {
  it("等待合并期间HTTP已写入更新评分时，队列中的旧快照不能覆盖新缓存", () => {
    const setup = fixture();
    act(() => {
      setup.source.emit("snapshot", {
        account_id: "41",
        results: [failedResult],
        health: latestHealth,
      });
      setup.client.setQueryData(
        ["accounts"],
        [
          {
            ...setup.account,
            health_score: 98,
            health_evaluated_at: "2026-09-17T08:00:04Z",
          },
        ],
      );
      vi.advanceTimersByTime(100);
    });
    expect(setup.client.getQueryData<AccountStatus[]>(["accounts"])?.[0]).toMatchObject({
      health_score: 98,
      recent_results: [healthyResult],
    });
  });

  it("新失败结果与评分在同一次缓存通知中生效，并保留调度决策字段", () => {
    const setup = fixture();
    const observed: Array<{ score: number | null; result: string | undefined }> = [];
    const unsubscribe = setup.client.getQueryCache().subscribe((event) => {
      if (event.type !== "updated") return;
      const account = setup.client.getQueryData<AccountStatus[]>(["accounts"])?.[0];
      if (account)
        observed.push({ score: account.health_score, result: account.recent_results[0].id });
    });

    act(() => {
      setup.source.emit("snapshot", {
        account_id: "41",
        results: [failedResult],
        health: latestHealth,
      });
      vi.advanceTimersByTime(100);
    });
    unsubscribe();

    expect(observed).toEqual([{ score: 62.5, result: "2" }]);
    expect(setup.client.getQueryData<AccountStatus[]>(["accounts"])?.[0]).toMatchObject({
      ...latestHealth,
      health: "healthy",
      routing_state: "active",
      weight: 80,
      failure_streak: 0,
      recovery_pass_streak: 3,
    });
  });

  it.each(["2026-09-17T08:00:03.123456788Z", "2026-09-17T08:00:03.1234Z", "2026-09-17T08:00:03Z"])(
    "晚到的旧健康快照 %s 不会回退评分或覆盖新结果",
    (evaluatedAt) => {
      const setup = fixture();
      act(() => {
        setup.source.emit("snapshot", {
          account_id: "41",
          results: [failedResult],
          health: latestHealth,
        });
        setup.source.emit("snapshot", {
          account_id: "41",
          results: [{ ...failedResult, ...healthyResult, id: "2" }],
          health: {
            ...latestHealth,
            health_score: 100,
            health_evaluated_at: evaluatedAt,
          },
        });
        vi.advanceTimersByTime(100);
      });

      const account = setup.client.getQueryData<AccountStatus[]>(["accounts"])?.[0];
      expect(account).toMatchObject(latestHealth);
      expect(account?.recent_results.find((result) => result.id === "2")).toEqual(failedResult);
    },
  );

  it("新快照没有样本和近期记录时同时清除旧评分、延迟与旧色块", () => {
    const setup = fixture();
    const emptyHealth = {
      ...latestHealth,
      health_score: null,
      short_score: null,
      long_score: null,
      sample_count: 0,
      short_sample_count: 0,
      long_sample_count: 0,
      ttfb_p50_ms: null,
      ttfb_p95_ms: null,
      health_evidence_at: null,
    };
    act(() => {
      setup.source.emit("snapshot", { account_id: "41", results: [], health: emptyHealth });
      vi.advanceTimersByTime(100);
    });

    expect(setup.client.getQueryData<AccountStatus[]>(["accounts"])?.[0]).toMatchObject({
      ...emptyHealth,
      recent_results: [],
    });
  });

  it("完整健康快照移除旧样本时近期结果以同一快照为准", () => {
    const setup = fixture();
    act(() => {
      setup.source.emit("snapshot", {
        account_id: "41",
        results: [failedResult],
        health: latestHealth,
      });
      vi.advanceTimersByTime(100);
    });

    expect(setup.client.getQueryData<AccountStatus[]>(["accounts"])?.[0].recent_results).toEqual([
      failedResult,
    ]);
  });

  it.each([
    { health_score: "25" },
    { short_sample_count: -1 },
    { ttfb_p50_ms: -1 },
    { health_evaluated_at: "invalid" },
  ])("健康快照字段无效 %j 时整包忽略，不单独追加结果", (invalid) => {
    const setup = fixture();
    act(() => {
      setup.source.emit("snapshot", {
        account_id: "41",
        results: [failedResult],
        health: { ...latestHealth, ...invalid },
      });
      vi.advanceTimersByTime(100);
    });

    expect(setup.client.getQueryData<AccountStatus[]>(["accounts"])).toEqual([setup.account]);
  });

  it("旧接口不带健康字段的快照和单结果事件仍合并结果且保留已有健康评估", () => {
    const setup = fixture();
    act(() => {
      setup.source.emit("snapshot", { account_id: "41", results: [], health: latestHealth });
      setup.source.emit("snapshot", { account_id: "41", results: [failedResult] });
      setup.source.emit("result", {
        account_id: "41",
        result: { ...failedResult, failure_reason: "网关超时" },
      });
      vi.advanceTimersByTime(100);
    });

    const account = setup.client.getQueryData<AccountStatus[]>(["accounts"])?.[0];
    expect(account).toMatchObject(latestHealth);
    expect(account?.recent_results[0]).toMatchObject({ id: "2", failure_reason: "网关超时" });
  });
});
