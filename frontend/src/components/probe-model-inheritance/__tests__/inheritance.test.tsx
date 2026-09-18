import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { api, type GroupStatus, type PolicySnapshot } from "@/api";
import { ProbeModelInheritance } from "../../probe-model-inheritance";

const clients: QueryClient[] = [];
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  vi.restoreAllMocks();
});
const policy: PolicySnapshot = {
  available: true,
  source: "test",
  mode: "监控模式",
  global_strategy: "balanced",
  group_strategies: [],
  missing_rate_fallback: null,
  change_threshold: null,
  cooldown_seconds: null,
  auto_apply: null,
  excluded_group_ids: null,
  traffic_enabled: null,
  probe_interval_seconds: 300,
  probe_model: "global-model",
  traffic_lookback_minutes: null,
  max_samples_per_account: null,
  advanced_policy: {},
  configuration_errors: [],
};
const group: GroupStatus = {
  id: "6",
  name: "主分组",
  account_count: 1,
  scheduling_open: 1,
  scheduling_closed: 0,
  scheduling_unknown: 0,
  strategy: "balanced",
  strategy_source: "global_default",
  participation_status: "participating",
  participation_reason: null,
  status: "healthy",
};
function mount(
  groupIds: (string | null)[] = ["6"],
  groups: GroupStatus[] = [group],
  currentPolicy: PolicySnapshot | null = policy,
  metadata?: Record<string, unknown>,
) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  });
  clients.push(client);
  if (currentPolicy) client.setQueryData(["policy"], currentPolicy);
  client.setQueryData(["groups"], groups);
  render(
    <QueryClientProvider client={client}>
      <ProbeModelInheritance groupIds={groupIds} metadata={metadata} />
    </QueryClientProvider>,
  );
  return client;
}

it("分组未独立指定时展示具体全局模型并随缓存更新", async () => {
  const client = mount();
  expect(screen.getByRole("note")).toHaveTextContent("分组「主分组」：继承全局模型：global-model");
  await act(async () => {
    client.setQueryData(["policy"], { ...policy, probe_model: "new-global" });
  });
  expect(await screen.findByText(/继承全局模型：new-global/)).toBeVisible();
});

it("账号属于多个分组时按稳定 ID 分别显示覆盖和继承，重名分组不混淆", () => {
  mount(
    ["6", "7"],
    [group, { ...group, id: "7", override: { probe_model: "group-model", probe_enabled: false } }],
  );
  expect(screen.getByText(/继承全局模型：global-model/)).toBeVisible();
  expect(screen.getByText(/继承分组模型：group-model（分组定时探活已关闭）/)).toBeVisible();
});

it("全局留空时显示账号同步模型且不按模型名排序", () => {
  mount(["6"], [group], { ...policy, probe_model: null }, { known_models: ["z-model", "a-model"] });
  expect(screen.getByRole("note")).toHaveTextContent("已同步的首个可用模型：z-model");
});

it("创建前无同步目录时明确模型将在创建同步后确定", () => {
  mount([], [], { ...policy, probe_model: null });
  expect(screen.getByRole("note")).toHaveTextContent("账号创建并同步模型后，使用首个可用模型");
});

it("缺少分组稳定 ID 时不冒充已继承全局模型", () => {
  mount([null]);
  expect(screen.getByRole("note")).toHaveTextContent("ID 缺失");
  expect(screen.getByRole("note")).not.toHaveTextContent("global-model");
});

it("首次读取未完成时显示加载，失败后可重试取得实际配置", async () => {
  let reject!: (reason: Error) => void;
  vi.spyOn(api, "policy")
    .mockImplementationOnce(
      () =>
        new Promise((_resolve, rejectPromise) => {
          reject = rejectPromise;
        }),
    )
    .mockResolvedValue(policy);
  mount(["6"], [group], null);
  expect(screen.getByRole("status", { name: "正在读取继承的探活配置" })).toBeVisible();
  await act(async () => {
    reject(new Error("读取失败"));
  });
  fireEvent.click(await screen.findByRole("button", { name: "重新读取" }));
  expect(await screen.findByText(/继承全局模型：global-model/)).toBeVisible();
});

it("后台刷新失败时保留已知继承配置", async () => {
  vi.spyOn(api, "policy").mockRejectedValue(new Error("暂时不可用"));
  const client = mount();
  await act(async () => {
    await client.refetchQueries({ queryKey: ["policy"] });
  });
  expect(screen.getByRole("note")).toHaveTextContent("global-model");
  expect(screen.queryByRole("button", { name: "重新读取" })).not.toBeInTheDocument();
});
