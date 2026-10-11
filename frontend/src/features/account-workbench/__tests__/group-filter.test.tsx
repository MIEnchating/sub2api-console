import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { AccountList } from "../components/account-list";
import { workbenchKeys } from "../constants";
import { accountListFixture } from "./list-pagination-fixture";

let client: QueryClient;
afterEach(() => {
  cleanup();
  client?.clear();
  vi.unstubAllGlobals();
});

function mount() {
  vi.stubGlobal("PointerEvent", MouseEvent);
  const accounts = accountListFixture(7);
  for (const account of accounts) account.groups = [{ id: account.id, name: `团队 ${account.id}` }];
  client = new QueryClient({ defaultOptions: { queries: { staleTime: Infinity, retry: false } } });
  client.setQueryData(workbenchKeys.accounts, accounts);
  render(
    <QueryClientProvider client={client}>
      <AccountList />
    </QueryClientProvider>,
  );
  return accounts;
}

it("菜单搜索隐藏已选分组时保留账号范围，关闭再打开可重新搜索", async () => {
  mount();
  const selector = screen.getByRole("combobox", { name: "筛选账号分组" });
  await userEvent.click(selector);
  await userEvent.click(await screen.findByRole("option", { name: "团队 1" }));
  await userEvent.click(selector);
  await userEvent.type(await screen.findByRole("textbox", { name: "搜索选项" }), "团队 7");
  expect(screen.queryByRole("option", { name: "团队 1" })).not.toBeInTheDocument();
  expect(selector).toHaveTextContent("团队 1");
  expect(screen.getByText("account-1@example.test")).toBeVisible();
  expect(screen.queryByText("account-7@example.test")).not.toBeInTheDocument();
  await userEvent.keyboard("{Escape}");
  await userEvent.click(selector);
  const search = await screen.findByRole("textbox", { name: "搜索选项" });
  await userEvent.clear(search);
  await userEvent.type(search, "7");
  await userEvent.click(await screen.findByRole("option", { name: "团队 7" }));
  expect(screen.getByText("account-7@example.test")).toBeVisible();
});

it("搜索期间目录改名按新名称匹配，空结果不改变当前筛选", async () => {
  const accounts = mount();
  await userEvent.click(screen.getByRole("combobox", { name: "筛选账号分组" }));
  const search = await screen.findByRole("textbox", { name: "搜索选项" });
  await userEvent.type(search, "新增团队");
  expect(screen.getByText("没有匹配的选项")).toBeVisible();
  act(() =>
    client.setQueryData(
      workbenchKeys.accounts,
      accounts.map((account) =>
        account.id === "7" ? { ...account, groups: [{ id: "7", name: "新增团队" }] } : account,
      ),
    ),
  );
  await userEvent.click(await screen.findByRole("option", { name: "新增团队" }));
  expect(screen.getByText("account-7@example.test")).toBeVisible();
});

it("同名分组通过搜索ID精确选择，重复组保持首次顺序并采用最后名称", async () => {
  const accounts = mount();
  act(() =>
    client.setQueryData(workbenchKeys.accounts, [
      {
        ...accounts[0],
        groups: [
          { id: "2", name: "旧名称" },
          { id: "1", name: "同名团队" },
        ],
      },
      { ...accounts[1], groups: [{ id: "2", name: "同名团队" }] },
      ...accounts.slice(2),
    ]),
  );
  await userEvent.click(screen.getByRole("combobox", { name: "筛选账号分组" }));
  expect(
    (await screen.findAllByRole("option")).slice(0, 3).map((option) => option.textContent),
  ).toEqual(["全部分组", "同名团队", "同名团队"]);
  await userEvent.type(screen.getByRole("textbox", { name: "搜索选项" }), "1");
  await userEvent.click(await screen.findByRole("option", { name: "同名团队" }));
  await waitFor(() => expect(screen.queryByText("account-2@example.test")).not.toBeInTheDocument());
  expect(screen.getByText("account-1@example.test")).toBeVisible();
});
