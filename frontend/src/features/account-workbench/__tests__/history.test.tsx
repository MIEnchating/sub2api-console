import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Task } from "@/api";
import { WorkbenchHistory } from "../components/workbench-history";

const clients: QueryClient[] = [];
beforeEach(() => vi.stubGlobal("PointerEvent", MouseEvent));
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

function task(id: string, status: Task["status"]): Task {
  return {
    id,
    status,
    skill: "account-workbench",
    operation: "account-workbench-import",
    message: `${id} 处理结果`,
    progress: status === "running" ? 20 : 100,
    result: { email: `${id}@example.com` },
    created_at: "2026-09-14T00:00:00Z",
    updated_at: "2026-09-14T00:01:00Z",
  };
}

function mount(
  initial: Task[],
  fail = false,
  active = initial.filter((item) => ["running", "queued", "waiting_input"].includes(item.status)),
): {
  update: (tasks: Task[]) => void;
  writes: {
    path: string;
    body: { items: { id: string; updated_at: string }[]; confirmed: boolean };
  }[];
} {
  let tasks = initial;
  const writes: {
    path: string;
    body: { items: { id: string; updated_at: string }[]; confirmed: boolean };
  }[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn<typeof fetch>(async (input, init) => {
      const path = String(input);
      if (init?.method === "POST") {
        const body = JSON.parse(String(init.body)) as (typeof writes)[number]["body"];
        writes.push({ path, body });
        if (path.endsWith("/reauthorization/preview"))
          return Response.json({
            id: "fresh-preview",
            target: "https://sub2api.example",
            expires_at: new Date(Date.now() + 600000).toISOString(),
            items: [],
            errors: [],
            fresh_login: true,
          });
        if (fail)
          return Response.json({ detail: "处理记录已变化，请刷新后重新选择" }, { status: 409 });
        if (path.endsWith("/delete")) {
          tasks = tasks.filter((value) => !body.items.some((item) => item.id === value.id));
          return Response.json({ deleted: body.items.length });
        }
        return Response.json({
          items: body.items.map((item) => ({
            id: item.id,
            cancelled: true,
            message: "已请求取消",
          })),
        });
      }
      if (path.endsWith("/history")) return Response.json(tasks);
      if (path.endsWith("/history/active")) return Response.json(active);
      if (path.includes("/tasks/"))
        return Response.json(tasks.find((item) => path.endsWith(`/${item.id}`)));
      return Response.json({ detail: "未配置此隔离请求" }, { status: 503 });
    }),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  clients.push(client);
  render(
    <QueryClientProvider client={client}>
      <WorkbenchHistory />
    </QueryClientProvider>,
  );
  return { writes, update: (values) => (tasks = values) };
}

describe("账号工作台处理记录", () => {
  it.each([
    ["account-workbench-import", "从此记录重新生成授权文件"],
    ["account-workbench-oauth-batch", "从此记录重新授权"],
  ])("打开进行中的 %s 任务后详情返回终态会显示 %s 入口", async (operation, label) => {
    const source = { ...task("finishing", "running"), operation };
    const state = mount([source]);
    const user = userEvent.setup();
    const details = await screen.findByRole("button", { name: "查看任务 finishing" });
    expect(screen.queryByRole("button", { name: label })).not.toBeInTheDocument();

    state.update([
      {
        ...source,
        status: "succeeded",
        progress: 100,
        result: { items: [{ index: 0, profile_id: "profile-101", status: "succeeded" }] },
      },
    ]);
    await user.click(details);

    expect(await screen.findByRole("button", { name: label })).toBeEnabled();
  });

  it("历史授权记录经显式预览创建全新授权范围且不复用旧会话", async () => {
    const source = task("prior-batch", "failed");
    source.operation = "account-workbench-oauth-batch";
    source.result = {
      items: [
        {
          index: 0,
          account_id: "101",
          user_id: "user-101",
          workspace_id: "workspace-101",
          profile_id: "profile-101",
          email: "owner@example.com",
          status: "failed",
        },
      ],
    };
    const state = mount([source]);
    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: "查看任务 prior-batch" }));
    await user.click(screen.getByRole("button", { name: "从此记录重新授权" }));
    expect(state.writes).toHaveLength(0);
    await user.click(screen.getByRole("button", { name: "预览历史账号重新授权" }));
    await waitFor(() => expect(state.writes).toHaveLength(1));
    expect(state.writes[0]).toEqual({
      path: "/api/account-workbench/reauthorization/preview",
      body: { account_ids: [], source_task_id: "prior-batch", fresh_login: true },
    });
  });
  it("按结果邮箱搜索后全选只选择匹配记录并在确认后按版本删除", async () => {
    const state = mount([
      task("alice", "succeeded"),
      task("bob", "failed"),
      task("active", "running"),
    ]);
    const user = userEvent.setup();
    await user.type(
      await screen.findByRole("textbox", { name: "搜索处理记录" }),
      "ALICE@example.com",
    );
    expect(screen.queryByRole("button", { name: "查看任务 bob" })).not.toBeInTheDocument();
    const select = screen.getByRole("checkbox", { name: /^选择筛选结果/ });
    select.focus();
    await user.keyboard(" ");
    expect(select).toBeChecked();
    await user.click(screen.getByRole("button", { name: "删除选中记录（1）" }));
    const dialog = screen.getByRole("dialog", { name: "确认删除处理记录" });
    expect(dialog).toHaveTextContent("ID：alice");
    expect(dialog).not.toHaveTextContent("ID：active");
    expect(state.writes).toHaveLength(0);
    await user.click(within(dialog).getByRole("button", { name: "确认执行" }));
    await screen.findByText("没有匹配的处理记录，请调整筛选条件");
    expect(state.writes[0]).toEqual({
      path: "/api/account-workbench/history/delete",
      body: { confirmed: true, items: [{ id: "alice", updated_at: "2026-09-14T00:01:00Z" }] },
    });
    await user.clear(screen.getByRole("textbox", { name: "搜索处理记录" }));
    expect(screen.getByRole("button", { name: "查看任务 bob" })).toBeInTheDocument();
  });

  it("取消全部任务从完整范围读取列表外旧任务且确认前不取消", async () => {
    const state = mount([task("done", "succeeded")], false, [
      task("active", "running"),
      task("waiting", "waiting_input"),
    ]);
    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: "取消全部工作台任务" }));
    let dialog = screen.getByRole("dialog", { name: "确认取消全部工作台任务" });
    await within(dialog).findByText("ID：waiting");
    expect(dialog).toHaveTextContent("ID：waiting");
    expect(dialog).not.toHaveTextContent("ID：done");
    await user.click(within(dialog).getByRole("button", { name: "返回" }));
    expect(state.writes).toHaveLength(0);
    await user.click(screen.getByRole("button", { name: "取消全部工作台任务" }));
    dialog = screen.getByRole("dialog", { name: "确认取消全部工作台任务" });
    await within(dialog).findByText("ID：waiting");
    await user.click(within(dialog).getByRole("button", { name: "确认取消这些任务" }));
    await waitFor(() => expect(state.writes).toHaveLength(1));
    expect(state.writes[0].body.items.map((item) => item.id)).toEqual(["active", "waiting"]);
    expect(state.writes[0].path).toBe("/api/account-workbench/history/cancel");
  });

  it("删除失败时保留记录和选择并恢复确认按钮", async () => {
    mount([task("changed", "failed")], true);
    const user = userEvent.setup();
    await user.click(await screen.findByRole("checkbox", { name: "选择处理记录 changed" }));
    await user.click(screen.getByRole("button", { name: "删除选中记录（1）" }));
    const dialog = screen.getByRole("dialog", { name: "确认删除处理记录" });
    await user.click(within(dialog).getByRole("button", { name: "确认执行" }));
    await waitFor(() =>
      expect(within(dialog).getByRole("button", { name: "确认执行" })).toBeEnabled(),
    );
    await user.click(within(dialog).getByRole("button", { name: "返回" }));
    expect(screen.getByRole("checkbox", { name: "选择处理记录 changed" })).toBeChecked();
    expect(screen.getByRole("button", { name: "查看任务 changed" })).toBeInTheDocument();
  });

  it("没有记录时显示空态且批量操作不可用", async () => {
    mount([]);
    await screen.findByText("暂无账号处理记录");
    expect(screen.getByRole("button", { name: "删除选中记录（0）" })).toBeDisabled();
    await userEvent.setup().click(screen.getByRole("button", { name: "取消全部工作台任务" }));
    await screen.findByText("共 0 个活动任务");
    expect(screen.getByRole("button", { name: "确认取消这些任务" })).toBeDisabled();
    expect(screen.queryByRole("checkbox", { name: /^选择筛选结果/ })).not.toBeInTheDocument();
  });
});
