import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import userEvent from "@testing-library/user-event";

import { GroupsPage } from "@/App";
import { api, type GroupStatus } from "@/api";
import { policy } from "@/features/accounts/__tests__/fixtures";

const clients: QueryClient[] = [];
beforeEach(() => {
  vi.stubGlobal("PointerEvent", MouseEvent);
  const matches = Element.prototype.matches;
  vi.spyOn(Element.prototype, "matches").mockImplementation(function (
    this: Element,
    selector: string,
  ) {
    if ([":fullscreen", ":popover-open", ":modal"].includes(selector)) return false;
    return matches.call(this, selector);
  });
});
afterEach(() => {
  clients.forEach((client) => client.clear());
  clients.length = 0;
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

function group(id: string | null, name = `分组 ${id}`): GroupStatus {
  return {
    id,
    name,
    account_count: 1,
    scheduling_open: 1,
    scheduling_closed: 0,
    scheduling_unknown: 0,
    strategy: "balanced",
    strategy_source: "group_override",
    participation_status: "participating",
    participation_reason: null,
    status: "healthy",
    override: { strategy: "balanced" },
  };
}

function renderGroups(rows: GroupStatus[]) {
  vi.spyOn(api, "groups").mockResolvedValue(rows);
  vi.spyOn(api, "policy").mockResolvedValue(policy);
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  });
  clients.push(client);
  client.setQueryData(["groups"], rows);
  client.setQueryData(["policy"], policy);
  render(
    <QueryClientProvider client={client}>
      <GroupsPage />
    </QueryClientProvider>,
  );
  return client;
}

it("勾选分组后显示底部批量操作条，按 Esc 清空选择", () => {
  renderGroups([group("1")]);
  expect(screen.queryByRole("toolbar", { name: /批量操作/ })).not.toBeInTheDocument();
  fireEvent.click(screen.getByRole("checkbox", { name: "选择分组 分组 1" }));
  const toolbar = screen.getByRole("toolbar", { name: "已选择 1 个分组的批量操作" });
  expect(toolbar).toHaveClass("fixed", "bottom-6", "left-1/2");
  for (const name of ["回落到全局策略", "排除分组", "恢复管控"]) {
    expect(within(toolbar).getByRole("button", { name })).toBeEnabled();
  }
  fireEvent.keyDown(toolbar, { key: "Escape" });
  expect(screen.getByRole("checkbox", { name: "选择分组 分组 1" })).toHaveAttribute(
    "aria-checked",
    "false",
  );
});

it("顶部维护菜单未勾选时预览当前筛选分组，取消前不执行写入", async () => {
  const user = userEvent.setup();
  const control = vi.spyOn(api, "setGroupExcluded");
  renderGroups([group("1", "目标分组"), group("2", "其他分组"), group(null, "目标无 ID")]);
  fireEvent.change(screen.getByPlaceholderText("搜索分组、平台或策略"), {
    target: { value: "目标" },
  });
  screen.getByRole("button", { name: "分组维护" }).focus();
  await user.keyboard("{Enter}");
  await screen.findByRole("menu");
  expect(screen.getByText("当前筛选 1 个分组")).toBeVisible();
  await user.click(screen.getByRole("menuitem", { name: "排除分组" }));
  const dialog = screen.getByRole("dialog", { name: "批量排除分组" });
  expect(within(dialog).getByRole("list", { name: "本次处理的分组" })).toHaveTextContent(
    "目标分组（#1）",
  );
  expect(within(dialog).getAllByRole("listitem")).toHaveLength(1);
  expect(control).not.toHaveBeenCalled();
  await user.click(within(dialog).getByRole("button", { name: "取消" }));
  expect(control).not.toHaveBeenCalled();
});

it("顶部维护菜单已有勾选时只处理选中分组，确认后按稳定 ID 写入", async () => {
  const user = userEvent.setup();
  const control = vi.spyOn(api, "setGroupExcluded").mockResolvedValue(group("2"));
  renderGroups([group("1"), group("2")]);
  await user.click(screen.getByRole("checkbox", { name: "选择分组 分组 2" }));
  screen.getByRole("button", { name: "分组维护" }).focus();
  await user.keyboard("{Enter}");
  await screen.findByRole("menu");
  expect(screen.getByText("已选择 1 个分组")).toBeVisible();
  await user.click(screen.getByRole("menuitem", { name: "恢复管控" }));
  const dialog = screen.getByRole("dialog", { name: "批量恢复管控" });
  expect(within(dialog).getAllByRole("listitem")).toHaveLength(1);
  expect(dialog).toHaveTextContent("分组 2（#2）");
  expect(control).not.toHaveBeenCalled();
  await user.click(within(dialog).getByRole("button", { name: "确认处理 1 个分组" }));
  await waitFor(() => expect(control).toHaveBeenCalledWith("2", false));
});

it("当前页全选只选择有稳定 ID 的分组，跨页保留选择且筛选后清空", () => {
  renderGroups([
    group(null, "无 ID"),
    ...Array.from({ length: 21 }, (_, i) => group(String(i + 1))),
  ]);
  expect(screen.getByRole("checkbox", { name: "选择分组 无 ID" })).toHaveAttribute(
    "aria-disabled",
    "true",
  );
  fireEvent.click(screen.getByRole("checkbox", { name: "选择当前页分组" }));
  expect(screen.getByRole("toolbar", { name: "已选择 19 个分组的批量操作" })).toBeVisible();
  fireEvent.click(screen.getByRole("button", { name: "转到下一页" }));
  fireEvent.click(screen.getByRole("checkbox", { name: "选择分组 分组 20" }));
  expect(screen.getByRole("checkbox", { name: "选择当前页分组" })).toHaveAttribute(
    "aria-checked",
    "mixed",
  );
  expect(screen.getByRole("toolbar", { name: "已选择 20 个分组的批量操作" })).toBeVisible();
  fireEvent.change(screen.getByPlaceholderText("搜索分组、平台或策略"), {
    target: { value: "不存在" },
  });
  expect(screen.queryByRole("toolbar", { name: /批量操作/ })).not.toBeInTheDocument();
  expect(screen.getByRole("checkbox", { name: "选择当前页分组" })).toHaveAttribute(
    "aria-disabled",
    "true",
  );
});

it.each([
  { label: "回落到全局策略", excluded: null },
  { label: "排除分组", excluded: true },
  { label: "恢复管控", excluded: false },
])("批量$label先展示范围，确认后按稳定 ID 执行并清空选择", async ({ label, excluded }) => {
  const clear = vi.spyOn(api, "clearGroupPolicy").mockResolvedValue(group("1"));
  const control = vi.spyOn(api, "setGroupExcluded").mockResolvedValue(group("1"));
  renderGroups([group("1", "同名分组"), group("2", "同名分组")]);
  fireEvent.click(screen.getByRole("checkbox", { name: "选择当前页分组" }));
  fireEvent.click(
    within(screen.getByRole("toolbar", { name: /批量操作/ })).getByRole("button", { name: label }),
  );
  const dialog = screen.getByRole("dialog", { name: `批量${label}` });
  expect(dialog).toHaveTextContent("#1");
  expect(dialog).toHaveTextContent("#2");
  expect(clear).not.toHaveBeenCalled();
  expect(control).not.toHaveBeenCalled();
  fireEvent.click(within(dialog).getByRole("button", { name: "确认处理 2 个分组" }));
  await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  if (excluded === null) {
    expect(clear.mock.calls).toEqual([["1"], ["2"]]);
    expect(control).not.toHaveBeenCalled();
  } else {
    expect(control.mock.calls).toEqual([
      ["1", excluded],
      ["2", excluded],
    ]);
    expect(clear).not.toHaveBeenCalled();
  }
  expect(screen.queryByRole("toolbar", { name: /批量操作/ })).not.toBeInTheDocument();
});

it("批量请求部分失败时显示原因，并仅保留失败项供重试", async () => {
  const control = vi
    .spyOn(api, "setGroupExcluded")
    .mockResolvedValueOnce(group("1"))
    .mockRejectedValueOnce(new Error("分组已被移除，请刷新后重试"))
    .mockResolvedValueOnce(group("2"));
  renderGroups([group("1"), group("2")]);
  fireEvent.click(screen.getByRole("checkbox", { name: "选择当前页分组" }));
  fireEvent.click(
    within(screen.getByRole("toolbar", { name: /批量操作/ })).getByRole("button", {
      name: "排除分组",
    }),
  );
  fireEvent.click(screen.getByRole("button", { name: "确认处理 2 个分组" }));
  expect(await screen.findByRole("alert")).toHaveTextContent("分组已被移除，请刷新后重试");
  expect(screen.getByRole("alert")).toHaveTextContent("成功 1 个，失败 1 个");
  fireEvent.click(screen.getByRole("button", { name: "重试失败的 1 个分组" }));
  await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  expect(control.mock.calls).toEqual([
    ["1", true],
    ["2", true],
    ["2", true],
  ]);
});

it("执行中禁止重复提交和关闭确认窗口", async () => {
  let resolve!: (value: GroupStatus) => void;
  vi.spyOn(api, "clearGroupPolicy").mockReturnValue(
    new Promise((done) => {
      resolve = done;
    }),
  );
  renderGroups([group("1")]);
  fireEvent.click(screen.getByRole("checkbox", { name: "选择当前页分组" }));
  fireEvent.click(
    within(screen.getByRole("toolbar", { name: /批量操作/ })).getByRole("button", {
      name: "回落到全局策略",
    }),
  );
  fireEvent.click(screen.getByRole("button", { name: "确认处理 1 个分组" }));
  await waitFor(() => expect(screen.getByRole("button", { name: /处理中/ })).toBeDisabled());
  expect(screen.getByRole("button", { name: "取消" })).toBeDisabled();
  fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
  expect(screen.getByRole("dialog")).toBeVisible();
  await act(async () => resolve(group("1")));
});

it("键盘勾选分组后可取消确认，长名称在有滚动上限的范围列表中完整显示", async () => {
  const user = userEvent.setup();
  const clear = vi.spyOn(api, "clearGroupPolicy");
  const name = "包含很长名称的业务分组".repeat(20);
  renderGroups([group("1", name)]);
  screen.getByRole("checkbox", { name: `选择分组 ${name}` }).focus();
  await user.keyboard(" ");
  const toolbar = screen.getByRole("toolbar", { name: /批量操作/ });
  expect(toolbar).toHaveClass("max-w-[calc(100vw-2rem)]");
  await user.click(within(toolbar).getByRole("button", { name: "回落到全局策略" }));
  const list = screen.getByRole("list", { name: "本次处理的分组" });
  expect(list).toHaveClass("max-h-60", "overflow-y-auto");
  expect(within(list).getByRole("listitem")).toHaveTextContent(name);
  expect(within(list).getByRole("listitem")).toHaveClass("[overflow-wrap:anywhere]");
  await user.click(screen.getByRole("button", { name: "取消" }));
  expect(clear).not.toHaveBeenCalled();
  expect(screen.getByRole("checkbox", { name: `选择分组 ${name}` })).toHaveAttribute(
    "aria-checked",
    "true",
  );
});

it("全局策略不可用时禁用批量写入，刷新恢复后才可操作", async () => {
  const client = renderGroups([group("1")]);
  await act(async () => {
    client.setQueryData(["policy"], { ...policy, available: false });
  });
  fireEvent.click(screen.getByRole("checkbox", { name: "选择当前页分组" }));
  const toolbar = screen.getByRole("toolbar", { name: /批量操作/ });
  expect(within(toolbar).getByRole("button", { name: "排除分组" })).toBeDisabled();
  fireEvent.click(screen.getByRole("button", { name: "刷新分组" }));
  await waitFor(() =>
    expect(within(toolbar).getByRole("button", { name: "排除分组" })).toBeEnabled(),
  );
});
