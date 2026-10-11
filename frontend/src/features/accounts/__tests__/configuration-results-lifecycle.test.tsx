import { QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { toast } from "sonner";
import { afterAll, afterEach, beforeEach, expect, it, vi } from "vitest";

// JSDOM 不支持菜单焦点计算使用的浏览器顶层选择器。
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

import { AccountsPage } from "@/App";
import { api, type AccountStatus, type Task } from "@/api";
import { createConsoleQueryClient } from "@/lib/query-client";
import { account, policy, task } from "./fixtures";

let client: ReturnType<typeof createConsoleQueryClient>;
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
  vi.spyOn(api, "policy").mockResolvedValue(policy);
  vi.spyOn(api, "dictionaries").mockResolvedValue({ items: [] });
  vi.spyOn(api, "modelCheckAccountStatuses").mockResolvedValue([]);
  vi.spyOn(api, "checkAccountConfiguration").mockResolvedValue(task("check-1", "queued"));
  vi.spyOn(api, "task").mockImplementation(async (id) => task(id));
  client = createConsoleQueryClient();
  client.setDefaultOptions({ queries: { retry: false, refetchOnWindowFocus: false } });
});
afterEach(() => {
  cleanup();
  client.clear();
  toast.dismiss();
  vi.restoreAllMocks();
});

async function openCheck(): Promise<void> {
  fireEvent.click(screen.getByRole("button", { name: "账号维护" }));
  fireEvent.click(await screen.findByRole("menuitem", { name: "配置校验与修复" }));
}

async function renderPage(): Promise<void> {
  render(
    <QueryClientProvider client={client}>
      <AccountsPage />
    </QueryClientProvider>,
  );
  await screen.findByText(account.name);
}

it("校验结束只刷新一次账号，读取完成前可关闭且不能修复旧结果", async () => {
  let finish!: (rows: AccountStatus[]) => void;
  const pending = new Promise<AccountStatus[]>((resolve) => {
    finish = resolve;
  });
  const accounts = vi
    .spyOn(api, "accounts")
    .mockResolvedValueOnce([
      {
        ...account,
        base_url_check: "official_mismatch",
        base_url_source: "platform_default",
        upstream_base_url: "https://api.example.test",
      },
    ])
    .mockReturnValue(pending);
  await renderPage();
  await openCheck();
  expect(await screen.findByRole("status", { name: "正在加载校验结果" })).toBeVisible();
  expect(screen.getByRole("button", { name: "关闭" })).toBeEnabled();
  expect(screen.queryByRole("button", { name: "修复并恢复" })).not.toBeInTheDocument();
  await act(async () => finish([{ ...account, name: "本轮校验结果" }]));
  expect(await within(screen.getByRole("dialog")).findByText("本轮校验结果")).toBeVisible();
  expect(accounts).toHaveBeenCalledTimes(2);
});

it("校验结果读取失败只提示一次，重新读取恢复最新结果且不重跑校验", async () => {
  const failure = vi.spyOn(toast, "error");
  vi.spyOn(api, "accounts")
    .mockResolvedValueOnce([account])
    .mockRejectedValue(new Error("账号列表暂不可用"));
  await renderPage();
  await openCheck();
  const retry = await screen.findByRole("button", { name: "重新读取" });
  expect(screen.queryByRole("status", { name: "正在加载校验结果" })).not.toBeInTheDocument();
  expect(screen.getByRole("button", { name: "关闭" })).toBeEnabled();
  expect(failure).toHaveBeenCalledTimes(1);
  expect(screen.queryByText("账号列表暂不可用")).not.toBeInTheDocument();
  let finishRetry!: (rows: AccountStatus[]) => void;
  vi.mocked(api.accounts).mockReturnValue(
    new Promise<AccountStatus[]>((resolve) => {
      finishRetry = resolve;
    }),
  );
  fireEvent.click(retry);
  expect(await screen.findByRole("status", { name: "正在加载校验结果" })).toBeVisible();
  expect(screen.queryByRole("button", { name: "重新读取" })).not.toBeInTheDocument();
  expect(screen.getByRole("button", { name: "关闭" })).toBeEnabled();
  await act(async () => finishRetry([{ ...account, name: "重新读取的校验结果" }]));
  expect(await within(screen.getByRole("dialog")).findByText("重新读取的校验结果")).toBeVisible();
  expect(api.checkAccountConfiguration).toHaveBeenCalledTimes(1);
});

it("关闭读取中的结果并启动新校验后，旧读取迟到不能展示为新任务结果", async () => {
  let finishOldRead!: (rows: AccountStatus[]) => void;
  const oldRead = new Promise<AccountStatus[]>((resolve) => {
    finishOldRead = resolve;
  });
  vi.spyOn(api, "accounts").mockResolvedValueOnce([account]).mockReturnValue(oldRead);
  await renderPage();
  await openCheck();
  await screen.findByRole("status", { name: "正在加载校验结果" });
  fireEvent.click(screen.getByRole("button", { name: "关闭" }));
  await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  let finishNewCheck!: (value: Task) => void;
  vi.mocked(api.checkAccountConfiguration).mockReturnValue(
    new Promise<Task>((resolve) => {
      finishNewCheck = resolve;
    }),
  );
  await openCheck();
  await screen.findByRole("status", { name: "正在启动配置校验与修复" });
  await act(async () => finishOldRead([{ ...account, name: "上一轮校验结果" }]));
  expect(within(screen.getByRole("dialog")).queryByText("上一轮校验结果")).not.toBeInTheDocument();
  expect(screen.queryByRole("button", { name: "重新运行" })).not.toBeInTheDocument();
  vi.mocked(api.accounts).mockResolvedValue([{ ...account, name: "新任务校验结果" }]);
  await act(async () => finishNewCheck(task("check-2", "queued")));
  expect(await within(screen.getByRole("dialog")).findByText("新任务校验结果")).toBeVisible();
});

it("部分完成的任务也显示结果读取状态，账号已移除时展示空结果而不保留旧账号", async () => {
  let finish!: (rows: AccountStatus[]) => void;
  vi.spyOn(api, "accounts")
    .mockResolvedValueOnce([account])
    .mockReturnValue(
      new Promise<AccountStatus[]>((resolve) => {
        finish = resolve;
      }),
    );
  vi.mocked(api.task).mockImplementation(async (id) => ({
    ...task(id, "partial"),
    result: { base_url: {} },
  }));
  await renderPage();
  await openCheck();
  expect(await screen.findByRole("status", { name: "正在加载校验结果" })).toBeVisible();
  await act(async () => finish([]));
  expect(await screen.findByRole("button", { name: "重新运行" })).toBeEnabled();
  expect(within(screen.getByRole("dialog")).queryByText(account.name)).not.toBeInTheDocument();
  expect(screen.queryByRole("status", { name: "正在加载校验结果" })).not.toBeInTheDocument();
});
