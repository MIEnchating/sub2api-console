import { seedDictionaries } from "@/test/dictionary-fixtures";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { toast } from "sonner";

import { GroupsPage } from "@/App";
import { api, type GroupStatus, type PolicySnapshot } from "@/api";

let client: QueryClient | undefined;
afterEach(() => {
  client?.clear();
  vi.restoreAllMocks();
});

const group: GroupStatus = {
  id: "6",
  name: "codex",
  account_count: 1,
  scheduling_open: 1,
  scheduling_closed: 0,
  scheduling_unknown: 0,
  strategy: "balanced",
  strategy_source: "global_default",
  participation_status: "participating",
  participation_reason: null,
  status: "healthy",
  override: null,
};

const snapshot: PolicySnapshot = {
  available: true,
  source: "console",
  mode: "完全模式",
  global_strategy: "balanced",
  group_strategies: [],
  missing_rate_fallback: null,
  change_threshold: null,
  cooldown_seconds: null,
  auto_apply: null,
  excluded_group_ids: [],
  traffic_enabled: null,
  probe_interval_seconds: 300,
  probe_model: null,
  traffic_lookback_minutes: null,
  max_samples_per_account: null,
  advanced_policy: { weights: { budget: 900 } },
  configuration_errors: [],
};

it("全局策略尚未返回时禁止用占位默认值编辑分组，加载后允许编辑", async () => {
  let resolvePolicy!: (value: PolicySnapshot) => void;
  const policyRequest = new Promise<PolicySnapshot>((resolve) => {
    resolvePolicy = resolve;
  });
  vi.spyOn(api, "groups").mockResolvedValue([group]);
  vi.spyOn(api, "policy").mockReturnValue(policyRequest);
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  seedDictionaries(client);
  render(
    <QueryClientProvider client={client}>
      <GroupsPage />
    </QueryClientProvider>,
  );
  const edit = await screen.findByRole("button", { name: "编辑分组" });
  expect(edit).toBeDisabled();
  resolvePolicy(snapshot);
  await waitFor(() => expect(edit).toBeEnabled());
  client.clear();
});

it("策略读取失败时禁止编辑，刷新成功后恢复编辑", async () => {
  const errorToast = vi.spyOn(toast, "error");
  vi.spyOn(api, "groups").mockResolvedValue([group]);
  vi.spyOn(api, "policy")
    .mockRejectedValueOnce(new Error("策略服务暂时不可用"))
    .mockResolvedValue(snapshot);
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  seedDictionaries(client);
  render(
    <QueryClientProvider client={client}>
      <GroupsPage />
    </QueryClientProvider>,
  );
  await waitFor(() =>
    expect(errorToast).toHaveBeenCalledWith("策略服务暂时不可用", expect.any(Object)),
  );
  expect(screen.getByRole("button", { name: "编辑分组" })).toBeDisabled();
  fireEvent.click(screen.getByRole("button", { name: "刷新分组" }));
  await waitFor(() => expect(screen.getByRole("button", { name: "编辑分组" })).toBeEnabled());
});

it("策略返回不可用时显示配置提示并禁止编辑", async () => {
  vi.spyOn(api, "groups").mockResolvedValue([group]);
  vi.spyOn(api, "policy").mockResolvedValue({ ...snapshot, available: false });
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  seedDictionaries(client);
  render(
    <QueryClientProvider client={client}>
      <GroupsPage />
    </QueryClientProvider>,
  );
  expect(await screen.findByRole("status")).toHaveTextContent("全局策略暂不可用");
  expect(screen.getByRole("button", { name: "编辑分组" })).toBeDisabled();
});

it("编辑中策略刷新失败时禁止保存，重新读取成功后保留草稿并恢复保存", async () => {
  vi.spyOn(api, "groups").mockResolvedValue([group]);
  vi.spyOn(api, "groupProbeModels").mockResolvedValue({
    group_id: "6",
    group_name: "codex",
    models: [],
    account_count: 1,
    accounts_with_models: 0,
    complete: false,
  });
  vi.spyOn(api, "policy")
    .mockResolvedValueOnce(snapshot)
    .mockRejectedValueOnce(new Error("策略读取失败"))
    .mockResolvedValue(snapshot);
  const save = vi.spyOn(api, "updateGroupPolicy");
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  seedDictionaries(client);
  render(
    <QueryClientProvider client={client}>
      <GroupsPage />
    </QueryClientProvider>,
  );
  const edit = await screen.findByRole("button", { name: "编辑分组" });
  await waitFor(() => expect(edit).toBeEnabled());
  fireEvent.click(edit);
  const dialog = within(screen.getByRole("dialog", { name: "编辑分组策略" }));
  const model = dialog.getByRole("textbox", { name: "手动输入探活模型" });
  fireEvent.change(model, { target: { value: "draft-model" } });
  await act(async () => {
    await client!.refetchQueries({ queryKey: ["policy"] });
  });
  await waitFor(() => expect(dialog.getByRole("button", { name: "保存策略" })).toBeDisabled());
  fireEvent.click(dialog.getByRole("button", { name: "保存策略" }));
  expect(save).not.toHaveBeenCalled();
  expect(model).toHaveValue("draft-model");
  expect(dialog.getByRole("button", { name: "取消" })).toBeEnabled();
  fireEvent.click(dialog.getByRole("button", { name: "重新读取" }));
  await waitFor(() => expect(dialog.getByRole("button", { name: "保存策略" })).toBeEnabled());
  expect(model).toHaveValue("draft-model");
});
