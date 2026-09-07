import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  createMemoryHistory,
  createRootRoute,
  createRouter,
  RouterProvider,
} from "@tanstack/react-router";
import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterAll, afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { UpstreamsPage } from "../../App";
import type { PrivateAuthConfigStatus, Task, UpstreamSummary } from "../../api";

const restoreSelectorMatching = vi.hoisted(() => {
  const matches = Element.prototype.matches;
  // NWSAPI captures this method during App imports, before beforeEach runs.
  Element.prototype.matches = function (selector: string): boolean {
    if ([":fullscreen", ":popover-open", ":modal"].includes(selector)) return false;
    return matches.call(this, selector);
  };
  return () => {
    Element.prototype.matches = matches;
  };
});
afterAll(restoreSelectorMatching);

const host = "api.example.test";
const clients: QueryClient[] = [];

beforeEach(() => {
  vi.stubGlobal("PointerEvent", MouseEvent);
  vi.spyOn(window, "scrollTo").mockImplementation(() => undefined);
  // JSDOM 26 recurses on top-layer and native select picker selectors.
  const getComputedStyle = window.getComputedStyle;
  vi.spyOn(window, "getComputedStyle").mockImplementation((element, pseudoElement) => {
    if (element instanceof HTMLSelectElement) {
      const style = document.createElement("div").style;
      style.display = "none";
      return style;
    }
    return getComputedStyle(element, pseudoElement);
  });
});
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

function json(value: unknown): Response {
  return new Response(JSON.stringify(value), { headers: { "Content-Type": "application/json" } });
}

async function renderUpstreams(): Promise<{ startTask: () => void; completeTask: () => void }> {
  const upstreams: UpstreamSummary = {
    hosts: [
      {
        upstream_id: "upstream-fixture",
        host,
        hosts: [host],
        base_url: `https://${host}`,
        name: "测试上游",
        upstream_type: "sub2api",
        account_count: 1,
        group_count: 1,
        auth_status: "已鉴权",
        raw_balance: "10",
        balance: "10",
        recharge_rate: "1",
        balance_status: "已读取",
        checked_at: null,
      },
    ],
    total_hosts: 1,
    authenticated_hosts: 1,
    recovery_required: 0,
    source: "test",
  };
  const auth: PrivateAuthConfigStatus = {
    auth_records: [],
    vault_entries: [
      {
        entry: "测试凭据",
        hosts: [host],
        has_username: true,
        has_password: true,
        username_is_email: true,
        header_names: [],
      },
    ],
  };
  let task: Task = {
    id: "upstream-action-fixture",
    skill: "upstream",
    operation: "test",
    status: "running",
    progress: 50,
    message: "正在处理测试上游",
    result: {},
    created_at: "2026-09-07T12:00:00Z",
    updated_at: "2026-09-07T12:00:00Z",
  };
  let startTask: () => void = () => {
    throw new Error("任务尚未启动");
  };
  const taskCreated = new Promise<Response>((resolve) => {
    startTask = () => resolve(json(task));
  });
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const path = String(input);
      if (
        init?.method === "POST" &&
        (path.endsWith("/balance-sync") || path === "/api/auth-recovery/run")
      )
        return taskCreated;
      if (path === `/api/tasks/${task.id}`) return json(task);
      if (path === "/api/upstreams") return json(upstreams);
      if (path === "/api/auth-recovery/config") return json(auth);
      if (path === "/api/config") return json({ mode: "完全模式" });
      throw new Error(`Unexpected test request: ${path}`);
    }),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  });
  clients.push(client);
  client.setQueryData(["upstreams"], upstreams);
  client.setQueryData(["auth-recovery-config"], auth);
  client.setQueryData(["config"], { mode: "完全模式" });
  const router = createRouter({
    routeTree: createRootRoute({ component: UpstreamsPage }),
    history: createMemoryHistory({ initialEntries: ["/"] }),
  });
  await act(async () => {
    await router.load();
    render(
      <QueryClientProvider client={client}>
        <RouterProvider router={router} />
      </QueryClientProvider>,
    );
  });
  return {
    startTask,
    completeTask: () => {
      task = {
        ...task,
        status: "succeeded",
        progress: 100,
        message: "测试任务已完成",
        result: { host, balance: "20" },
      };
    },
  };
}

describe("上游单项任务弹窗", () => {
  it.each([
    { action: "同步余额", phase: "创建中" },
    { action: "同步余额", phase: "执行中" },
    { action: "恢复鉴权", phase: "创建中" },
    { action: "恢复鉴权", phase: "执行中" },
  ])("$action 任务$phase按 Escape 时保留跟踪，完成后允许关闭", async (fixture) => {
    const user = userEvent.setup();
    const state = await renderUpstreams();
    await user.click(await screen.findByRole("button", { name: "更多操作" }));
    await user.click(await screen.findByRole("menuitem", { name: fixture.action }));
    const dialog = await screen.findByRole("dialog", { name: fixture.action });
    if (fixture.action === "恢复鉴权") {
      await user.click(within(dialog).getByRole("combobox"));
      await user.click(await screen.findByRole("option", { name: "密码箱登录" }));
      fireEvent.click(within(dialog).getByRole("button", { name: "开始恢复" }));
    }
    const runningText = fixture.action === "恢复鉴权" ? "正在恢复鉴权" : "正在同步余额";
    if (fixture.phase === "执行中") {
      await act(async () => state.startTask());
      await waitFor(() =>
        expect(within(dialog).getAllByText(runningText).length).toBeGreaterThan(0),
      );
    } else {
      await within(dialog).findByText(
        fixture.action === "恢复鉴权" ? "正在创建鉴权恢复任务" : "正在创建余额同步任务",
      );
    }

    fireEvent.keyDown(dialog, { key: "Escape" });

    expect(screen.getByRole("dialog", { name: fixture.action })).toBeVisible();
    if (fixture.phase === "创建中") {
      await act(async () => state.startTask());
      await waitFor(() =>
        expect(within(dialog).getAllByText(runningText).length).toBeGreaterThan(0),
      );
    }
    state.completeTask();
    await waitFor(() => expect(within(dialog).getByRole("button", { name: "关闭" })).toBeVisible());
    expect(within(dialog).queryByText(runningText)).not.toBeInTheDocument();

    fireEvent.keyDown(dialog, { key: "Escape" });

    await waitFor(() =>
      expect(screen.queryByRole("dialog", { name: fixture.action })).not.toBeInTheDocument(),
    );
  });
});
