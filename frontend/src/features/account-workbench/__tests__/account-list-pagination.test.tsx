import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor, within } from "@testing-library/react";
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

function mount(count: number): void {
  vi.stubGlobal("PointerEvent", MouseEvent);
  client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } });
  client.setQueryData(workbenchKeys.accounts, accountListFixture(count));
  render(
    <QueryClientProvider client={client}>
      <AccountList />
    </QueryClientProvider>,
  );
}

it("500 个账号每页只挂载 20 行，键盘翻页后可读取对应账号配置", async () => {
  mount(500);
  expect(within(screen.getByRole("table", { name: "账号列表" })).getAllByRole("row")).toHaveLength(
    21,
  );
  expect(screen.queryByText("account-21@example.test")).not.toBeInTheDocument();
  const user = userEvent.setup();
  screen.getByRole("button", { name: "转到下一页" }).focus();
  await user.keyboard("{Enter}");
  await user.click(screen.getByRole("button", { name: "查看 账号 21 配置" }));
  expect(
    within(screen.getByRole("dialog")).getByText(/账号 #21 · account-21@example.test/),
  ).toBeVisible();
});

it("末页搜索覆盖全部账号且从首页展示结果，清空搜索后仍在首页", async () => {
  mount(41);
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "转到最后一页" }));
  await user.type(screen.getByRole("searchbox", { name: "搜索账号" }), "account-1@");
  expect(screen.getByText("account-1@example.test")).toBeVisible();
  await user.clear(screen.getByRole("searchbox", { name: "搜索账号" }));
  expect(screen.getByText("account-1@example.test")).toBeVisible();
  expect(screen.getByRole("button", { name: "转到上一页" })).toBeDisabled();
});

it("后台刷新保留当前页，账号数量缩减后回到有效页", async () => {
  mount(41);
  await userEvent.setup().click(screen.getByRole("button", { name: "转到下一页" }));
  client.setQueryData(workbenchKeys.accounts, accountListFixture(42));
  await waitFor(() => expect(screen.getByText("account-21@example.test")).toBeVisible());
  client.setQueryData(workbenchKeys.accounts, accountListFixture(1));
  await waitFor(() => expect(screen.getByText("account-1@example.test")).toBeVisible());
  expect(screen.getByRole("button", { name: "转到下一页" })).toBeDisabled();
});

it("调整每页行数后回到首页，无匹配数据展示空状态", async () => {
  mount(41);
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "转到下一页" }));
  await user.click(screen.getByRole("combobox", { name: "每页行数" }));
  await user.click(await screen.findByRole("option", { name: "50" }));
  expect(within(screen.getByRole("table")).getAllByRole("row")).toHaveLength(42);
  expect(screen.getByRole("button", { name: "转到上一页" })).toBeDisabled();
  await user.type(screen.getByRole("searchbox", { name: "搜索账号" }), "absent");
  expect(screen.getByText("没有匹配的账号")).toBeVisible();
  expect(screen.queryByRole("table")).not.toBeInTheDocument();
});
