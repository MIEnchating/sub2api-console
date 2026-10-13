import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { AccountList } from "../components/account-list";
import { workbenchKeys } from "../constants";
import type { WorkbenchAccount } from "../types";

const account: WorkbenchAccount = {
  id: "41",
  name: "团队账号",
  email: "owner@example.test",
  status: "active",
  schedulable: true,
  plan: "plus",
  groups: [{ id: "7", name: "原组" }],
  proxy_name: "",
  concurrency: "0",
  load_factor: "",
  rate_multiplier: "1",
  model_mapping: {},
  fingerprint: "",
};
const preview = {
  account_id: "41",
  current_group_ids: ["7"],
  target_version: "target-1",
  account_version: "account-1",
  groups: [
    { id: "7", name: "原组" },
    { id: "8", name: "空目标组" },
  ],
};
let client: QueryClient;
afterEach(() => {
  cleanup();
  client?.clear();
  vi.unstubAllGlobals();
});
function setup(
  options: {
    empty?: boolean;
    readFailure?: boolean;
    writeFailure?: boolean;
    taskStatus?: string;
  } = {},
) {
  let moved = false;
  let reads = 0;
  const writes: unknown[] = [];
  const fetcher = vi.fn(async (input: RequestInfo | URL, init?: RequestInit): Promise<Response> => {
    const url = String(input);
    if (url === "/api/account-workbench/accounts/41/groups") {
      if (init?.method === "PUT") {
        writes.push(JSON.parse(String(init.body)));
        if (options.writeFailure)
          return Response.json(
            { error: { message: "账号分组已变化，请重新打开分组选择" } },
            { status: 409 },
          );
        return Response.json({ id: "move-task", status: "queued", result: {} });
      }
      reads++;
      if (options.readFailure && reads === 1) return Response.json({}, { status: 503 });
      return Response.json(
        options.empty ? { ...preview, groups: [], current_group_ids: [] } : preview,
      );
    }
    if (url === "/api/tasks/move-task") {
      moved = options.taskStatus !== "failed";
      return Response.json({
        id: "move-task",
        status: options.taskStatus ?? "succeeded",
        message: "分组未确认，请重新核对",
        result: {},
      });
    }
    if (url === "/api/account-workbench/accounts")
      return Response.json([
        { ...account, groups: moved ? [{ id: "8", name: "空目标组" }] : account.groups },
      ]);
    throw new Error(`Unexpected request: ${url}`);
  });
  vi.stubGlobal("fetch", fetcher);
  client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity }, mutations: { retry: false } },
  });
  client.setQueryData(workbenchKeys.accounts, [account]);
  render(
    <QueryClientProvider client={client}>
      <AccountList />
    </QueryClientProvider>,
  );
  return { writes, fetcher };
}
async function open() {
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "移动 团队账号 分组" }));
  await screen.findByRole("button", { name: "确认移动" });
  return user;
}
async function chooseTarget(user: ReturnType<typeof userEvent.setup>) {
  await user.click(screen.getByRole("combobox", { name: "目标分组" }));
  await user.click(await screen.findByRole("option", { name: "原组（#7）" }));
  await user.click(screen.getByRole("option", { name: "空目标组（#8）" }));
  expect(screen.getByRole("option", { name: "空目标组（#8）" })).toHaveAttribute(
    "aria-selected",
    "true",
  );
  await user.keyboard("{Escape}");
}

it("操作列打开移动分组，确认替换后等待任务完成并刷新账号分组", async () => {
  const f = setup();
  const user = await open();
  expect(screen.getByRole("button", { name: "确认移动" })).toBeDisabled();
  await chooseTarget(user);
  expect(screen.getByText("移动后分组：空目标组")).toBeVisible();
  await user.click(screen.getByRole("button", { name: "确认移动" }));
  await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  expect(
    await within(screen.getByRole("table", { name: "账号列表" })).findByText("空目标组"),
  ).toBeVisible();
  expect(f.writes).toEqual([
    {
      group_ids: ["8"],
      expected_group_ids: ["7"],
      target_version: "target-1",
      account_version: "account-1",
    },
  ]);
  expect(f.fetcher.mock.calls.some(([url]) => String(url).includes("dictionaries"))).toBe(false);
});

it("选择后取消关闭弹窗且不提交分组变更", async () => {
  const f = setup();
  const user = await open();
  await chooseTarget(user);
  await user.click(screen.getByRole("button", { name: "取消" }));
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  expect(f.writes).toEqual([]);
});

it("清空目标分组后禁止确认，键盘可以重新打开选项", async () => {
  setup();
  const user = await open();
  const trigger = screen.getByRole("combobox", { name: "目标分组" });
  trigger.focus();
  await user.keyboard("{Enter}");
  expect(trigger).toHaveAttribute("aria-expanded", "true");
  await user.click(screen.getByRole("button", { name: "清空选择" }));
  expect(screen.getByRole("button", { name: "确认移动" })).toBeDisabled();
  expect(screen.getByText("移动后分组：未分组")).toBeVisible();
});

it("没有可用远端分组时解释空状态并禁用选择及提交", async () => {
  setup({ empty: true });
  await open();
  expect(screen.getByText("没有可用的目标分组，请先在管理平台添加或启用兼容分组。")).toBeVisible();
  expect(screen.getByRole("combobox", { name: "目标分组" })).toHaveAttribute(
    "aria-disabled",
    "true",
  );
  expect(screen.getByRole("button", { name: "确认移动" })).toBeDisabled();
});

it("读取分组失败保留关闭与重试，重试后展示远端选项", async () => {
  setup({ readFailure: true });
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "移动 团队账号 分组" }));
  await user.click(await screen.findByRole("button", { name: "重新读取" }));
  expect(screen.getByRole("button", { name: "关闭" })).toBeEnabled();
  expect(await screen.findByText("当前分组：原组")).toBeVisible();
});

it("提交遇到版本冲突时保留选择并允许重新读取", async () => {
  const f = setup({ writeFailure: true });
  const user = await open();
  await chooseTarget(user);
  await user.click(screen.getByRole("button", { name: "确认移动" }));
  await waitFor(() => expect(screen.getByRole("button", { name: "确认移动" })).toBeEnabled());
  expect(screen.getByText("移动后分组：空目标组")).toBeVisible();
  expect(screen.getByRole("button", { name: "重新读取" })).toBeEnabled();
  expect(f.writes).toHaveLength(1);
});

it("后台移动失败仍刷新远端列表并恢复操作按钮，不自动重放写入", async () => {
  const f = setup({ taskStatus: "failed" });
  const user = await open();
  await chooseTarget(user);
  await user.click(screen.getByRole("button", { name: "确认移动" }));
  await waitFor(() =>
    expect(f.fetcher.mock.calls.some(([url]) => url === "/api/account-workbench/accounts")).toBe(
      true,
    ),
  );
  expect(screen.getByRole("button", { name: "移动 团队账号 分组" })).toBeEnabled();
  expect(f.writes).toHaveLength(1);
});

it("分组读取未完成时显示忙碌提示且可以关闭，迟到结果不重新打开弹窗", async () => {
  const f = setup();
  let resolveRead!: (response: Response) => void;
  f.fetcher.mockImplementationOnce(
    () =>
      new Promise<Response>((resolve) => {
        resolveRead = resolve;
      }),
  );
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "移动 团队账号 分组" }));
  expect(screen.getByRole("status", { name: "正在读取账号分组…" })).toBeVisible();
  expect(screen.queryByRole("button", { name: "确认移动" })).not.toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "关闭" }));
  await act(async () => resolveRead(Response.json(preview)));
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  expect(f.writes).toEqual([]);
});

it("任务尚未完成时显示忙碌状态并防止再次移动，终态到达后恢复入口", async () => {
  const f = setup();
  const fetcher = f.fetcher.getMockImplementation()!;
  let resolveTask!: (response: Response) => void;
  f.fetcher.mockImplementation((input, init) => {
    if (String(input) === "/api/tasks/move-task")
      return new Promise<Response>((resolve) => {
        resolveTask = resolve;
      });
    return fetcher(input, init);
  });
  const user = await open();
  await chooseTarget(user);
  await user.click(screen.getByRole("button", { name: "确认移动" }));
  expect(await screen.findByRole("status", { name: "正在移动分组…" })).toBeVisible();
  expect(screen.getByRole("button", { name: "移动 团队账号 分组" })).toBeDisabled();
  await waitFor(() => expect(resolveTask).toBeDefined());
  await act(async () =>
    resolveTask(Response.json({ id: "move-task", status: "succeeded", result: {} })),
  );
  await waitFor(() =>
    expect(screen.getByRole("button", { name: "移动 团队账号 分组" })).toBeEnabled(),
  );
  expect(f.writes).toHaveLength(1);
});
