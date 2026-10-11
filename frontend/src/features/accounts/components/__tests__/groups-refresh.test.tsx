import { seedDictionaries } from "@/test/dictionary-fixtures";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import type { AccountGroupsPreview } from "@/api";
import { AccountGroupsDialog } from "../account-groups-dialog";

const preview: AccountGroupsPreview = {
  account_id: "41",
  current_group_ids: ["7"],
  groups: [
    { id: "7", name: "原分组" },
    { id: "8", name: "目标组" },
    { id: "9", name: "新分组" },
  ],
  target_version: "target-1",
};
const clients: QueryClient[] = [];
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  vi.unstubAllGlobals();
});

function setup() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  seedDictionaries(client);
  clients.push(client);
  seedDictionaries(client, {
    group: preview.groups.map((group) => ({ value: group.id, name: group.name })),
  });
  client.setQueryData(["account-groups", "41"], preview);
  const pending: Array<(response: Response) => void> = [];
  const fetcher = vi.fn<typeof fetch>().mockImplementation((input, init) => {
    if (String(input) !== "/api/accounts/41/groups") throw new Error("Unexpected request");
    if (init?.method === "PUT") return Promise.resolve(Response.json({ id: "groups-task" }));
    return new Promise<Response>((resolve) => pending.push(resolve));
  });
  vi.stubGlobal("fetch", fetcher);
  render(
    <QueryClientProvider client={client}>
      <AccountGroupsDialog
        account={{ id: "41", name: "账号 A" }}
        onStarted={vi.fn()}
        onOpenChange={vi.fn()}
      />
    </QueryClientProvider>,
  );
  return {
    fetcher,
    async respond(index: number, value: AccountGroupsPreview = preview, status = 200) {
      await waitFor(() => expect(pending.length).toBe(index + 1));
      await act(async () => pending[index](Response.json(value, { status })));
      await waitFor(() =>
        expect(client.getQueryState(["account-groups", "41"])?.fetchStatus).toBe("idle"),
      );
    },
    async refresh() {
      await act(async () => {
        void client.invalidateQueries({ queryKey: ["account-groups", "41"], exact: true });
      });
    },
  };
}

async function selectTarget() {
  const user = userEvent.setup();
  await user.click(screen.getByRole("combobox", { name: "目标分组" }));
  await user.click(await screen.findByRole("option", { name: "目标组（#8）" }));
  await user.keyboard("{Escape}");
  return user;
}

it("缓存分组正在重读时保留选择与关闭入口，但禁止确认提交", async () => {
  const view = setup();
  const user = await selectTarget();
  expect(screen.getByRole("button", { name: "确认切换" })).toBeDisabled();
  expect(screen.getByRole("button", { name: "取消" })).toBeEnabled();
  expect(screen.getByRole("status", { name: "正在更新账号分组…" })).toBeVisible();
  await user.click(screen.getByRole("button", { name: "确认切换" }));
  expect(view.fetcher.mock.calls.every(([, init]) => !init?.method)).toBe(true);
  await view.respond(0);
  expect(screen.getByRole("button", { name: "确认切换" })).toBeEnabled();
});

it("当前分组后台变化时保留已编辑目标，提交使用最新预览的当前分组", async () => {
  const view = setup();
  await view.respond(0);
  const user = await selectTarget();
  await view.refresh();
  await view.respond(1, { ...preview, current_group_ids: ["9"] });
  expect(screen.getByText("当前分组：新分组")).toBeVisible();
  expect(screen.getByText("切换后分组：原分组、目标组")).toBeVisible();
  await user.click(screen.getByRole("button", { name: "确认切换" }));
  await waitFor(() =>
    expect(view.fetcher.mock.calls.some(([, init]) => init?.method === "PUT")).toBe(true),
  );
  const write = view.fetcher.mock.calls.find(([, init]) => init?.method === "PUT");
  expect(JSON.parse(String(write?.[1]?.body))).toEqual({
    group_ids: ["7", "8"],
    expected_group_ids: ["9"],
    target_version: "target-1",
  });
});

it("尚未编辑时后台变化同步默认选择，不把旧分组误作待提交草稿", async () => {
  const view = setup();
  await view.respond(0, { ...preview, current_group_ids: ["9"] });
  expect(screen.getByText("切换后分组：新分组")).toBeVisible();
  expect(screen.getByRole("button", { name: "确认切换" })).toBeDisabled();
});

it("后台读取失败时保留草稿并禁止提交，重新读取成功后允许继续", async () => {
  const view = setup();
  await view.respond(0);
  const user = await selectTarget();
  await view.refresh();
  await view.respond(1, preview, 503);
  expect(screen.getByText("切换后分组：原分组、目标组")).toBeVisible();
  expect(screen.getByRole("button", { name: "确认切换" })).toBeDisabled();
  await user.click(screen.getByRole("button", { name: "重新读取" }));
  await view.respond(2);
  expect(screen.getByRole("button", { name: "确认切换" })).toBeEnabled();
  expect(screen.getByText("切换后分组：原分组、目标组")).toBeVisible();
});

it("刷新移除已选目标组时保留草稿并标记无效，移除无效选择后可提交", async () => {
  const view = setup();
  await view.respond(0);
  const user = await selectTarget();
  await view.refresh();
  await view.respond(1, {
    ...preview,
    current_group_ids: ["9"],
    groups: preview.groups.filter((group) => group.id !== "8"),
  });
  expect(screen.getByRole("combobox", { name: "目标分组" })).toHaveAttribute(
    "aria-invalid",
    "true",
  );
  expect(screen.getByRole("button", { name: "确认切换" })).toBeDisabled();
  expect(screen.getByText("部分目标分组已不可用，请移除后重新选择")).toBeVisible();
  await user.click(screen.getByRole("combobox", { name: "目标分组" }));
  expect(screen.queryByRole("option", { name: "8" })).not.toBeInTheDocument();
  await user.keyboard("{Escape}");
  const trigger = screen.getByRole("combobox", { name: "目标分组" });
  await user.click(within(trigger).getAllByRole("button", { name: "移除" })[1]);
  expect(screen.getByRole("button", { name: "确认切换" })).toBeEnabled();
});

it("管理目标版本变化时重新初始化选择，不将旧目标草稿带入新目标", async () => {
  const view = setup();
  await view.respond(0);
  await selectTarget();
  await view.refresh();
  await view.respond(1, { ...preview, target_version: "target-2", current_group_ids: ["9"] });
  expect(screen.getByText("切换后分组：新分组")).toBeVisible();
  expect(screen.getByRole("button", { name: "确认切换" })).toBeDisabled();
});
