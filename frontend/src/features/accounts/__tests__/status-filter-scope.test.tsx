import { seedDictionaries } from "@/test/dictionary-fixtures";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  createMemoryHistory,
  createRootRoute,
  createRouter,
  RouterProvider,
} from "@tanstack/react-router";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";

import { AccountsPage } from "@/App";
import { accountStatusDictionary } from "../components/__tests__/status-dictionary-fixture";
import { account, policy } from "./fixtures";

const clients: QueryClient[] = [];
afterEach(() => {
  clients.splice(0).forEach((client) => client.clear());
  vi.unstubAllGlobals();
});

async function setup(): Promise<void> {
  vi.stubGlobal("PointerEvent", MouseEvent);
  vi.stubGlobal("scrollTo", vi.fn());
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => Response.json([])),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  });
  seedDictionaries(client, {
    group: [
      { value: "1", name: "codex" },
      { value: "2", name: "claude" },
    ],
  });
  clients.push(client);
  client.setQueryData(
    ["accounts"],
    [
      { ...account, name: "健康账号", platform: "openai", account_type: "apikey" },
      {
        ...account,
        id: "42",
        name: "熔断账号",
        health: "fused",
        groups: ["claude"],
        platform: "anthropic",
        account_type: "oauth",
      },
      { ...account, id: "43", name: "人工控制账号", health: "manual_priority", manual_priority: 1 },
    ],
  );
  client.setQueryData(["policy"], policy);
  client.setQueryData(["model-check-account-statuses"], []);
  client.setQueryData(["dictionaries", "account_status"], {
    items: accountStatusDictionary.map((entry) => ({ ...entry, enabled: true })),
  });
  const router = createRouter({
    routeTree: createRootRoute({ component: AccountsPage }),
    history: createMemoryHistory({ initialEntries: ["/"] }),
  });
  await router.load();
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  );
  await screen.findByRole("button", { name: "状态筛选" });
}

it("选择某状态后仍可切换同范围的其他状态，不从当前结果反向裁掉选项", async () => {
  const user = userEvent.setup();
  await setup();

  await user.click(screen.getByRole("button", { name: "状态筛选" }));
  expect(screen.getAllByRole("option")).toHaveLength(12);
  await user.click(screen.getByRole("option", { name: "已熔断" }));
  expect(within(screen.getByRole("table")).queryByText("健康账号")).not.toBeInTheDocument();

  expect(screen.getByRole("option", { name: "健康" })).toBeVisible();
  await user.click(screen.getByRole("option", { name: "健康" }));
  expect(within(screen.getByRole("table")).getByText("健康账号")).toBeVisible();
  expect(within(screen.getByRole("table")).queryByText("熔断账号")).not.toBeInTheDocument();
});

it("搜索缩小账号范围时，仍展示不受账号数量影响的全部状态", async () => {
  const user = userEvent.setup();
  await setup();

  await user.type(screen.getByRole("textbox", { name: "搜索账号、ID、Host 或分组" }), "健康账号");
  await user.click(screen.getByRole("button", { name: "状态筛选" }));

  expect(screen.getAllByRole("option")).toHaveLength(12);
  expect(screen.getByRole("option", { name: "健康" })).toBeVisible();
  expect(screen.getByRole("option", { name: "已熔断" })).toBeEnabled();
});

it("当前结果没有已排除账号时，仍能搜索并选择已排除状态", async () => {
  const user = userEvent.setup();
  await setup();
  await user.type(screen.getByRole("textbox", { name: "搜索账号、ID、Host 或分组" }), "健康账号");
  await user.click(screen.getByRole("button", { name: "状态筛选" }));
  await user.type(screen.getByRole("combobox", { name: "搜索状态" }), "已排除");

  await user.click(screen.getByRole("option", { name: "已排除" }));

  expect(screen.getByRole("option", { name: "已排除" })).toHaveAttribute("aria-selected", "true");
  expect(within(screen.getByRole("table")).queryByText("健康账号")).not.toBeInTheDocument();
});

it.each([
  { filter: "分组", option: "claude", state: "已熔断" },
  { filter: "平台", option: "Anthropic", state: "已熔断" },
  { filter: "类型", option: "API Key", state: "健康" },
])("选择$filter后，仍可搜索并选择当前没有匹配账号的状态", async (scenario) => {
  const user = userEvent.setup();
  await setup();

  await user.click(screen.getByRole("button", { name: `${scenario.filter}筛选` }));
  await user.click(screen.getByRole("option", { name: scenario.option }));
  await user.keyboard("{Escape}");
  await user.click(screen.getByRole("button", { name: "状态筛选" }));

  expect(screen.getAllByRole("option")).toHaveLength(12);
  expect(screen.getByRole("option", { name: scenario.state })).toBeVisible();
  await user.type(screen.getByRole("combobox", { name: "搜索状态" }), "保底强留");
  await user.click(screen.getByRole("option", { name: "保底强留" }));
  expect(screen.getByRole("option", { name: "保底强留" })).toHaveAttribute("aria-selected", "true");
});

it("未打开显示手动控制账号时，搜索并选择手动控制会显示对应账号", async () => {
  const user = userEvent.setup();
  await setup();

  await user.click(screen.getByRole("button", { name: "状态筛选" }));
  await user.type(screen.getByRole("combobox", { name: "搜索状态" }), "手动控制");
  await user.click(screen.getByRole("option", { name: "手动控制" }));

  expect(screen.getByRole("switch", { name: "显示手动控制账号" })).toBeChecked();
  expect(within(screen.getByRole("table")).getByText("人工控制账号")).toBeVisible();
});
