import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { AccountList } from "../components/account-list";
import { RememberedAccountList } from "../components/remembered-account-list";
import { rememberedAccountKey, workbenchKeys } from "../constants";
import { template } from "./template-fixture";

let client: QueryClient;
afterEach(() => {
  cleanup();
  client?.clear();
  vi.unstubAllGlobals();
});
function mount(remembered = false): Array<Record<string, unknown>> {
  vi.stubGlobal("PointerEvent", MouseEvent);
  const writes: Array<Record<string, unknown>> = [];
  const account = { ...template.summary, id: "41", name: "团队账号", schedulable: true };
  const saved = {
    id: "saved",
    account_id: "41",
    name: "团队账号",
    email: "owner@example.test",
    revision: 3,
    login_available: false,
    last_imported_at: "2026-10-11T01:00:00Z",
    plan: "plus",
  };
  client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } });
  client.setQueryData(workbenchKeys.accounts, [account]);
  client.setQueryData(rememberedAccountKey, [saved]);
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input, init) => {
      const path = String(input);
      if (path.endsWith("/action-preview"))
        return Response.json({
          account_id: "41",
          account_version: "account-v1",
          target_version: "target-v1",
          platform_present: true,
          schedulable: true,
          remembered: saved,
        });
      if (init?.method === "POST") {
        writes.push(JSON.parse(String(init.body)));
        return Response.json({ id: "task", status: "running", result: {} });
      }
      if (path.includes("/tasks/"))
        return Response.json({
          id: "task",
          status: "succeeded",
          message: "账号操作完成",
          result: {},
        });
      return Response.json([]);
    }),
  );
  render(
    <QueryClientProvider client={client}>
      {remembered ? <RememberedAccountList /> : <AccountList />}
    </QueryClientProvider>,
  );
  return writes;
}
it("平台删除默认保留已记住账号，勾选后按预览版本提交联动删除", async () => {
  const writes = mount();
  const user = userEvent.setup();
  const remove = screen.getByRole("button", { name: "删除 团队账号" });
  expect(remove).toHaveClass("size-8");
  await user.click(remove);
  const option = await screen.findByRole("checkbox", { name: "同时删除已记住账号" });
  expect(option).not.toBeChecked();
  await user.click(option);
  await user.click(screen.getByRole("button", { name: "确认删除" }));
  await waitFor(() => expect(writes).toHaveLength(1));
  expect(writes[0]).toMatchObject({
    action: "delete",
    delete_remembered: true,
    remembered_revision: 3,
    account_version: "account-v1",
    target_version: "target-v1",
  });
});
it("删除已记住账号可选择同时删除稳定 ID 对应的平台账号", async () => {
  const writes = mount(true);
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "删除 团队账号" }));
  const option = screen.getByRole("checkbox", { name: "同时删除平台账号" });
  expect(option).not.toBeChecked();
  await user.click(option);
  await waitFor(() => expect(screen.getByRole("button", { name: "确认删除" })).toBeEnabled());
  await user.click(screen.getByRole("button", { name: "确认删除" }));
  await waitFor(() => expect(writes).toHaveLength(1));
  expect(writes[0]).toMatchObject({
    action: "delete",
    delete_remembered: true,
    remembered_revision: 3,
  });
});
it("调度开关支持键盘操作，确认前不写入且取消保留原状态", async () => {
  const writes = mount();
  const user = userEvent.setup();
  const toggle = screen.getByRole("switch", { name: "团队账号 调度开关" });
  expect(toggle).toBeChecked();
  toggle.focus();
  await user.keyboard(" ");
  const dialog = await screen.findByRole("dialog", { name: "关闭调度" });
  await user.click(within(dialog).getByRole("button", { name: "返回" }));
  expect(writes).toHaveLength(0);
  expect(toggle).toBeChecked();
  await user.click(toggle);
  await waitFor(() => expect(screen.getByRole("button", { name: "确认关闭" })).toBeEnabled());
  await user.click(screen.getByRole("button", { name: "确认关闭" }));
  await waitFor(() => expect(writes).toHaveLength(1));
  expect(writes[0]).toMatchObject({
    action: "scheduling",
    schedulable: false,
    delete_remembered: false,
  });
});

it("影响范围读取期间禁止提交，关闭窗口取消读取", async () => {
  mount();
  let signal: AbortSignal | null | undefined;
  vi.stubGlobal(
    "fetch",
    vi.fn((_input, init) => {
      signal = init?.signal;
      return new Promise<Response>(() => {});
    }),
  );
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "删除 团队账号" }));
  expect(screen.getByRole("status", { name: "正在核对账号影响范围" })).toBeVisible();
  expect(screen.getByRole("button", { name: "确认删除" })).toBeDisabled();
  await user.click(screen.getByRole("button", { name: "返回" }));
  expect(signal?.aborted).toBe(true);
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
});

it("影响范围读取失败时保留取消和重试入口且禁止删除", async () => {
  mount();
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => Response.json({ detail: "站点暂时不可用" }, { status: 503 })),
  );
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "删除 团队账号" }));
  expect(await screen.findByRole("button", { name: "重新读取" })).toBeEnabled();
  expect(screen.getByRole("button", { name: "确认删除" })).toBeDisabled();
  expect(screen.getByRole("button", { name: "返回" })).toBeEnabled();
});

it("平台删除不勾选联动时只提交平台删除且执行期间禁止重复操作", async () => {
  mount();
  const writes: unknown[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input, init) => {
      if (String(input).endsWith("/action-preview"))
        return Response.json({
          account_id: "41",
          account_version: "a",
          target_version: "t",
          platform_present: true,
          schedulable: true,
          remembered: null,
        });
      if (init?.method === "POST") writes.push(JSON.parse(String(init.body)));
      return Response.json({ id: "running-task", status: "running", result: {} });
    }),
  );
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "删除 团队账号" }));
  await waitFor(() => expect(screen.getByRole("button", { name: "确认删除" })).toBeEnabled());
  expect(screen.queryByRole("checkbox", { name: "同时删除已记住账号" })).not.toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "确认删除" }));
  expect(await screen.findByText("正在执行账号操作…")).toBeVisible();
  expect(screen.getByRole("button", { name: "确认删除" })).toBeDisabled();
  expect(screen.getByRole("button", { name: "返回" })).toBeDisabled();
  expect(writes).toEqual([
    {
      action: "delete",
      account_version: "a",
      target_version: "t",
      delete_remembered: false,
      remembered_revision: 0,
    },
  ]);
});
