import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";

import type { AccountCreationSettings, Task } from "@/api";
import { AccountCreationSettingsCard } from "../account-creation-settings-card";

const clients: QueryClient[] = [];
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  vi.unstubAllGlobals();
});

it("池模式同步提交后离开设置时取消状态读取，保留后台任务且不回填迟到结果", async () => {
  const task: Task = {
    id: "pool-1",
    skill: "console",
    operation: "account-pool-mode-sync",
    status: "running",
    progress: 20,
    message: "正在同步已有账号池模式",
    result: {},
    created_at: "2026-10-09T00:00:00Z",
    updated_at: "2026-10-09T00:00:00Z",
  };
  const settings: AccountCreationSettings = {
    default: {
      models: [],
      concurrency: 1,
      load_factor: null,
      priority: 1,
      pool_mode: false,
      pool_mode_retry_count: 3,
      pool_mode_retry_status_codes: [429],
    },
    groups: [],
    platform_probe_models: {},
  };
  let signal: AbortSignal | null | undefined;
  let finish!: (response: Response) => void;
  const requests: string[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn((input: string, init?: RequestInit) => {
      requests.push(`${init?.method ?? "GET"} ${input}`);
      if (init?.method === "POST") return Promise.resolve(Response.json(task));
      signal = init?.signal;
      return new Promise<Response>((resolve) => {
        finish = resolve;
      });
    }),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  });
  clients.push(client);
  client.setQueryDefaults(["account-pool-mode-sync"], { staleTime: 0 });
  client.setQueryData(["account-creation-settings"], settings);
  client.setQueryData(["accounts", "options"], [{ id: "account-1", name: "已有账号" }]);
  client.setQueryData(["groups", "options"], []);
  client.setQueryData(["dictionaries", "group"], { items: [] });
  const view = render(
    <QueryClientProvider client={client}>
      <AccountCreationSettingsCard fallbackConcurrency={1} fallbackPriority={1} />
    </QueryClientProvider>,
  );
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "同步池模式到已有账号" }));
  await user.click(screen.getByRole("button", { name: "确认同步 1 个账号" }));
  await screen.findByRole("status", { name: task.message });
  await waitFor(() => expect(finish).toBeTypeOf("function"));
  view.unmount();
  await act(async () => finish(Response.json({ ...task, status: "succeeded", progress: 100 })));
  expect(signal?.aborted).toBe(true);
  expect(client.getQueryData<Task>(["account-pool-mode-sync", "pool-1"])?.status).toBe("running");
  expect(requests.filter((request) => request.startsWith("POST "))).toHaveLength(1);
  expect(requests.some((request) => request.includes("/cancel"))).toBe(false);
});
