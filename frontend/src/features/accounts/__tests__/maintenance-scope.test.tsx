import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  createMemoryHistory,
  createRootRoute,
  createRouter,
  RouterProvider,
} from "@tanstack/react-router";
import { act, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { AccountsPage } from "@/App";
import { seedDictionaries } from "@/test/dictionary-fixtures";
import { account, policy } from "./fixtures";

let client: QueryClient;
afterEach(() => {
  client?.clear();
  vi.unstubAllGlobals();
});

async function setup(): Promise<void> {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => Response.json([])),
  );
  vi.stubGlobal("PointerEvent", MouseEvent);
  vi.stubGlobal("scrollTo", vi.fn());
  client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } });
  seedDictionaries(client, { group: [{ value: "1", name: "codex" }] });
  client.setQueryData(
    ["accounts"],
    [
      { ...account, name: "目标自动账号" },
      { ...account, id: "42", name: "目标手动账号", manual_priority: 1 },
      { ...account, id: "43", name: "范围外账号" },
    ],
  );
  client.setQueryData(["policy"], policy);
  client.setQueryData(["model-check-account-statuses"], []);
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
  await screen.findByRole("button", { name: "账号维护" });
}

it("搜索后打开命名修复只预览匹配的自动账号，刷新名称不扩大已确认范围", async () => {
  const user = userEvent.setup();
  await setup();
  await user.type(screen.getByRole("textbox", { name: "搜索账号、ID、Host 或分组" }), "目标");
  await user.click(screen.getByRole("button", { name: "账号维护" }));
  await user.click(await screen.findByRole("menuitem", { name: "命名修复" }));
  const dialog = within(await screen.findByRole("dialog", { name: "账号命名修复" }));
  expect(dialog.getByText("目标自动账号")).toBeVisible();
  expect(dialog.queryByText("目标手动账号")).not.toBeInTheDocument();
  expect(dialog.queryByText("范围外账号")).not.toBeInTheDocument();
  await act(async () => {
    client.setQueryData(
      ["accounts"],
      [
        { ...account, name: "已刷新名称" },
        { ...account, id: "44", name: "目标新增账号" },
      ],
    );
  });
  expect(await dialog.findByText("已刷新名称")).toBeVisible();
  expect(dialog.queryByText("目标新增账号")).not.toBeInTheDocument();
  expect(dialog.getByRole("button", { name: "确认修复 1 个自动管理账号" })).toBeEnabled();
});

it("筛选无匹配账号时维护菜单禁用自动批量操作", async () => {
  const user = userEvent.setup();
  await setup();
  await user.type(
    screen.getByRole("textbox", { name: "搜索账号、ID、Host 或分组" }),
    "不存在的账号",
  );
  await user.click(screen.getByRole("button", { name: "账号维护" }));
  for (const name of ["配置校验与修复", "同步倍率", "同步模型", "复验绑定", "命名修复"]) {
    expect(await screen.findByRole("menuitem", { name })).toHaveAttribute("aria-disabled", "true");
  }
});
