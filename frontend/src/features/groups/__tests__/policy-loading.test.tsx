import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
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
  render(
    <QueryClientProvider client={client}>
      <GroupsPage />
    </QueryClientProvider>,
  );
  expect(await screen.findByRole("status")).toHaveTextContent("全局策略暂不可用");
  expect(screen.getByRole("button", { name: "编辑分组" })).toBeDisabled();
});
