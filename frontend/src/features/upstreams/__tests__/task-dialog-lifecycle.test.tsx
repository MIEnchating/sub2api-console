import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  RouterProvider,
} from "@tanstack/react-router";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";

import { UpstreamsPage } from "@/App";
import { api, type ManualAuthVerifyResult, type Task } from "@/api";

let client: QueryClient | undefined;
beforeEach(() => {
  vi.stubGlobal("PointerEvent", MouseEvent);
  vi.spyOn(window, "scrollTo").mockImplementation(() => {});
  const matches = Element.prototype.matches;
  vi.spyOn(Element.prototype, "matches").mockImplementation(function (
    this: Element,
    selector: string,
  ) {
    if ([":fullscreen", ":popover-open", ":modal"].includes(selector)) return false;
    return matches.call(this, selector);
  });
});

it("手动凭据验证等待响应时按 Escape 保留窗口，失败后允许关闭", async () => {
  const user = userEvent.setup();
  let rejectVerification!: (error: Error) => void;
  vi.spyOn(api, "verifyManualAuth").mockReturnValue(
    new Promise<ManualAuthVerifyResult>((_resolve, reject) => {
      rejectVerification = reject;
    }),
  );
  renderUpstreams();
  await user.click(await screen.findByRole("button", { name: "更多操作" }));
  await user.click(await screen.findByRole("menuitem", { name: "恢复鉴权" }));
  fireEvent.change(screen.getByLabelText("Token", { exact: true }), {
    target: { value: "test-token" },
  });
  fireEvent.change(screen.getByLabelText("刷新 Token", { exact: true }), {
    target: { value: "test-refresh-token" },
  });
  fireEvent.click(screen.getByRole("button", { name: "验证并保存" }));
  await screen.findByRole("button", { name: "正在验证" });
  const dialog = screen.getByRole("dialog", { name: "恢复鉴权" });
  dialog.focus();
  await user.keyboard("{Escape}");
  expect(screen.getByRole("dialog", { name: "恢复鉴权" })).toBeVisible();
  rejectVerification(new Error("凭据验证被上游拒绝"));
  await waitFor(() => expect(screen.getByRole("button", { name: "验证并保存" })).toBeEnabled());
  dialog.focus();
  await user.keyboard("{Escape}");
  await waitFor(() =>
    expect(screen.queryByRole("dialog", { name: "恢复鉴权" })).not.toBeInTheDocument(),
  );
});
afterEach(() => {
  client?.clear();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

function renderUpstreams(): void {
  vi.spyOn(api, "upstreams").mockResolvedValue({
    hosts: [
      {
        upstream_id: "upstream-1",
        host: "api.example.test",
        hosts: ["api.example.test"],
        base_url: "https://api.example.test",
        name: "测试上游",
        upstream_type: "sub2api",
        account_count: 1,
        group_count: 1,
        auth_status: "已认证",
        raw_balance: "10",
        balance: "10",
        recharge_rate: "1",
        balance_status: "正常",
        checked_at: null,
      },
    ],
    total_hosts: 1,
    authenticated_hosts: 1,
    recovery_required: 0,
    source: "console",
  });
  vi.spyOn(api, "config").mockResolvedValue({
    database_available: true,
    data_database_available: true,
    mode: "完全模式",
    config_keys: [],
    secret_values_hidden: true,
    probes_enabled: false,
    admin_base_url: "https://management.example.test",
    request_timeout_seconds: 30,
    account_default_concurrency: 10,
    account_default_priority: 1,
    initialized: true,
    target_configured: true,
    console_username: "tester",
    configuration_errors: [],
  });
  vi.spyOn(api, "authRecoveryConfig").mockResolvedValue({ auth_records: [], vault_entries: [] });
  const root = createRootRoute();
  const route = createRoute({
    getParentRoute: () => root,
    path: "/upstreams",
    component: UpstreamsPage,
  });
  const router = createRouter({
    routeTree: root.addChildren([route]),
    history: createMemoryHistory({ initialEntries: ["/upstreams"] }),
  });
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  );
}

it.each([
  ["同步余额", "正在创建"],
  ["同步余额", "正在读取"],
  ["同步上游", "正在创建"],
  ["同步上游", "正在读取"],
])("%s 任务%s时按 Escape 保留任务窗口及后续状态", async (operation, phase) => {
  const user = userEvent.setup();
  const queued: Task = {
    id: "upstream-task",
    operation: "upstream-sync",
    skill: "upstreams",
    status: "queued",
    progress: 0,
    message: "等待处理",
    result: {},
    created_at: "2026-09-07T00:00:00Z",
    updated_at: "2026-09-07T00:00:00Z",
  };
  let resolveTask!: (value: Task) => void;
  const pendingTask = new Promise<Task>((resolve) => {
    resolveTask = resolve;
  });
  const create = vi.spyOn(api, operation === "同步余额" ? "runBalanceSync" : "runUpstreamSync");
  create.mockReturnValue(phase === "正在创建" ? pendingTask : Promise.resolve(queued));
  const readTask = vi.spyOn(api, "task").mockReturnValue(pendingTask);
  renderUpstreams();
  if (operation === "同步余额") {
    await user.click(await screen.findByRole("button", { name: "更多操作" }));
    await user.click(await screen.findByRole("menuitem", { name: "同步余额" }));
  } else {
    await user.click(await screen.findByRole("button", { name: "同步上游" }));
  }
  const dialog = await screen.findByRole("dialog", { name: operation });
  if (phase === "正在读取")
    await waitFor(() => expect(readTask).toHaveBeenCalledWith("upstream-task"));
  dialog.focus();
  await user.keyboard("{Escape}");
  expect(screen.getByRole("dialog", { name: operation })).toBeVisible();
  resolveTask({
    ...queued,
    status: "failed",
    message: "上游暂时不可用，请稍后重试",
    result: { error: "上游暂时不可用，请稍后重试" },
  });
  expect(await screen.findByText("上游暂时不可用，请稍后重试")).toBeVisible();
});
