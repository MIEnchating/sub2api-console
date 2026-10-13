import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { AccountList } from "../components/account-list";
import { MaintenancePanel } from "../components/maintenance-panel";
import { WorkbenchToolbar } from "../components/workbench-section";
import { maintenanceKey, rememberedAccountKey, workbenchKeys } from "../constants";
import { template } from "./template-fixture";
const clients: QueryClient[] = [];
afterEach(() => {
  cleanup();
  clients.forEach((client) => client.clear());
  clients.length = 0;
  vi.unstubAllGlobals();
});
function client(): QueryClient {
  const value = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  });
  clients.push(value);
  value.setQueryData(workbenchKeys.accounts, [
    { ...template.summary, status: "rate_limited", schedulable: true },
  ]);
  value.setQueryData(rememberedAccountKey, []);
  return value;
}
it("账号操作显示智商检测和代理列，已记住账号在可关闭弹窗中展示", async () => {
  render(
    <QueryClientProvider client={client()}>
      <AccountList />
    </QueryClientProvider>,
  );
  expect(screen.getByRole("columnheader", { name: "代理" })).toBeVisible();
  expect(screen.getByText("限流（429）")).toBeVisible();
  expect(screen.getByRole("button", { name: /智商检测/ })).toBeEnabled();
  await userEvent.setup().click(screen.getByRole("button", { name: "查看已记住账号" }));
  expect(screen.getByRole("dialog", { name: "已记住账号" })).toBeVisible();
  await userEvent.setup().keyboard("{Escape}");
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
});
it("维护两张表分别放在可键盘切换的Tab中，设置包含两个智商检测开关", async () => {
  const cache = client();
  cache.setQueryData(maintenanceKey, {
    revision: 1,
    enabled: true,
    interval_minutes: 17,
    cooldown_minutes: 8,
    check_after_repair: true,
    check_enabled: true,
    group_ids: [],
    running: false,
    task_id: "",
    last_check_at: null,
    next_check_at: null,
    message: "",
    results: [],
  });
  render(
    <QueryClientProvider client={cache}>
      <MaintenancePanel />
    </QueryClientProvider>,
  );
  const user = userEvent.setup();
  await user.click(screen.getByRole("tab", { name: "运行状态" }));
  expect(screen.getByRole("table", { name: "维护运行状态" })).toBeVisible();
  await user.keyboard("{ArrowRight}");
  expect(screen.getByRole("tab", { name: "账号结果" })).toHaveAttribute("aria-selected", "true");
  expect(screen.queryByRole("table", { name: "维护运行状态" })).not.toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "维护设置" }));
  const dialog = within(screen.getByRole("dialog", { name: "维护设置" }));
  expect(dialog.getByRole("checkbox", { name: "启用智商检测" })).toBeChecked();
  expect(dialog.getByRole("checkbox", { name: "恢复后执行智商检测" })).toBeChecked();
});
it("工具栏筛选和按钮共享一行容器，说明不再单独占据一行", () => {
  render(
    <WorkbenchToolbar description="说明" actions={<button>刷新</button>}>
      <input aria-label="筛选" />
    </WorkbenchToolbar>,
  );
  const toolbar = screen.getByRole("group", { name: "工作台操作" });
  expect(toolbar).toHaveClass("flex", "flex-wrap");
  expect(screen.getByText("说明")).toHaveClass("sr-only");
  expect(screen.getByRole("textbox", { name: "筛选" })).toBeVisible();
});

it("点击账号智商检测使用保存模型和糖果题，任务执行期间禁止重复提交", async () => {
  let payload: unknown;
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input, init) => {
      if (String(input).endsWith("/import-model"))
        return Response.json({ model: "saved-model", revision: 1 });
      if (String(input).endsWith("/animations")) payload = JSON.parse(String(init?.body));
      return Response.json({ id: "check-task", status: "running", result: {}, message: "检测中" });
    }),
  );
  render(
    <QueryClientProvider client={client()}>
      <AccountList />
    </QueryClientProvider>,
  );
  await userEvent.setup().click(screen.getByRole("button", { name: /智商检测/ }));
  await screen.findByText("正在执行糖果题检测");
  await waitFor(() =>
    expect(payload).toEqual({
      targets: [{ account_id: template.summary.id, model: "saved-model" }],
      mode: "precheck",
      precheck_questions: ["candy"],
      timeout_seconds: 120,
    }),
  );
  await userEvent.setup().keyboard("{Escape}");
  expect(screen.getByRole("button", { name: /智商检测/ })).toBeDisabled();
});
