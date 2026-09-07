import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";

import type { PricingSnapshot, Task } from "@/api";
import { PricingConfigPage } from "../pricing-page";

afterEach(() => vi.unstubAllGlobals());

it("立即调整价格分组时先展示账号影响范围并在确认后启动任务", async () => {
  const snapshot: PricingSnapshot = {
    config: {
      enabled: true,
      profit_margin: 0.2,
      interval_seconds: 120,
      write_concurrency: 4,
      exchange_group_sets: [["6", "7"]],
      exchange_group_set_names: ["标准渠道"],
    },
    groups: [
      {
        id: "6",
        name: "原售价",
        platform: "openai",
        status: "active",
        rate_multiplier: "2",
        managed: true,
        available: true,
        reason: null,
      },
      {
        id: "7",
        name: "新售价",
        platform: "openai",
        status: "active",
        rate_multiplier: "1",
        managed: true,
        available: true,
        reason: null,
      },
    ],
    decisions: [
      {
        account_id: "41",
        account_name: "标准渠道账号",
        platform: "openai",
        cost_multiplier: "0.5",
        current_group_ids: ["6"],
        desired_group_ids: ["7"],
        eligible_groups: ["新售价"],
        changed: true,
        skipped: false,
        reason: null,
      },
    ],
    accounts: 1,
    changes: 1,
    skipped: 0,
    generated_at: "2026-09-01T00:00:00Z",
  };
  const task: Task = {
    id: "pricing-1",
    skill: "console",
    operation: "price-group-allocation",
    status: "succeeded",
    progress: 100,
    message: "已完成",
    result: {},
    created_at: snapshot.generated_at,
    updated_at: snapshot.generated_at,
  };
  const writes: string[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const path = String(input);
      if (init?.method === "POST") writes.push(path);
      return new Response(
        JSON.stringify(path.includes("/apply") || path.includes("/tasks/") ? task : snapshot),
      );
    }),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  });
  client.setQueryData(["pricing"], snapshot);
  render(
    <QueryClientProvider client={client}>
      <PricingConfigPage />
    </QueryClientProvider>,
  );
  fireEvent.click(screen.getByRole("button", { name: "立即调整" }));

  expect(screen.getByRole("dialog", { name: "确认价格分组调整" })).toBeInTheDocument();
  expect(screen.getByText("标准渠道账号")).toBeInTheDocument();
  expect(writes).toEqual([]);
  fireEvent.click(screen.getByRole("button", { name: "确认调整" }));
  await waitFor(() => expect(writes).toEqual(["/api/pricing/apply"]));
});
