import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";

import type { PricingSnapshot } from "@/api";
import { PricingConfigPage } from "../pricing-page";

let client: QueryClient;
afterEach(() => {
  cleanup();
  client?.clear();
  vi.unstubAllGlobals();
});

function snapshot(margin = 0.2): PricingSnapshot {
  return {
    config: {
      enabled: false,
      profit_margin: margin,
      interval_seconds: 120,
      write_concurrency: 4,
      exchange_group_sets: [],
      exchange_group_set_names: [],
    },
    groups: [],
    decisions: [],
    accounts: 0,
    changes: 0,
    skipped: 0,
    generated_at: "2026-09-14T00:00:00Z",
  };
}

function renderConfig(): () => void {
  client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } });
  client.setQueryData(["pricing"], snapshot());
  const view = render(
    <QueryClientProvider client={client}>
      <PricingConfigPage />
    </QueryClientProvider>,
  );
  return () =>
    view.rerender(
      <QueryClientProvider client={client}>
        <PricingConfigPage />
      </QueryClientProvider>,
    );
}

it("修改价格草稿后后台返回新配置时保留未保存输入", () => {
  const rerenderConfig = renderConfig();
  fireEvent.change(screen.getByRole("spinbutton", { name: "目标盈利比例" }), {
    target: { value: "25" },
  });

  act(() => {
    client.setQueryData(["pricing"], snapshot(0.3));
  });
  rerenderConfig();

  expect(screen.getByRole("spinbutton", { name: "目标盈利比例" })).toHaveValue(25);
});

it("尚未编辑价格配置时后台返回新值会同步到表单", () => {
  const rerenderConfig = renderConfig();

  act(() => {
    client.setQueryData(["pricing"], snapshot(0.3));
  });
  rerenderConfig();

  expect(screen.getByRole("spinbutton", { name: "目标盈利比例" })).toHaveValue(30);
});

it("保存请求期间继续修改价格草稿时响应不会覆盖较新的输入", async () => {
  let resolve!: (response: Response) => void;
  vi.stubGlobal(
    "fetch",
    () =>
      new Promise<Response>((done) => {
        resolve = done;
      }),
  );
  renderConfig();
  const margin = screen.getByRole("spinbutton", { name: "目标盈利比例" });
  fireEvent.change(margin, { target: { value: "25" } });
  fireEvent.click(screen.getByRole("button", { name: "保存配置" }));
  await screen.findByRole("button", { name: "保存中" });
  fireEvent.change(margin, { target: { value: "35" } });

  await act(async () => resolve(Response.json(snapshot(0.25))));

  await waitFor(() => expect(screen.getByRole("button", { name: "保存配置" })).toBeEnabled());
  expect(margin).toHaveValue(35);
});
