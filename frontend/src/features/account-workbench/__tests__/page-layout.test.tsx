import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { AccountWorkbenchPage } from "../components/account-workbench-page";
import { maintenanceKey, runKeys, templateKeys, workbenchKeys, workbenchTabs } from "../constants";

let client: QueryClient;
afterEach(() => {
  cleanup();
  client?.clear();
  vi.unstubAllGlobals();
});

it("切换工作台页面内容时不显示重复顶部标题，并保留分区和操作入口", async () => {
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
  const page = (tab: "import" | "accounts" | "templates" | "records" | "maintenance") => (
    <QueryClientProvider client={client}>
      <AccountWorkbenchPage tab={tab} onStarted={() => undefined} />
    </QueryClientProvider>
  );
  const view = render(page("import"));
  expect(screen.queryByRole("heading", { name: "账号工作台" })).not.toBeInTheDocument();
  for (const item of workbenchTabs) {
    view.rerender(page(item.id));
    const label = item.label;
    expect(screen.queryByRole("heading", { name: label })).not.toBeInTheDocument();
    if (label === "导入账号") {
      expect(screen.getByRole("textbox", { name: "账号资料" })).toBeVisible();
      expect(screen.getByRole("heading", { name: "处理设置" })).toBeVisible();
      expect(screen.getByRole("button", { name: "解析并预览" })).toBeVisible();
    } else if (label === "账号列表") {
      expect(screen.getByRole("button", { name: "刷新列表" })).toBeEnabled();
    } else {
      expect(screen.getByRole("button", { name: "刷新" })).toBeEnabled();
      if (label === "配置模板")
        expect(screen.getByRole("button", { name: "创建模板" })).toBeEnabled();
    }
  }
});
