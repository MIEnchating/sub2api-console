import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { toast } from "sonner";

import { ConfigPage } from "@/App";
import type { NotificationStatus, RuntimeConfig, Task } from "@/api";

const clients: QueryClient[] = [];
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  toast.dismiss();
  vi.unstubAllGlobals();
});

const config: RuntimeConfig = {
  database_available: true,
  data_database_available: true,
  mode: "完全模式",
  config_keys: [],
  secret_values_hidden: true,
  probes_enabled: true,
  account_default_concurrency: 10,
  account_default_priority: 1,
  admin_base_url: "https://sub2api.example.test",
  request_timeout_seconds: 30,
  initialized: true,
  target_configured: true,
  console_username: "admin",
  configuration_errors: [],
};
const notifications: NotificationStatus = {
  configured: true,
  app_id: "bot-fixture",
  client_secret_configured: true,
  home_channel: "fixture-user",
  channel_type: "c2c",
  destination_configured: true,
  configuration_errors: [],
  queues: {
    producer_firing: 0,
    producer_recovered: 0,
    consumer_pending: 0,
    consumer_failed: 0,
    consumer_active: false,
  },
};
const scenarios = [
  {
    tab: "connection",
    button: "保存并测试同步",
    operation: "management-sync",
    writes: ["/api/config/target", "/api/management/sync"],
  },
  {
    tab: "notifications",
    button: "连接获取",
    operation: "notification-target-discovery",
    writes: ["/api/notifications/target-discovery"],
  },
] as const;

it.each(scenarios)(
  "$operation 创建后离开设置时中止读取，迟到结果不回填且不取消后台任务",
  async (scenario) => {
    const task: Task = {
      id: scenario.operation,
      operation: scenario.operation,
      skill: "console",
      status: "running",
      message: "任务正在执行",
      progress: 0,
      result: {},
      created_at: "2026-10-09T00:00:00Z",
      updated_at: "2026-10-09T00:00:00Z",
    };
    let signal: AbortSignal | null | undefined;
    let finish!: (response: Response) => void;
    const writes: string[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn((input: string, init?: RequestInit) => {
        const path = new URL(input, "http://localhost").pathname;
        if (init?.method === "POST") {
          writes.push(path);
          if (path === "/api/config/target") return Promise.resolve(Response.json(config));
          if ((scenario.writes as readonly string[]).includes(path))
            return Promise.resolve(Response.json(task));
        }
        if (path === `/api/tasks/${task.id}`) {
          signal = init?.signal;
          return new Promise<Response>((resolve) => {
            finish = resolve;
          });
        }
        throw new Error(`未配置的隔离请求 ${init?.method ?? "GET"} ${path}`);
      }),
    );
    const client = new QueryClient({
      defaultOptions: { queries: { enabled: false, retry: false } },
    });
    clients.push(client);
    client.setQueryData(["config"], config);
    client.setQueryData(["notification-status"], notifications);
    client.setQueryDefaults(["auto-inspection"], { staleTime: Infinity });
    client.setQueryData(["auto-inspection"], {});
    const view = render(
      <QueryClientProvider client={client}>
        <ConfigPage activeTab={scenario.tab} />
      </QueryClientProvider>,
    );
    await userEvent.setup().click(screen.getByRole("button", { name: scenario.button }));
    await waitFor(() => expect(finish).toBeTypeOf("function"));
    view.unmount();
    await act(async () => finish(Response.json({ ...task, status: "succeeded", progress: 100 })));
    expect(signal?.aborted).toBe(true);
    expect(client.getQueryData(["task", task.id])).toBeUndefined();
    expect(writes).toEqual(scenario.writes);
  },
);
