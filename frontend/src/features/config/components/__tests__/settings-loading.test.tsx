import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { AccountCreationSettingsCard } from "../account-creation-settings-card";
import { ModelSyncSettingsCard } from "../model-sync-settings-card";

let client: QueryClient;
afterEach(() => {
  cleanup();
  client?.clear();
  vi.unstubAllGlobals();
});

it.each([
  {
    content: <AccountCreationSettingsCard fallbackConcurrency={10} fallbackPriority={1} />,
    label: "正在读取账号设置",
  },
  { content: <ModelSyncSettingsCard />, label: "正在读取全局屏蔽模型" },
])("$label 时保留面板高度、具名忙碌状态并使用多个内容占位", ({ content, label }) => {
  vi.stubGlobal(
    "fetch",
    vi.fn(() => new Promise<Response>(() => {})),
  );
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(<QueryClientProvider client={client}>{content}</QueryClientProvider>);
  const loading = screen.getByRole("status", { name: label });
  expect(loading).toHaveClass("h-full", "min-h-0");
  expect(loading).toHaveAttribute("aria-busy", "true");
  expect(loading.querySelector('[data-slot="skeleton"]')).not.toBeNull();
  expect(screen.queryByRole("button", { name: /保存/ })).not.toBeInTheDocument();
});

it("账号设置加载时保留模型与路由分栏，操作栏位于内容滚动区之外", () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(() => new Promise<Response>(() => {})),
  );
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <AccountCreationSettingsCard fallbackConcurrency={10} fallbackPriority={1} />
    </QueryClientProvider>,
  );
  const loading = screen.getByRole("status", { name: "正在读取账号设置" });
  expect(loading.querySelector('[data-slot="settings-scroll"]')).toHaveClass(
    "lg:grid-cols-[minmax(0,1.15fr)_minmax(0,1fr)]",
    "overflow-y-auto",
  );
  expect(loading.querySelector('[data-slot="account-models-skeleton"]')).toHaveClass("h-44");
  expect(loading.querySelector('[data-slot="settings-footer"]')).toHaveClass("shrink-0");
  expect(
    loading
      .querySelector('[data-slot="settings-footer"]')
      ?.closest('[data-slot="settings-scroll"]'),
  ).toBeNull();
});
