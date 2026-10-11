import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { AccountList } from "../components/account-list";
import { workbenchKeys } from "../constants";
import type { WorkbenchAccount } from "../types";
import { accountListFixture } from "./list-pagination-fixture";

let client: QueryClient;
afterEach(() => {
  cleanup();
  client?.clear();
  vi.unstubAllGlobals();
});

function mount(): WorkbenchAccount[] {
  vi.stubGlobal("PointerEvent", MouseEvent);
  const accounts = accountListFixture(2);
  accounts[0].groups = [{ id: "7", name: "团队组" }];
  accounts[1].groups = [{ id: "8", name: "其他组" }];
  accounts[0].proxy_name = "原代理";
  client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } });
  client.setQueryData(workbenchKeys.accounts, accounts);
  render(
    <QueryClientProvider client={client}>
      <AccountList />
    </QueryClientProvider>,
  );
  return accounts;
}

it("搜索期间目录刷新重新匹配名称邮箱分组与状态，不沿用旧搜索内容", async () => {
  const accounts = mount();
  const search = screen.getByRole("searchbox", { name: "搜索账号" });
  fireEvent.change(search, { target: { value: "团队组" } });
  expect(screen.getByText("account-1@example.test")).toBeVisible();
  const updated = {
    ...accounts[0],
    name: "New owner",
    email: "new@example.test",
    groups: [{ id: "7", name: "新组" }],
    schedulable: false,
  };
  act(() => client.setQueryData(workbenchKeys.accounts, [updated, accounts[1]]));
  expect(await screen.findByText("没有匹配的账号")).toBeVisible();
  fireEvent.change(search, { target: { value: " NEW@ " } });
  expect(screen.getByText("new@example.test")).toBeVisible();
  fireEvent.change(search, { target: { value: "新组" } });
  await userEvent.click(screen.getByRole("combobox", { name: "筛选账号状态" }));
  await userEvent.click(await screen.findByRole("option", { name: "正常" }));
  expect(screen.getByText("没有匹配的账号")).toBeVisible();
  await userEvent.click(screen.getByRole("combobox", { name: "筛选账号状态" }));
  await userEvent.click(await screen.findByRole("option", { name: "已停用" }));
  expect(screen.getByText("New owner")).toBeVisible();
});

it("同一账号后台刷新后详情显示新配置并保留精确倍率", async () => {
  const accounts = mount();
  await userEvent.click(screen.getByRole("button", { name: "查看 账号 1 配置" }));
  const updated = {
    ...accounts[0],
    name: "更新账号",
    proxy_name: "新代理",
    concurrency: "0",
    rate_multiplier: "0.1234567890123456789",
    model_mapping: { source: "target" },
  };
  act(() => client.setQueryData(workbenchKeys.accounts, [updated, accounts[1]]));
  const dialog = within(screen.getByRole("dialog"));
  await waitFor(() =>
    expect(dialog.getByRole("heading", { level: 2 })).toHaveTextContent("更新账号"),
  );
  expect(dialog.getByText("新代理")).toBeVisible();
  expect(dialog.queryByText("原代理")).not.toBeInTheDocument();
  expect(dialog.getByText("0")).toBeVisible();
  expect(dialog.getByText("0.1234567890123456789")).toBeVisible();
  expect(dialog.getByRole("row", { name: "source target" })).toBeVisible();
});

it("刷新移除当前账号后关闭详情，同ID重新出现也不自动打开", async () => {
  const accounts = mount();
  await userEvent.click(screen.getByRole("button", { name: "查看 账号 1 配置" }));
  act(() => client.setQueryData(workbenchKeys.accounts, [accounts[1]]));
  await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  act(() => client.setQueryData(workbenchKeys.accounts, accounts));
  await screen.findByRole("button", { name: "查看 账号 1 配置" });
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
});

it("刷新后所选分组消失时保留筛选范围，显式选择全部后才显示其他账号", async () => {
  const accounts = mount();
  await userEvent.click(screen.getByRole("combobox", { name: "筛选账号分组" }));
  await userEvent.click(await screen.findByRole("option", { name: "团队组" }));
  act(() => client.setQueryData(workbenchKeys.accounts, [accounts[1]]));
  expect(await screen.findByText("没有匹配的账号")).toBeVisible();
  const selector = screen.getByRole("combobox", { name: "筛选账号分组" });
  expect(selector).not.toHaveTextContent("全部分组");
  expect(screen.queryByText("account-2@example.test")).not.toBeInTheDocument();
  await userEvent.click(selector);
  await userEvent.click(await screen.findByRole("option", { name: "全部分组" }));
  expect(await screen.findByText("account-2@example.test")).toBeVisible();
});

it("分组改名保持稳定ID筛选，原分组恢复后继续展示对应账号", async () => {
  const accounts = mount();
  await userEvent.click(screen.getByRole("combobox", { name: "筛选账号分组" }));
  await userEvent.click(await screen.findByRole("option", { name: "团队组" }));
  const renamed = { ...accounts[0], groups: [{ id: "7", name: "更名团队" }] };
  act(() => client.setQueryData(workbenchKeys.accounts, [renamed, accounts[1]]));
  await waitFor(() =>
    expect(screen.getByRole("combobox", { name: "筛选账号分组" })).toHaveTextContent("更名团队"),
  );
  expect(screen.queryByText("account-2@example.test")).not.toBeInTheDocument();
  act(() => client.setQueryData(workbenchKeys.accounts, [accounts[1]]));
  await screen.findByText("没有匹配的账号");
  act(() => client.setQueryData(workbenchKeys.accounts, [renamed, accounts[1]]));
  expect(await screen.findByText("account-1@example.test")).toBeVisible();
  expect(screen.queryByText("account-2@example.test")).not.toBeInTheDocument();
});

it("后台读取失败时保留已打开的详情及当前配置", async () => {
  mount();
  vi.stubGlobal("fetch", vi.fn().mockRejectedValue(new Error("测试读取失败")));
  await userEvent.click(screen.getByRole("button", { name: "查看 账号 1 配置" }));
  await act(async () => {
    await client.invalidateQueries({ queryKey: workbenchKeys.accounts });
  });
  await waitFor(() => expect(client.getQueryState(workbenchKeys.accounts)?.status).toBe("error"));
  expect(within(screen.getByRole("dialog")).getByText("原代理")).toBeVisible();
});

it("所选分组消失后仍可用键盘选择全部分组", async () => {
  const accounts = mount();
  await userEvent.click(screen.getByRole("combobox", { name: "筛选账号分组" }));
  await userEvent.click(await screen.findByRole("option", { name: "团队组" }));
  act(() => client.setQueryData(workbenchKeys.accounts, [accounts[1]]));
  await screen.findByText("没有匹配的账号");
  screen.getByRole("combobox", { name: "筛选账号分组" }).focus();
  await userEvent.keyboard("{Enter}");
  const option = await screen.findByRole("option", { name: "全部分组" });
  option.focus();
  await userEvent.keyboard("{Enter}");
  expect(await screen.findByText("account-2@example.test")).toBeVisible();
});
