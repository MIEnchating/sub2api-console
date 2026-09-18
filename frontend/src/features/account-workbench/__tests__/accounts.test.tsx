import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { AccountList } from "../components/account-list";
import { AccountWorkbenchPage } from "../components/account-workbench-page";
import { workbenchKeys } from "../constants";
import type { WorkbenchAccount } from "../types";

let client: QueryClient;
afterEach(() => {
  cleanup();
  client?.clear();
  vi.unstubAllGlobals();
});
const account: WorkbenchAccount = {
  id: "41",
  name: "团队账号",
  email: "owner@example.test",
  status: "active",
  schedulable: true,
  plan: "prolite",
  groups: [{ id: "7", name: "团队组" }],
  proxy_name: "",
  concurrency: "0",
  load_factor: "",
  rate_multiplier: "0.1234567890123456789",
  model_mapping: { "gpt-5": "gpt-5.6" },
  fingerprint: "session",
};
function mount(page = false): void {
  vi.stubGlobal("PointerEvent", MouseEvent);
  client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } });
  client.setQueryData(workbenchKeys.accounts, [
    account,
    {
      ...account,
      id: "42",
      name: "个人账号",
      email: "other@example.test",
      groups: [],
      schedulable: false,
    },
  ]);
  render(
    <QueryClientProvider client={client}>
      {page ? <AccountWorkbenchPage /> : <AccountList />}
    </QueryClientProvider>,
  );
}
it("新版工作台默认导航包含账号列表且不再挂载旧工具入口", async () => {
  mount(true);
  expect(
    within(screen.getByRole("tablist", { name: "账号工作台功能" }))
      .getAllByRole("tab")
      .map((item) => item.textContent),
  ).toEqual(["导入账号", "账号列表", "配置模板", "处理记录", "自动维护"]);
  await userEvent.setup().click(screen.getByRole("tab", { name: "账号列表" }));
  expect(screen.getByRole("table", { name: "账号列表" })).toBeVisible();
});
it("邮箱搜索筛出对应账号并能打开完整只读配置", async () => {
  mount();
  const user = userEvent.setup();
  await user.type(screen.getByRole("searchbox", { name: "搜索账号" }), "OWNER@");
  expect(screen.getByText("团队账号")).toBeVisible();
  expect(screen.queryByText("个人账号")).not.toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "查看 团队账号 配置" }));
  const dialog = within(screen.getByRole("dialog"));
  expect(dialog.getByText("gpt-5 → gpt-5.6")).toBeVisible();
  expect(dialog.getByText("设备+会话")).toBeVisible();
  expect(dialog.getByText("0.1234567890123456789")).toBeVisible();
  await user.keyboard("{Escape}");
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
});
it("筛选已停用包含暂停调度账号，搜索无结果显示空状态", async () => {
  mount();
  const user = userEvent.setup();
  await user.click(screen.getByRole("combobox", { name: "筛选账号状态" }));
  await user.click(screen.getByRole("option", { name: "已停用" }));
  expect(screen.getByText("个人账号")).toBeVisible();
  expect(screen.queryByText("团队账号")).not.toBeInTheDocument();
  await user.type(screen.getByRole("searchbox", { name: "搜索账号" }), "missing");
  expect(screen.getByText("没有匹配的账号")).toBeVisible();
});

it("筛选框初始与选择后均显示中文状态和分组名称，不显示协议编码", async () => {
  mount();
  const user = userEvent.setup();
  const status = screen.getByRole("combobox", { name: "筛选账号状态" });
  const group = screen.getByRole("combobox", { name: "筛选账号分组" });
  expect(status).toHaveTextContent("全部状态");
  expect(group).toHaveTextContent("全部分组");
  await user.click(status);
  await user.click(screen.getByRole("option", { name: "正常" }));
  expect(status).toHaveTextContent("正常");
  await user.click(group);
  await user.click(screen.getByRole("option", { name: "团队组" }));
  expect(group).toHaveTextContent("团队组");
});
