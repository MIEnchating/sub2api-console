import { QueryClient } from "@tanstack/react-query";
import { afterEach, describe, expect, it } from "vitest";

import type { AccountStatus } from "@/api";
import { shareAccountHealthSnapshots } from "../account-health-snapshot";

function account(overrides: Partial<AccountStatus> = {}): AccountStatus {
  return {
    id: "41",
    name: "测试账号",
    groups: [],
    upstream_id: "test-upstream",
    upstream_host: "upstream.example.test",
    upstream_type: "newapi",
    schedulable: true,
    priority: 10,
    load_factor: "1",
    concurrency: 5,
    multiplier: "1",
    balance: "10",
    paused: false,
    paused_reason: null,
    routing_state: "healthy",
    health_status: "healthy",
    health: "healthy",
    desired_health: "healthy",
    apply_pending: false,
    apply_error: null,
    decision_state: "healthy",
    decision_reason: null,
    failure_streak: 0,
    recovery_pass_streak: 2,
    target_priority: 10,
    target_load_factor: "1",
    target_schedulable: true,
    target_concurrency: 5,
    health_score: 100,
    short_score: 100,
    long_score: 100,
    sample_count: 1,
    short_sample_count: 1,
    long_sample_count: 1,
    ttfb_p50_ms: 100,
    ttfb_p95_ms: 100,
    recent_results: [],
    weight: 80,
    ...overrides,
  };
}

const currentHealth = {
  health_score: 66.25,
  short_score: 62.5,
  long_score: 75,
  sample_count: 3,
  short_sample_count: 3,
  long_sample_count: 3,
  ttfb_p50_ms: 120,
  ttfb_p95_ms: 180,
  health_evaluated_at: "2026-09-17T08:00:03.123456789Z",
  health_evidence_at: "2026-09-17T08:00:02Z",
  recent_results: [
    {
      id: "4",
      result: "失败",
      event_type: "gateway_error",
      score: 25,
      observed_at: "2026-09-17T08:00:02Z",
      latency_ms: null,
      failure_reason: "HTTP 503",
      source: "traffic",
    },
  ],
};
const clients: QueryClient[] = [];

function client(): QueryClient {
  const value = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  clients.push(value);
  return value;
}

afterEach(() => {
  for (const value of clients.splice(0)) value.clear();
});

describe("账号列表健康快照共享", () => {
  it("旧整表请求晚于实时更新返回时按账号ID保留健康快照，同时应用其他字段与账号增删", async () => {
    const queryClient = client();
    queryClient.setQueryData(["accounts"], [account(), account({ id: "deleted" })]);
    let finishRequest: (accounts: AccountStatus[]) => void = () => {};
    const response = new Promise<AccountStatus[]>((resolve) => {
      finishRequest = resolve;
    });
    const request = queryClient.fetchQuery({
      queryKey: ["accounts"],
      queryFn: () => response,
      structuralSharing: shareAccountHealthSnapshots,
    });
    queryClient.setQueryData(["accounts"], [account(currentHealth), account({ id: "deleted" })]);
    const added = account({ id: "added", name: "新增账号" });
    finishRequest([
      added,
      account({
        name: "账号新名称",
        concurrency: 8,
        health: "degraded",
        failure_streak: 1,
        weight: 40,
        health_evaluated_at: "2026-09-17T08:00:03.123456788Z",
      }),
    ]);
    await request;

    const accounts = queryClient.getQueryData<AccountStatus[]>(["accounts"]);
    expect(accounts?.map((item) => item.id)).toEqual(["added", "41"]);
    expect(accounts?.[0]).toEqual(added);
    expect(accounts?.[1]).toMatchObject({
      ...currentHealth,
      name: "账号新名称",
      concurrency: 8,
      health: "degraded",
      failure_streak: 1,
      weight: 40,
    });
  });

  it("整表请求携带更新的评估时间时正常替换健康评分与结果", async () => {
    const queryClient = client();
    queryClient.setQueryData(["accounts"], [account(currentHealth)]);
    const incoming = account({
      health_evaluated_at: "2026-09-17T08:00:04Z",
      health_evidence_at: "2026-09-17T08:00:04Z",
    });

    await queryClient.fetchQuery({
      queryKey: ["accounts"],
      queryFn: async () => [incoming],
      structuralSharing: shareAccountHealthSnapshots,
    });

    expect(queryClient.getQueryData(["accounts"])).toEqual([incoming]);
  });

  it("旧整表请求在实时清空证据后返回时不会恢复旧分数或旧色块", async () => {
    const queryClient = client();
    const empty = account({
      health_score: null,
      short_score: null,
      long_score: null,
      sample_count: 0,
      short_sample_count: 0,
      long_sample_count: 0,
      ttfb_p50_ms: null,
      ttfb_p95_ms: null,
      health_evaluated_at: "2026-09-17T08:00:04Z",
      health_evidence_at: null,
    });
    queryClient.setQueryData(["accounts"], [empty]);

    await queryClient.fetchQuery({
      queryKey: ["accounts"],
      queryFn: async () => [account(currentHealth)],
      structuralSharing: shareAccountHealthSnapshots,
    });

    expect(queryClient.getQueryData(["accounts"])).toEqual([empty]);
  });

  it("首次读取不含实时健康时间的旧版响应时仍能正常显示", async () => {
    const queryClient = client();
    const incoming = account();

    await queryClient.fetchQuery({
      queryKey: ["accounts"],
      queryFn: async () => [incoming],
      structuralSharing: shareAccountHealthSnapshots,
    });

    expect(queryClient.getQueryData(["accounts"])).toEqual([incoming]);
  });

  it("整表请求返回空账号列表时移除缓存中的全部账号", async () => {
    const queryClient = client();
    queryClient.setQueryData(["accounts"], [account(currentHealth)]);

    await queryClient.fetchQuery({
      queryKey: ["accounts"],
      queryFn: async () => [],
      structuralSharing: shareAccountHealthSnapshots,
    });

    expect(queryClient.getQueryData(["accounts"])).toEqual([]);
  });
});
