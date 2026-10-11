import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { AccountCreationSettingsCard } from "../account-creation-settings-card";
import { ModelSyncSettingsCard } from "../model-sync-settings-card";
import { TaskConcurrencySettingsCard } from "../task-concurrency-settings-card";

const initialAccount = {
  default: {
    models: ["original"],
    concurrency: 10,
    priority: 1,
    load_factor: null,
    pool_mode: false,
    pool_mode_retry_count: 3,
    pool_mode_retry_status_codes: [429],
  },
  groups: [],
  platform_probe_models: {},
};
const fixtures = [
  {
    key: "account-creation-settings",
    endpoint: "account-settings",
    element: <AccountCreationSettingsCard fallbackConcurrency={10} fallbackPriority={1} />,
    field: "全局默认 账号模型",
    button: "保存全局默认",
    initial: initialAccount,
    saved: { ...initialAccount, default: { ...initialAccount.default, models: ["saved"] } },
    input: "saved",
    expected: "saved",
  },
  {
    key: "account-model-sync-settings",
    endpoint: "model-sync",
    element: <ModelSyncSettingsCard />,
    field: "屏蔽规则",
    button: "保存屏蔽规则",
    initial: { blocked_patterns: ["original"] },
    saved: { blocked_patterns: ["saved"] },
    input: "saved",
    expected: "saved",
  },
  {
    key: "task-concurrency",
    endpoint: "task-concurrency",
    element: <TaskConcurrencySettingsCard />,
    field: "账号操作并发上限",
    button: "保存任务并发",
    initial: {
      version: "v1",
      queue_capacity: 100,
      pools: [{ id: "account", limit: 8, running: 0, waiting: 0, queue_capacity: 100 }],
    },
    saved: {
      version: "v2",
      queue_capacity: 100,
      pools: [{ id: "account", limit: 12, running: 0, waiting: 0, queue_capacity: 100 }],
    },
    input: "12",
    expected: 12,
  },
];
let client: QueryClient;
afterEach(() => {
  cleanup();
  client?.clear();
  vi.unstubAllGlobals();
});

it.each(fixtures)("$endpoint 保存成功后迟到的旧读取不能覆盖返回值和表单", async (fixture) => {
  let finish!: (response: Response) => void;
  let finishReadback: ((response: Response) => void) | undefined;
  let signal: AbortSignal | null | undefined;
  vi.stubGlobal("fetch", (url: string, init?: RequestInit) => {
    if (!url.endsWith(fixture.endpoint))
      return Promise.resolve(Response.json(url.includes("dictionaries") ? { items: [] } : []));
    if (init?.method === "PUT") return Promise.resolve(Response.json(fixture.saved));
    return new Promise<Response>((resolve) => {
      if (!finish) {
        signal = init?.signal;
        finish = resolve;
      } else {
        finishReadback = resolve;
      }
    });
  });
  client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } });
  client.setQueryData([fixture.key], fixture.initial);
  render(<QueryClientProvider client={client}>{fixture.element}</QueryClientProvider>);
  const field = screen.getByLabelText(fixture.field);
  fireEvent.change(field, { target: { value: fixture.input } });
  let reading!: Promise<void>;
  await act(async () => {
    reading = client.refetchQueries({ queryKey: [fixture.key] });
  });
  await waitFor(() => expect(finish).toBeDefined());
  fireEvent.click(screen.getByRole("button", { name: fixture.button }));
  await waitFor(() => expect(client.getQueryData([fixture.key])).toEqual(fixture.saved));
  await act(async () => {
    finish(Response.json(fixture.initial));
    await reading;
  });
  expect(client.getQueryData([fixture.key])).toEqual(fixture.saved);
  expect(field).toHaveValue(fixture.expected);
  expect(signal?.aborted).toBe(true);
  await act(async () => {
    finishReadback?.(Response.json(fixture.saved));
  });
  await waitFor(() => expect(client.isFetching()).toBe(0));
});
