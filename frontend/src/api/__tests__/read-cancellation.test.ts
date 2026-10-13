import { QueryClient, QueryObserver } from "@tanstack/react-query";
import { afterEach, expect, it, vi } from "vitest";
import { api } from "@/api";

const clients: QueryClient[] = [];
afterEach(() => {
  clients.splice(0).forEach((client) => client.clear());
  vi.unstubAllGlobals();
});

const reads = [
  ["初始化", api.setupStatus],
  ["会话", api.session],
  ["概览", api.overview],
  ["运行配置", api.config],
  ["价格目录", api.pricing],
  ["价格变更", api.pricingChanges],
  ["价格备份", api.pricingBackups],
  ["上游目录", api.upstreams],
  ["上游用量", api.upstreamUsage],
  ["自动化计划", api.accountModelAutomationSchedules],
  ["自动化任务组", api.accountModelAutomationTaskGroups],
  ["巡检状态", api.autoInspection],
  ["鉴权索引", api.authRecoveryConfig],
  ["上游分组历史", api.allUpstreamGroupHistory],
  ["日志清理状态", api.logCleanupStatus],
  ["检测任务", api.detectionTasks],
  ["动画计划", api.animationSchedules],
  ["检测配置", api.modelCheckConfiguration],
] as const;

it.each(reads)("%s读取被取消时中止底层网络等待", async (_label, read) => {
  let signal: AbortSignal | null | undefined;
  vi.stubGlobal("fetch", (_input: RequestInfo | URL, init?: RequestInit) => {
    signal = init?.signal;
    return new Promise<Response>((_resolve, reject) => {
      signal?.addEventListener("abort", () => reject(signal?.reason), { once: true });
    });
  });
  const controller = new AbortController();
  const result = read(controller.signal).catch((error: unknown) => error);
  controller.abort();
  expect(signal?.aborted).toBe(true);
  expect(await result).toBe(controller.signal.reason);
});

it("共享巡检读取仍有订阅者时继续请求，最后订阅者离开才取消且不缓存迟到结果", async () => {
  let signal: AbortSignal | null | undefined;
  let finish!: (response: Response) => void;
  const fetch = vi.fn((_input: RequestInfo | URL, init?: RequestInit) => {
    signal = init?.signal;
    return new Promise<Response>((resolve) => {
      finish = resolve;
    });
  });
  vi.stubGlobal("fetch", fetch);
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  clients.push(client);
  const options = {
    queryKey: ["auto-inspection"],
    queryFn: (context: { signal: AbortSignal }) => api.autoInspection(context.signal),
  };
  const header = new QueryObserver(client, options);
  const page = new QueryObserver(client, options);
  const leaveHeader = header.subscribe(() => {});
  const leavePage = page.subscribe(() => {});
  expect(fetch).toHaveBeenCalledTimes(1);
  leavePage();
  expect(signal?.aborted).toBe(false);
  leaveHeader();
  expect(signal?.aborted).toBe(true);
  finish(Response.json({ enabled: true }));
  await client
    .getQueryCache()
    .find({ queryKey: options.queryKey })
    ?.promise?.catch(() => {});
  expect(client.getQueryData(options.queryKey)).toBeUndefined();
});
