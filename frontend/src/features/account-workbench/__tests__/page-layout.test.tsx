import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { AccountWorkbenchPage } from "../components/account-workbench-page";
import { maintenanceKey, runKeys, templateKeys, workbenchKeys } from "../constants";

let client: QueryClient;
afterEach(() => {
  cleanup();
  client?.clear();
  vi.unstubAllGlobals();
});

it("切换工作台标签时不显示重复顶部标题，并保留导航、分区和操作入口", async () => {
  vi.stubGlobal("PointerEvent", MouseEvent);
  client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } });
  client.setQueryData(templateKeys.library, { revision: 1, preferred_id: "", items: [] });
  client.setQueryData(workbenchKeys.accounts, []);
  client.setQueryData(runKeys.list, []);
  client.setQueryData(maintenanceKey, {
    revision: 1,
    enabled: false,
    interval_minutes: 5,
    cooldown_minutes: 10,
    check_after_repair: true,
    group_ids: [],
    running: false,
    task_id: "",
    last_check_at: null,
    next_check_at: null,
    message: "",
    results: [],
  });
  render(
    <QueryClientProvider client={client}>
      <AccountWorkbenchPage />
    </QueryClientProvider>,
  );
  const user = userEvent.setup();
  expect(screen.queryByRole("heading", { name: "账号工作台" })).not.toBeInTheDocument();
  expect(screen.getByRole("tablist", { name: "账号工作台功能" })).toBeVisible();
  for (const label of ["导入账号", "账号列表", "配置模板", "处理记录", "自动维护"]) {
    const tab = screen.getByRole("tab", { name: label });
    await user.click(tab);
    expect(tab).toHaveAttribute("aria-selected", "true");
    const panel = within(screen.getByRole("tabpanel", { name: label }));
    expect(panel.queryByRole("heading", { name: label })).not.toBeInTheDocument();
    if (label === "导入账号") {
      expect(panel.getByRole("textbox", { name: "账号资料" })).toBeVisible();
      expect(panel.getByRole("heading", { name: "处理设置" })).toBeVisible();
      expect(panel.getByRole("button", { name: "解析并预览" })).toBeVisible();
    } else if (label === "账号列表") {
      expect(panel.getByRole("button", { name: "刷新列表" })).toBeEnabled();
    } else {
      expect(panel.getByRole("button", { name: "刷新" })).toBeEnabled();
      if (label === "配置模板")
        expect(panel.getByRole("button", { name: "创建模板" })).toBeEnabled();
    }
  }
});
