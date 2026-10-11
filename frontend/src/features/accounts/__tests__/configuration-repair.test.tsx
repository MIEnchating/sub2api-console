import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { toast } from "sonner";
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
import { api } from "@/api";

import { account, policy, task } from "./fixtures";

let client: QueryClient | undefined;
afterEach(() => {
  client?.clear();
  vi.restoreAllMocks();
});

it("Base URL 修复完成后只提示一次该修复结果", async () => {
  const success = vi.spyOn(toast, "success");
  vi.spyOn(api, "accounts").mockResolvedValue([
    {
      ...account,
      base_url_check: "official_mismatch",
      base_url_source: "platform_default",
      upstream_base_url: "https://api.example.test",
      base_url: "https://api.openai.com",
    },
  ]);
  vi.spyOn(api, "policy").mockResolvedValue(policy);
  vi.spyOn(api, "checkAccountConfiguration").mockResolvedValue(
    task("configuration-check", "queued"),
  );
  vi.spyOn(api, "repairAccountBaseURLs").mockResolvedValue(task("base-url-repair", "queued"));
  vi.spyOn(api, "task").mockImplementation(async (id) => task(id));
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <AccountsPage />
    </QueryClientProvider>,
  );
  await screen.findByText("待校验账号");
  fireEvent.click(screen.getByRole("button", { name: "账号维护" }));
  fireEvent.click(await screen.findByRole("menuitem", { name: "配置校验与修复" }));
  fireEvent.click(await screen.findByRole("button", { name: "修复并恢复" }, { timeout: 5_000 }));
  await waitFor(() => expect(success).toHaveBeenCalledWith("Base URL 已修复"));
  expect(success).toHaveBeenCalledTimes(1);
  client.clear();
});

it("修复任务运行时关闭并从维护菜单重开只查看结果，不启动另一轮检查", async () => {
  vi.spyOn(api, "accounts").mockResolvedValue([
    {
      ...account,
      base_url_check: "official_mismatch",
      base_url_source: "platform_default",
      upstream_base_url: "https://api.example.test",
    },
  ]);
  vi.spyOn(api, "policy").mockResolvedValue(policy);
  const check = vi
    .spyOn(api, "checkAccountConfiguration")
    .mockResolvedValue(task("configuration-check", "queued"));
  vi.spyOn(api, "repairAccountBaseURLs").mockResolvedValue(task("base-url-repair", "queued"));
  vi.spyOn(api, "task").mockImplementation(async (id) =>
    task(id, id === "base-url-repair" ? "running" : "succeeded"),
  );
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <AccountsPage />
    </QueryClientProvider>,
  );
  await screen.findByText(account.name);
  fireEvent.click(screen.getByRole("button", { name: "账号维护" }));
  fireEvent.click(await screen.findByRole("menuitem", { name: "配置校验与修复" }));
  fireEvent.click(await screen.findByRole("button", { name: "修复并恢复" }));
  expect(await screen.findByRole("button", { name: "修复中" })).toBeDisabled();
  fireEvent.click(screen.getByRole("button", { name: "关闭" }));
  await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  fireEvent.click(screen.getByRole("button", { name: "账号维护" }));
  fireEvent.click(await screen.findByRole("menuitem", { name: "配置校验与修复" }));
  expect(await screen.findByRole("button", { name: "修复中" })).toBeDisabled();
  expect(check).toHaveBeenCalledTimes(1);
});

it("修复创建请求未返回时禁止重新运行，创建失败后恢复操作", async () => {
  vi.spyOn(api, "accounts").mockResolvedValue([
    {
      ...account,
      base_url_check: "official_mismatch",
      base_url_source: "platform_default",
      upstream_base_url: "https://api.example.test",
    },
  ]);
  vi.spyOn(api, "policy").mockResolvedValue(policy);
  vi.spyOn(api, "checkAccountConfiguration").mockResolvedValue(task("configuration-check"));
  let rejectRepair: (error: Error) => void = () => undefined;
  vi.spyOn(api, "repairAccountBaseURLs").mockReturnValue(
    new Promise<ReturnType<typeof task>>((_resolve, reject) => {
      rejectRepair = reject;
    }),
  );
  vi.spyOn(api, "task").mockImplementation(async (id) => task(id));
  const failure = vi.spyOn(toast, "error");
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <AccountsPage />
    </QueryClientProvider>,
  );
  await screen.findByText(account.name);
  fireEvent.click(screen.getByRole("button", { name: "账号维护" }));
  fireEvent.click(await screen.findByRole("menuitem", { name: "配置校验与修复" }));
  fireEvent.click(await screen.findByRole("button", { name: "修复并恢复" }));
  expect(await screen.findByRole("button", { name: "修复中" })).toBeDisabled();
  expect(screen.getByRole("button", { name: "重新运行" })).toBeDisabled();
  rejectRepair(new Error("修复服务暂不可用"));
  await waitFor(() => expect(screen.getByRole("button", { name: "重新运行" })).toBeEnabled());
  expect(screen.getByRole("button", { name: "修复并恢复" })).toBeEnabled();
  expect(failure).toHaveBeenCalledTimes(1);
});
