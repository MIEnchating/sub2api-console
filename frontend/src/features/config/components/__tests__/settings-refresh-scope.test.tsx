import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { ConfigPage } from "@/App";
import type { ConfigTab } from "../../constants";

let client: QueryClient;
afterEach(() => {
  cleanup();
  client?.clear();
  vi.unstubAllGlobals();
});

const config = {
  mode: "完全模式",
  config_keys: [],
  configuration_errors: [],
  account_default_concurrency: 10,
  account_default_priority: 1,
  admin_base_url: "https://sub2api.example.test",
  request_timeout_seconds: 30,
};
const responses: Record<string, unknown> = {
  "/api/config": config,
  "/api/config/account-settings": {
    default: {
      models: [],
      concurrency: 10,
      priority: 1,
      load_factor: null,
      pool_mode: false,
      pool_mode_retry_count: 3,
      pool_mode_retry_status_codes: [429],
    },
    groups: [],
    platform_probe_models: {},
  },
  "/api/config/model-sync": { blocked_patterns: [] },
  "/api/config/task-concurrency": { version: "v1", queue_capacity: 100, pools: [] },
  "/api/catalog/groups": [],
  "/api/catalog/accounts": [],
  "/api/notifications/status": { configured: false, configuration_errors: [], queues: {} },
  "/api/config/log-cleanup": { enabled: false, retention_days: 30 },
};

function renderSettings(tab: ConfigTab): string[] {
  const reads: string[] = [];
  vi.stubGlobal("fetch", async (url: string) => {
    const requestURL = new URL(url, "http://localhost");
    const path = requestURL.pathname;
    reads.push(path + requestURL.search);
    if (path.startsWith("/api/dictionaries")) return Response.json({ items: [] });
    if (path in responses) return Response.json(responses[path]);
    throw new Error(`未配置的隔离请求 ${path}`);
  });
  client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } });
  render(
    <QueryClientProvider client={client}>
      <ConfigPage activeTab={tab} />
    </QueryClientProvider>,
  );
  return reads;
}

it.each(["accounts", "tasks", "dictionaries"] as const)(
  "%s 设置就绪后不读取其他标签页的通知及清理状态",
  async (tab) => {
    const reads = renderSettings(tab);
    await waitFor(() => expect(client.isFetching()).toBe(0));
    expect(reads).not.toContain("/api/notifications/status");
    expect(reads).not.toContain("/api/config/log-cleanup");
  },
);

it.each([
  {
    tab: "accounts",
    paths: [
      "/api/config/account-settings",
      "/api/config/model-sync",
      "/api/catalog/groups",
      "/api/catalog/accounts",
    ],
  },
  { tab: "tasks", paths: ["/api/config/task-concurrency"] },
  { tab: "notifications", paths: ["/api/notifications/status"] },
  { tab: "dictionaries", paths: ["/api/dictionaries?kind=platform"] },
] as const)("$tab 顶部刷新重新读取当前页面的数据", async (fixture) => {
  const reads = renderSettings(fixture.tab);
  await waitFor(() => expect(client.isFetching()).toBe(0));
  reads.length = 0;
  fireEvent.click(screen.getByRole("button", { name: "刷新系统设置" }));
  await waitFor(() => expect(client.isFetching()).toBe(0));
  for (const path of fixture.paths) expect(reads).toContain(path);
});
