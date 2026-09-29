import type { QueryClient } from "@tanstack/react-query";
import { act, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";

import { api } from "@/api";
import { toast } from "sonner";

import { boundCandidate, renderOnboarding } from "./onboarding-fixture";

let client: QueryClient | undefined;
afterEach(() => {
  client?.clear();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

it.each(["active", "disabled"])("直达 %s 的已有绑定分组时允许预览本地分组变更", async (status) => {
  vi.stubGlobal("PointerEvent", MouseEvent);
  client = renderOnboarding(boundCandidate(status));
  fireEvent.click(await screen.findByRole("combobox", { name: "已有绑定分组 本地分组" }));
  fireEvent.click(await screen.findByRole("option", { name: /备用分组/ }));
  fireEvent.keyDown(screen.getByRole("combobox", { name: "已有绑定分组 本地分组" }), {
    key: "Escape",
  });
  fireEvent.click(screen.getByRole("button", { name: "预览更新绑定" }));
  const confirmation = await screen.findByRole("dialog", { name: "确认账号绑定变更" });
  expect(within(confirmation).getByText("原分组、备用分组", { exact: true })).toBeVisible();
  expect(within(confirmation).getByText("待更新", { exact: true })).toBeVisible();
});

it("直达分组时本地分组尚未返回则显示读取状态且不误报平台不匹配", async () => {
  let resolveGroups!: (groups: import("@/api").GroupStatus[]) => void;
  client = renderOnboarding(boundCandidate("active"));
  vi.mocked(api.groups).mockImplementation(
    () =>
      new Promise((resolve) => {
        resolveGroups = resolve;
      }),
  );
  const errorToast = vi.spyOn(toast, "error");
  expect(await screen.findByText("正在读取本地分组")).toBeVisible();
  expect(errorToast).not.toHaveBeenCalled();
  await act(async () => resolveGroups([]));
  expect(await screen.findByRole("button", { name: "刷新本地分组" })).toBeVisible();
});

it("从统计变化进入开户后提供返回统计变化入口", async () => {
  client = renderOnboarding(boundCandidate("active"), true, undefined, { history: "overview" });
  fireEvent.click(await screen.findByRole("button", { name: "返回统计变化" }));
  expect(await screen.findByText("返回上游管理完成")).toBeVisible();
});

it("Claude 上游分组在存在 Anthropic 或 Composite 本地分组时可以选择并预览", async () => {
  vi.stubGlobal("PointerEvent", MouseEvent);
  const candidate = {
    ...boundCandidate("active"),
    group_name: "Claude Max｜蒸馏组",
    platform: "anthropic",
    bound: false,
    bound_accounts: [],
    can_create_key: true,
  };
  client = renderOnboarding(candidate);
  const groups = await api.groups();
  vi.mocked(api.groups).mockResolvedValue(
    groups.map((group, index) => ({
      ...group,
      name: index === 0 ? "CCMAX-特价" : "国产-平价",
      platform: index === 0 ? "anthropic" : "composite",
    })),
  );
  await act(async () => {
    await client!.invalidateQueries({ queryKey: ["groups"] });
  });
  const selector = await screen.findByRole("combobox", { name: "Claude Max｜蒸馏组 本地分组" });
  fireEvent.click(selector);
  fireEvent.click(await screen.findByRole("option", { name: /CCMAX-特价/ }));
  fireEvent.keyDown(selector, { key: "Escape" });
  expect(screen.getByRole("button", { name: "预览添加账号" })).toBeEnabled();
});

it("本地分组为空后刷新取得兼容分组时重新提供选择", async () => {
  vi.stubGlobal("PointerEvent", MouseEvent);
  client = renderOnboarding(boundCandidate("active"), true, []);
  const refresh = await screen.findByRole("button", { name: "刷新本地分组" });
  vi.mocked(api.groups).mockResolvedValue([
    {
      id: "1",
      name: "已同步分组",
      platform: "openai",
      account_count: 0,
      scheduling_open: 0,
      scheduling_closed: 0,
      scheduling_unknown: 0,
      strategy: "balanced",
      strategy_source: "global_default",
      participation_status: "participating",
      participation_reason: null,
      status: "healthy",
    },
  ]);
  fireEvent.click(refresh);
  const selector = await screen.findByRole("combobox", { name: "已有绑定分组 本地分组" });
  await waitFor(() =>
    expect(screen.queryByRole("button", { name: "刷新本地分组" })).not.toBeInTheDocument(),
  );
  fireEvent.click(selector);
  expect(await screen.findByRole("option", { name: /已同步分组/ })).toBeVisible();
});

it("空目录缓存刷新失败时只提示读取失败而不误报平台不兼容", async () => {
  const errorToast = vi.spyOn(toast, "error");
  client = renderOnboarding(boundCandidate("active"), true, []);
  client.setQueryData(["groups"], []);
  vi.mocked(api.groups).mockRejectedValue(new Error("本地目录暂时不可用"));
  await waitFor(() => expect(errorToast).toHaveBeenCalled());
  await screen.findByRole("combobox", { name: "已有绑定分组 本地分组" });
  expect(errorToast.mock.calls.flat()).not.toEqual(
    expect.arrayContaining([expect.stringContaining("当前没有兼容")]),
  );
  expect(screen.getByRole("button", { name: "刷新本地分组" })).toBeEnabled();
});
