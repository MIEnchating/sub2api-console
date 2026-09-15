import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { AccountWorkbenchPage } from "../components/account-workbench-page";

let client: QueryClient;
afterEach(() => {
  cleanup();
  client?.clear();
  vi.unstubAllGlobals();
});

function mount(local = false): void {
  vi.stubGlobal("PointerEvent", MouseEvent);
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => Response.json([])),
  );
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  client.setQueryData(["setup-status"], { target_configured: !local });
  render(
    <QueryClientProvider client={client}>
      <AccountWorkbenchPage />
    </QueryClientProvider>,
  );
}

it("打开工作台只显示四页和统一账号输入，不再选择独立工具", async () => {
  mount();
  const navigation = within(screen.getByRole("tablist", { name: "账号工作台功能" }));
  expect(navigation.getAllByRole("tab").map((tab) => tab.textContent)).toEqual([
    "导入账号",
    "配置模板",
    "处理记录",
    "自动维护",
  ]);
  expect(await screen.findByRole("textbox", { name: "账号内容" })).toBeVisible();
  expect(screen.queryByRole("combobox", { name: "账号操作" })).not.toBeInTheDocument();
  expect(screen.queryByRole("button", { name: "恢复已保存批次" })).not.toBeInTheDocument();
  expect(screen.getByRole("button", { name: "导入站点", pressed: true })).toBeVisible();
  expect(screen.getByRole("button", { name: "仅导出 JSON" })).toBeVisible();
});

it("输入账号后切换页签再返回保留未提交资料", async () => {
  mount();
  const user = userEvent.setup();
  const input = await screen.findByRole("textbox", { name: "账号内容" });
  await user.type(input, "rt_fixture_pending");
  await user.click(screen.getByRole("tab", { name: "处理记录" }));
  expect(screen.queryByRole("textbox", { name: "账号内容" })).not.toBeInTheDocument();
  await user.click(screen.getByRole("tab", { name: "导入账号" }));
  expect(screen.getByRole("textbox", { name: "账号内容" })).toHaveValue("rt_fixture_pending");
});

it("方向键和首尾键切换主任务时同步焦点、选中状态与关联面板", async () => {
  mount();
  const user = userEvent.setup();
  const navigation = within(screen.getByRole("tablist", { name: "账号工作台功能" }));
  navigation.getByRole("tab", { name: "导入账号" }).focus();
  for (const [key, label] of [
    ["{ArrowRight}", "配置模板"],
    ["{ArrowRight}", "处理记录"],
    ["{Home}", "导入账号"],
  ]) {
    await user.keyboard(key);
    const selected = navigation.getByRole("tab", { name: label });
    expect(selected).toHaveFocus();
    expect(selected).toHaveAttribute("aria-selected", "true");
    expect(screen.getByRole("tabpanel", { name: label })).toHaveAttribute(
      "id",
      selected.getAttribute("aria-controls"),
    );
  }
});

it("未连接站点时统一输入默认仅导出且不会读取托管模板", async () => {
  mount(true);
  expect(await screen.findByRole("textbox", { name: "账号内容" })).toBeVisible();
  expect(screen.getByRole("button", { name: "仅导出 JSON", pressed: true })).toBeVisible();
  expect(screen.getByRole("button", { name: "导入站点" })).toBeDisabled();
  expect(vi.mocked(fetch).mock.calls.some(([url]) => String(url).includes("/templates"))).toBe(
    false,
  );
});
