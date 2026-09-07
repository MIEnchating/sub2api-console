import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";

import type { AccountCreationSettings } from "@/api";
import { AccountCreationSettingsCard } from "../account-creation-settings-card";

afterEach(() => vi.unstubAllGlobals());

it("首次账号设置读取完成前禁止占位配置写入，读取后保存保留已有分组和探活模型", async () => {
  const user = userEvent.setup();
  const settings: AccountCreationSettings = {
    default: {
      models: ["model-a"],
      concurrency: 20,
      load_factor: null,
      priority: 2,
      pool_mode: false,
      pool_mode_retry_count: 2,
      pool_mode_retry_status_codes: [429],
    },
    groups: [
      {
        group_id: "6",
        models: ["group-model"],
        concurrency: 30,
        load_factor: null,
        priority: 3,
        pool_mode: true,
        pool_mode_retry_count: 3,
        pool_mode_retry_status_codes: [503],
      },
    ],
    platform_probe_models: { openai: "probe-model" },
  };
  let completeRead!: (response: Response) => void;
  const reading = new Promise<Response>((resolve) => {
    completeRead = resolve;
  });
  const writes: AccountCreationSettings[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
      if (init?.method !== "PUT") return reading;
      const saved = JSON.parse(String(init.body)) as AccountCreationSettings;
      writes.push(saved);
      return new Response(JSON.stringify(saved));
    }),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  });
  client.setQueryData(["groups"], []);
  client.setQueryData(["accounts"], [{ id: "41", name: "账号 A" }]);
  render(
    <QueryClientProvider client={client}>
      <AccountCreationSettingsCard fallbackConcurrency={10} fallbackPriority={1} />
    </QueryClientProvider>,
  );

  expect(screen.queryByRole("button", { name: "保存全局默认" })).not.toBeInTheDocument();
  expect(screen.queryByRole("button", { name: "同步池模式到已有账号" })).not.toBeInTheDocument();
  await act(async () => completeRead(new Response(JSON.stringify(settings))));
  const concurrency = await screen.findByRole("spinbutton", { name: "全局默认 并发上限" });
  await user.clear(concurrency);
  await user.type(concurrency, "25");
  await user.click(screen.getByRole("button", { name: "保存全局默认" }));

  await waitFor(() => expect(writes).toHaveLength(1));
  expect(writes[0].default.concurrency).toBe(25);
  expect(writes[0].groups).toEqual(settings.groups);
  expect(writes[0].platform_probe_models).toEqual(settings.platform_probe_models);
});
