import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import type { TaskConcurrencySettings } from "@/api";
import { TaskConcurrencySettingsCard } from "../task-concurrency-settings-card";

const settings: TaskConcurrencySettings = {
  pools: [
    { id: "account", limit: 8, queue_capacity: 100, running: 2, waiting: 1 },
    { id: "probe", limit: 4, queue_capacity: 100, running: 0, waiting: 0 },
  ],
  queue_capacity: 100,
  version: "v1",
};
afterEach(() => vi.unstubAllGlobals());
function mount() {
  const client = new QueryClient({
    defaultOptions: { queries: { enabled: false, retry: false }, mutations: { retry: false } },
  });
  client.setQueryData(["task-concurrency"], settings);
  render(
    <QueryClientProvider client={client}>
      <TaskConcurrencySettingsCard />
    </QueryClientProvider>,
  );
  return client;
}

it("读取设置后显示各模块现值和运行状态，编辑后保存完整并发配置", async () => {
  const user = userEvent.setup();
  const writes: unknown[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (_url: string, init?: RequestInit) => {
      writes.push(JSON.parse(String(init?.body)));
      return new Response(
        JSON.stringify({
          ...settings,
          version: "v2",
          pools: [{ ...settings.pools[0], limit: 12 }, settings.pools[1]],
        }),
        { status: 200 },
      );
    }),
  );
  mount();
  const input = screen.getByRole("spinbutton", { name: "账号操作并发上限" });
  expect(input).toHaveValue(8);
  expect(screen.getByText("运行 2 · 等待 1 · 生效上限 8")).toBeVisible();
  expect(screen.getByRole("button", { name: "保存任务并发" })).toBeDisabled();
  await user.clear(input);
  await user.type(input, "12");
  await user.click(screen.getByRole("button", { name: "保存任务并发" }));
  await waitFor(() =>
    expect(writes).toEqual([
      { limits: { account: 12, probe: 4 }, queue_capacity: 100, version: "v1" },
    ]),
  );
  await waitFor(() => expect(screen.getByRole("button", { name: "保存任务并发" })).toBeDisabled());
});

it("后台刷新状态时保留未保存输入及原版本，避免覆盖其他页面修改", async () => {
  const user = userEvent.setup();
  const client = mount();
  const input = screen.getByRole("spinbutton", { name: "账号操作并发上限" });
  await user.clear(input);
  await user.type(input, "12");
  act(() =>
    client.setQueryData(["task-concurrency"], {
      ...settings,
      version: "v2",
      pools: [{ ...settings.pools[0], limit: 20, running: 3 }, settings.pools[1]],
    }),
  );
  expect(input).toHaveValue(12);
  expect(await screen.findByText("运行 3 · 等待 1 · 生效上限 20")).toBeVisible();
});

it("输入空值时展示字段错误并阻止保存", async () => {
  const user = userEvent.setup();
  const fetch = vi.fn();
  vi.stubGlobal("fetch", fetch);
  mount();
  const input = screen.getByRole("spinbutton", { name: "账号操作并发上限" });
  await user.clear(input);
  await user.click(screen.getByRole("button", { name: "保存任务并发" }));
  expect(await screen.findByRole("alert")).toHaveTextContent("请输入 1–10000 之间的整数");
  expect(input).toHaveAttribute("aria-invalid", "true");
  expect(fetch).not.toHaveBeenCalled();
});

it("保存失败时保留草稿并允许重试", async () => {
  const user = userEvent.setup();
  vi.stubGlobal(
    "fetch",
    vi.fn(
      async () =>
        new Response(JSON.stringify({ error: "任务并发设置已更新，请刷新后重试" }), {
          status: 409,
        }),
    ),
  );
  mount();
  const input = screen.getByRole("spinbutton", { name: "账号操作并发上限" });
  await user.clear(input);
  await user.type(input, "12");
  await user.click(screen.getByRole("button", { name: "保存任务并发" }));
  await waitFor(() => expect(screen.getByRole("button", { name: "保存任务并发" })).toBeEnabled());
  expect(input).toHaveValue(12);
});

it("首次读取时显示骨架，失败时提供重试且不能提交", async () => {
  let resolve: ((value: Response) => void) | undefined;
  vi.stubGlobal(
    "fetch",
    vi.fn(
      () =>
        new Promise<Response>((done) => {
          resolve = done;
        }),
    ),
  );
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <TaskConcurrencySettingsCard />
    </QueryClientProvider>,
  );
  expect(screen.getByRole("status", { name: "正在读取任务并发设置" })).toHaveAttribute(
    "aria-busy",
    "true",
  );
  expect(screen.queryByRole("button", { name: "保存任务并发" })).not.toBeInTheDocument();
  await act(async () => resolve?.(new Response("{}", { status: 503 })));
  expect(await screen.findByRole("button", { name: "重新读取" })).toBeVisible();
});
