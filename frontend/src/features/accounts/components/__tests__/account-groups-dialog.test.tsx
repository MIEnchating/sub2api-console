import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";

import { type AccountGroupsPreview } from "@/api";
import { AccountGroupsDialog } from "../account-groups-dialog";

const preview: AccountGroupsPreview = {
  account_id: "41",
  current_group_ids: ["7"],
  groups: [
    { id: "7", name: "当前组" },
    { id: "8", name: "目标组" },
  ],
  target_version: "version-1",
};

function response(data: unknown, status = 200): Response {
  return new Response(JSON.stringify(data), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function setup(data: AccountGroupsPreview = preview, manual: number | null = null, locked = false) {
  const onStarted = vi.fn();
  const onOpenChange = vi.fn();
  const requests: Array<{ url: string; init?: RequestInit }> = [];
  const fetch = vi.fn(async (url: string, init?: RequestInit) => {
    requests.push({ url, init });
    if (url.includes("/dictionaries")) return response({ items: [] });
    if (init?.method === "PUT") return response({ id: "groups-task", status: "queued" });
    return response(data);
  });
  vi.stubGlobal("fetch", fetch);
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  const view = render(
    <QueryClientProvider client={client}>
      <AccountGroupsDialog
        account={{ id: "41", name: "账号 A", manual_priority: manual, groups_locked: locked }}
        onStarted={onStarted}
        onOpenChange={onOpenChange}
      />
    </QueryClientProvider>,
  );
  return { ...view, client, fetch, requests, onStarted, onOpenChange };
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

it("读取分组期间显示忙碌提示并保留关闭入口，不显示确认按钮", async () => {
  const f = setup();
  expect(screen.getByRole("status", { name: "正在读取账号分组…" })).toBeVisible();
  expect(screen.getByRole("button", { name: "关闭" })).toBeEnabled();
  expect(screen.queryByRole("button", { name: "确认切换" })).not.toBeInTheDocument();
  await screen.findByRole("button", { name: "确认切换" });
  f.client.clear();
});

it("未改变分组时禁止提交，选择多个分组后按稳定 ID 提交并登记任务", async () => {
  const user = userEvent.setup();
  const f = setup();
  const submit = await screen.findByRole("button", { name: "确认切换" });
  expect(submit).toBeDisabled();
  await user.click(screen.getByRole("combobox", { name: "目标分组" }));
  await user.click(await screen.findByRole("option", { name: "目标组（#8）" }));
  await user.keyboard("{Escape}");
  expect(screen.getByText("切换后分组：当前组、目标组")).toBeVisible();
  await user.click(submit);
  await waitFor(() =>
    expect(f.onStarted).toHaveBeenCalledWith(expect.objectContaining({ id: "groups-task" })),
  );
  const put = f.requests.find((item) => item.init?.method === "PUT");
  expect(JSON.parse(String(put?.init?.body))).toEqual({
    group_ids: ["7", "8"],
    expected_group_ids: ["7"],
    target_version: "version-1",
  });
  expect(f.onOpenChange).toHaveBeenCalledWith(false);
  f.client.clear();
});

it("手动控制账号禁用分组选择和确认并解释解锁方式", async () => {
  const f = setup(preview, 1);
  expect(await screen.findByRole("button", { name: "确认切换" })).toBeDisabled();
  expect(screen.getByRole("combobox", { name: "目标分组" })).toHaveAttribute(
    "aria-disabled",
    "true",
  );
  expect(screen.getByText("请先取消手动控制，再切换分组。")).toBeVisible();
  f.client.clear();
});

it("分组锁定时禁用目标选择和提交，并说明解除方式", async () => {
  const f = setup(preview, null, true);
  expect(await screen.findByRole("button", { name: "确认切换" })).toBeDisabled();
  expect(screen.getByRole("combobox", { name: "目标分组" })).toHaveAttribute(
    "aria-disabled",
    "true",
  );
  expect(screen.getByText("请先关闭锁定分组开关，再切换分组。")).toBeVisible();
  f.client.clear();
});

it("无兼容分组时展示空状态并禁止确认", async () => {
  const f = setup({ ...preview, current_group_ids: [], groups: [] });
  expect(await screen.findByText("没有与账号平台兼容的分组，请先同步管理平台目录。")).toBeVisible();
  expect(screen.getByRole("button", { name: "确认切换" })).toBeDisabled();
  f.client.clear();
});

it("提交失败保留所选分组和重试入口，不关闭弹窗", async () => {
  const user = userEvent.setup();
  const f = setup({ ...preview, current_group_ids: [] });
  await screen.findByRole("button", { name: "确认切换" });
  await user.click(screen.getByRole("combobox", { name: "目标分组" }));
  await user.click(await screen.findByRole("option", { name: "目标组（#8）" }));
  await user.keyboard("{Escape}");
  f.fetch.mockResolvedValueOnce(
    response({ error: { message: "账号分组已变化，请重新选择" } }, 409),
  );
  await user.click(screen.getByRole("button", { name: "确认切换" }));
  await waitFor(() => expect(screen.getByRole("button", { name: "确认切换" })).toBeEnabled());
  expect(screen.getByText("切换后分组：目标组")).toBeVisible();
  expect(f.onOpenChange).not.toHaveBeenCalled();
  expect(f.onStarted).not.toHaveBeenCalled();
  f.client.clear();
});

it("读取失败提供重新读取，重试成功后显示真实分组", async () => {
  vi.stubGlobal("fetch", vi.fn().mockResolvedValue(response({}, 503)));
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <AccountGroupsDialog
        account={{ id: "41", name: "账号 A" }}
        onStarted={vi.fn()}
        onOpenChange={vi.fn()}
      />
    </QueryClientProvider>,
  );
  const user = userEvent.setup();
  const retry = await screen.findByRole("button", { name: "重新读取" });
  expect(screen.queryByRole("button", { name: "确认切换" })).not.toBeInTheDocument();
  vi.stubGlobal(
    "fetch",
    vi.fn(async (url: string) => response(url.includes("dictionaries") ? { items: [] } : preview)),
  );
  await user.click(retry);
  expect(await screen.findByText("当前分组：当前组")).toBeVisible();
  client.clear();
});
