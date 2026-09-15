import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";

import type { PricingSnapshot } from "@/api";
import { PricingConfigPage } from "../pricing-page";

let client: QueryClient;

afterEach(() => {
  cleanup();
  client?.clear();
  vi.unstubAllGlobals();
});

function renderPendingConfig(cached = false): (response: Response) => void {
  let resolve!: (response: Response) => void;
  const response = new Promise<Response>((done) => {
    resolve = done;
  });
  vi.stubGlobal(
    "fetch",
    vi.fn(() => response),
  );
  client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  });
  if (cached) client.setQueryData(["pricing"], emptySnapshot());
  render(
    <QueryClientProvider client={client}>
      <PricingConfigPage />
    </QueryClientProvider>,
  );
  return resolve;
}

function emptySnapshot(): PricingSnapshot {
  return {
    config: {
      enabled: false,
      profit_margin: 0.2,
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

it("价格配置首次读取时按参数侧栏和互换范围占位，不显示两个等宽表单", () => {
  renderPendingConfig();

  const loading = screen.getByRole("status", { name: "正在读取价格设置" });
  expect(loading).toHaveAttribute("aria-busy", "true");
  expect(loading.querySelector('[data-slot="pricing-config-layout"]')).toHaveClass(
    "xl:grid-cols-[18rem_minmax(0,1fr)]",
  );
  expect(
    screen
      .getByTestId("pricing-settings-skeleton")
      .querySelectorAll('[data-slot="skeleton-control"]'),
  ).toHaveLength(3);
  expect(screen.getByTestId("pricing-exchange-skeleton")).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "保存配置" })).toBeDisabled();
  expect(screen.getByRole("button", { name: "立即调整" })).toBeDisabled();
  expect(screen.queryByRole("spinbutton")).not.toBeInTheDocument();
  expect(screen.queryByRole("progressbar")).not.toBeInTheDocument();
});

it("价格配置读取成功且没有互换组时移除骨架并展示设置和创建入口", async () => {
  const resolve = renderPendingConfig();

  await act(async () => resolve(Response.json(emptySnapshot())));

  expect(await screen.findByRole("spinbutton", { name: "目标盈利比例" })).toHaveValue(20);
  expect(screen.getByRole("button", { name: "创建第一个互换组" })).toBeVisible();
  expect(screen.getByRole("button", { name: "保存配置" })).toBeEnabled();
  expect(screen.queryByRole("status", { name: "正在读取价格设置" })).not.toBeInTheDocument();
});

it("已有价格配置刷新期间保留当前表单和未保存输入", async () => {
  const resolve = renderPendingConfig(true);
  const user = userEvent.setup();
  const margin = screen.getByRole("spinbutton", { name: "目标盈利比例" });
  await user.clear(margin);
  await user.type(margin, "25");

  await user.click(screen.getByRole("button", { name: "刷新价格数据" }));

  await waitFor(() => expect(client.isFetching()).toBe(1));
  expect(margin).toBeVisible();
  expect(margin).toHaveValue(25);
  expect(screen.queryByRole("status", { name: "正在读取价格设置" })).not.toBeInTheDocument();
  await act(async () => resolve(Response.json(emptySnapshot())));
  await waitFor(() => expect(client.isFetching()).toBe(0));
});

it("价格配置首次读取失败时结束骨架加载并保留刷新入口", async () => {
  const resolve = renderPendingConfig();

  await act(async () => resolve(Response.json({ detail: "价格数据暂时不可用" }, { status: 503 })));

  await waitFor(() =>
    expect(screen.queryByRole("status", { name: "正在读取价格设置" })).not.toBeInTheDocument(),
  );
  expect(screen.getByRole("button", { name: "刷新价格数据" })).toBeEnabled();
  expect(screen.getByRole("button", { name: "保存配置" })).toBeDisabled();
});
