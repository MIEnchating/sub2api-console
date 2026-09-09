import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterAll, beforeEach, afterEach, expect, it, vi } from "vitest";

// JSDOM 26 的样式匹配不支持浏览器顶层选择器，会在菜单焦点计算时递归。
const restoreSelectorMatching = vi.hoisted(() => {
  const matches = Element.prototype.matches;
  Element.prototype.matches = function (selector: string): boolean {
    if ([":fullscreen", ":popover-open", ":modal"].includes(selector)) return false;
    return matches.call(this, selector);
  };
  return () => {
    Element.prototype.matches = matches;
  };
});
afterAll(restoreSelectorMatching);
beforeEach(() => {
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

import { AccountsPage } from "@/App";
import { api, type Task } from "@/api";

import { account, policy, task } from "./fixtures";

let client: QueryClient | undefined;
afterEach(() => {
  client?.clear();
  vi.restoreAllMocks();
});

it("复验绑定任务已创建而首次状态尚未返回时持续展示加载状态", async () => {
  vi.spyOn(api, "accounts").mockResolvedValue([account]);
  vi.spyOn(api, "policy").mockResolvedValue(policy);
  vi.spyOn(api, "revalidateAccounts").mockResolvedValue(task("revalidation", "queued"));
  const readTask = vi.spyOn(api, "task").mockReturnValue(new Promise<Task>(() => {}));
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <AccountsPage />
    </QueryClientProvider>,
  );
  await screen.findByText("待校验账号");
  fireEvent.click(screen.getByRole("button", { name: "账号维护" }));
  fireEvent.click(await screen.findByRole("menuitem", { name: "复验绑定" }));
  await waitFor(() => expect(readTask).toHaveBeenCalledWith("revalidation"));
  expect(within(screen.getByRole("dialog")).getByRole("status")).toBeVisible();
  client.clear();
});
