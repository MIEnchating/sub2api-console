import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { toast } from "sonner";

import { api, type AccountCreationSettings } from "@/api";
import { AccountCreationSettingsCard } from "../account-creation-settings-card";

const settings: AccountCreationSettings = {
  default: {
    models: ["model-a"],
    concurrency: 10,
    load_factor: null,
    priority: 1,
    pool_mode: false,
    pool_mode_retry_count: 3,
    pool_mode_retry_status_codes: [429],
  },
  groups: [],
  platform_probe_models: { openai: "probe-a" },
};
const clients: QueryClient[] = [];

beforeEach(() => vi.stubGlobal("PointerEvent", MouseEvent));
afterEach(() => {
  cleanup();
  clients.forEach((client) => client.clear());
  clients.length = 0;
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

function renderSettings(initial: AccountCreationSettings = settings): QueryClient {
  const client = new QueryClient({ defaultOptions: { queries: { enabled: false } } });
  clients.push(client);
  client.setQueryData(["account-creation-settings"], initial);
  client.setQueryData(["groups"], []);
  render(
    <QueryClientProvider client={client}>
      <AccountCreationSettingsCard fallbackConcurrency={10} fallbackPriority={1} />
    </QueryClientProvider>,
  );
  return client;
}

it("全局策略后台刷新时保留草稿，成功保存后清除未保存状态", async () => {
  vi.spyOn(api, "updateAccountCreationSettings").mockImplementation(async (value) => value);
  const client = renderSettings();
  const field = screen.getByRole("textbox", { name: "全局默认 账号模型" });
  fireEvent.change(field, { target: { value: "model-draft" } });
  await act(async () => {
    client.setQueryData(["account-creation-settings"], {
      ...settings,
      groups: [{ ...settings.default, group_id: "6" }],
      default: { ...settings.default, models: ["model-refresh"] },
    });
  });
  await screen.findByRole("tab", { name: "分组独立配置 1" });
  expect(field).toHaveValue("model-draft");
  fireEvent.click(screen.getByRole("button", { name: "保存全局默认" }));
  await waitFor(() => {
    expect(field).toBeEnabled();
    expect(screen.getByRole("button", { name: "保存全局默认" })).toBeDisabled();
  });
  expect(field).toHaveValue("model-draft");
});

it("探活模型后台刷新时保留草稿，成功保存后清除未保存状态", async () => {
  vi.spyOn(api, "updateAccountCreationSettings").mockImplementation(async (value) => value);
  const client = renderSettings();
  fireEvent.click(screen.getByRole("tab", { name: "探活模型" }));
  const field = screen.getByRole("textbox", { name: "OpenAI 默认探活模型" });
  fireEvent.change(field, { target: { value: "probe-draft" } });
  await act(async () => {
    client.setQueryData(["account-creation-settings"], {
      ...settings,
      groups: [{ ...settings.default, group_id: "6" }],
      platform_probe_models: { openai: "probe-refresh" },
    });
  });
  await screen.findByRole("tab", { name: "分组独立配置 1" });
  expect(field).toHaveValue("probe-draft");
  fireEvent.click(screen.getByRole("button", { name: "保存默认探活模型" }));
  await waitFor(() => {
    expect(field).toBeEnabled();
    expect(screen.getByRole("button", { name: "保存默认探活模型" })).toBeDisabled();
  });
  expect(field).toHaveValue("probe-draft");
});

it("分组策略后台刷新时保留待保存的继承切换", async () => {
  const initial = { ...settings, groups: [{ ...settings.default, group_id: "6" }] };
  const client = renderSettings(initial);
  fireEvent.click(screen.getByRole("tab", { name: "分组独立配置 1" }));
  fireEvent.click(screen.getByRole("switch", { name: "分组 6 使用独立设置" }));
  expect(screen.getByRole("switch", { name: "分组 6 使用独立设置" })).not.toBeChecked();
  await act(async () => {
    client.setQueryData(["account-creation-settings"], {
      ...initial,
      groups: [
        { ...settings.default, group_id: "6", concurrency: 20 },
        { ...settings.default, group_id: "7" },
      ],
    });
  });
  await screen.findByRole("tab", { name: "分组独立配置 2" });
  expect(screen.getByRole("switch", { name: "分组 6 使用独立设置" })).not.toBeChecked();
  expect(screen.getByRole("button", { name: "保存继承设置" })).toBeEnabled();
});

it.each([
  { tab: "全局默认", field: "全局默认 账号模型", submit: "保存全局默认" },
  { tab: "探活模型", field: "OpenAI 默认探活模型", submit: "保存默认探活模型" },
])("$tab 保存失败时保留草稿并允许重试", async (fixture) => {
  vi.spyOn(api, "updateAccountCreationSettings").mockRejectedValue(new Error("配置写入失败"));
  const errorToast = vi.spyOn(toast, "error");
  renderSettings();
  fireEvent.click(screen.getByRole("tab", { name: fixture.tab }));
  const field = screen.getByRole("textbox", { name: fixture.field });
  fireEvent.change(field, { target: { value: "model-draft" } });
  fireEvent.click(screen.getByRole("button", { name: fixture.submit }));
  await waitFor(() => expect(errorToast).toHaveBeenCalledWith("配置写入失败", expect.any(Object)));
  await waitFor(() => expect(screen.getByRole("button", { name: fixture.submit })).toBeEnabled());
  expect(field).toHaveValue("model-draft");
  expect(field).toBeEnabled();
});
