import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { AccountCreationSettingsCard } from "../account-creation-settings-card";
import { ModelSyncSettingsCard } from "../model-sync-settings-card";
import { TaskConcurrencySettingsCard } from "../task-concurrency-settings-card";

let client: QueryClient;
afterEach(() => {
  cleanup();
  client?.clear();
  vi.unstubAllGlobals();
});

const cases = [
  {
    endpoint: "account-settings",
    key: "account-creation-settings",
    element: <AccountCreationSettingsCard fallbackConcurrency={10} fallbackPriority={1} />,
  },
  {
    endpoint: "model-sync",
    key: "account-model-sync-settings",
    element: <ModelSyncSettingsCard />,
  },
  {
    endpoint: "task-concurrency",
    key: "task-concurrency",
    element: <TaskConcurrencySettingsCard />,
  },
];

it.each(cases)("$endpoint 最后一个观察者离页时取消读取，迟到响应不填回缓存", async (fixture) => {
  let signal: AbortSignal | null | undefined;
  let finish!: (response: Response) => void;
  vi.stubGlobal("fetch", (url: string, init?: RequestInit) => {
    if (!url.endsWith(fixture.endpoint))
      return Promise.resolve(Response.json(url.includes("dictionaries") ? { items: [] } : []));
    signal = init?.signal;
    return new Promise<Response>((resolve) => {
      finish = resolve;
    });
  });
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const view = render(<QueryClientProvider client={client}>{fixture.element}</QueryClientProvider>);
  await waitFor(() => expect(finish).toBeDefined());
  view.unmount();
  await act(async () => finish(Response.json({ obsolete: true })));
  expect(signal?.aborted).toBe(true);
  expect(client.getQueryData([fixture.key])).toBeUndefined();
});

it("账号设置首次读取失败时只提供重试，成功后采用服务端配置而非占位值", async () => {
  let failed = true;
  vi.stubGlobal("fetch", async (url: string) => {
    if (!url.endsWith("account-settings"))
      return Response.json(url.includes("dictionaries") ? { items: [] } : []);
    if (failed) return Response.json({ detail: "配置读取失败" }, { status: 503 });
    return Response.json({
      default: {
        models: [],
        concurrency: 35,
        priority: 2,
        load_factor: null,
        pool_mode: false,
        pool_mode_retry_count: 3,
        pool_mode_retry_status_codes: [429],
      },
      groups: [],
      platform_probe_models: {},
    });
  });
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(<QueryClientProvider client={client}>{cases[0].element}</QueryClientProvider>);
  const retry = await screen.findByRole("button", { name: "重新读取" });
  expect(screen.queryByRole("button", { name: "保存全局默认" })).not.toBeInTheDocument();
  failed = false;
  fireEvent.click(retry);
  expect(await screen.findByRole("spinbutton", { name: "全局默认 并发上限" })).toHaveValue(35);
});
