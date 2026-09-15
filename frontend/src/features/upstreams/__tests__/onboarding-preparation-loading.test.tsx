import type { QueryClient } from "@tanstack/react-query";
import { act, cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";

import { api } from "@/api";

import { boundCandidate, renderOnboarding } from "./onboarding-fixture";

let client: QueryClient | undefined;
afterEach(() => {
  cleanup();
  client?.clear();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

it("候选后台刷新失败时保留已选择的本地分组和可用预览", async () => {
  vi.stubGlobal("PointerEvent", MouseEvent);
  const candidate = {
    ...boundCandidate("active"),
    group_name: "新账号分组",
    bindable: true,
    can_create_key: true,
    can_bind_existing_key: false,
    bound: false,
    key_present: false,
    upstream_key_id: null,
    upstream_key_name: null,
    bound_accounts: [],
  };
  client = renderOnboarding(candidate, false);
  fireEvent.click(await screen.findByRole("combobox", { name: "新账号分组 本地分组" }));
  fireEvent.click(await screen.findByRole("option", { name: /备用分组/ }));
  fireEvent.keyDown(screen.getByRole("combobox", { name: "新账号分组 本地分组" }), {
    key: "Escape",
  });
  expect(screen.getByRole("button", { name: "预览 1 项变更" })).toBeEnabled();

  let failRefresh!: (error: Error) => void;
  vi.mocked(api.prepareOnboarding).mockImplementationOnce(
    () =>
      new Promise((_, reject) => {
        failRefresh = reject;
      }),
  );
  let refresh: Promise<void> | undefined;
  act(() => {
    refresh = client!.refetchQueries({ queryKey: ["onboarding-preparation"] });
  });
  await waitFor(() => expect(screen.getByRole("button", { name: "同步余额" })).toBeDisabled());
  await act(async () => {
    failRefresh(new Error("上游暂时不可用"));
    await refresh;
  });

  await waitFor(() => expect(screen.getByRole("button", { name: "重新读取" })).toBeEnabled());
  expect(screen.getByRole("button", { name: "预览 1 项变更" })).toBeEnabled();
});

it("已缓存上游详情时进入添加账号，准备请求完成后显示分组并结束加载", async () => {
  client = renderOnboarding(boundCandidate("active"), true, undefined, {
    cacheEntryConfiguration: true,
    strictMode: true,
  });

  expect(await screen.findByRole("button", { name: "预览更新绑定" })).toBeEnabled();
  expect(screen.queryByRole("button", { name: "正在获取" })).not.toBeInTheDocument();
});
