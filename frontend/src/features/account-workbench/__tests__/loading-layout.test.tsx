import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";

import { AccountWorkbenchPage } from "../components/account-workbench-page";

let client: QueryClient;
afterEach(() => {
  cleanup();
  client?.clear();
  vi.unstubAllGlobals();
});

function renderPendingWorkbench(): void {
  vi.stubGlobal(
    "fetch",
    vi.fn(() => new Promise<Response>(() => {})),
  );
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <AccountWorkbenchPage />
    </QueryClientProvider>,
  );
}

it("导入配置首次读取时保留单张导入表单和大文本输入位置", () => {
  renderPendingWorkbench();
  const loading = screen.getByRole("status", { name: "正在读取账号导入配置" });
  expect(loading.querySelector('[data-slot="skeleton-textarea"]')).toHaveClass("h-64");
  expect(loading.querySelectorAll('[data-slot="workbench-form-skeleton"]')).toHaveLength(1);
  expect(screen.queryByRole("button", { name: "解析并预览" })).not.toBeInTheDocument();
});

it("切换维护设置等待数据时保留整行单表单及并排参数", async () => {
  renderPendingWorkbench();
  await userEvent.setup().click(screen.getByRole("tab", { name: "自动维护" }));
  const loading = screen.getByRole("status", { name: "正在读取账号维护设置" });
  expect(loading.querySelectorAll('[data-slot="workbench-form-skeleton"]')).toHaveLength(1);
  expect(loading.querySelector('[data-slot="maintenance-parameters"]')).toHaveClass(
    "sm:grid-cols-2",
  );
  expect(within(loading).queryByRole("button")).not.toBeInTheDocument();
});

it("配置模板首次读取时按桌面两列模板卡占位", async () => {
  renderPendingWorkbench();
  await userEvent.setup().click(screen.getByRole("tab", { name: "配置模板" }));
  const loading = screen.getByRole("status", { name: "正在读取账号配置模板" });
  expect(loading.querySelector('[data-slot="workbench-template-grid"]')).toHaveClass(
    "lg:grid-cols-2",
  );
});

it("维护分组首次读取时标题位于边框外且选项在桌面并排", async () => {
  renderPendingWorkbench();
  await userEvent.setup().click(screen.getByRole("tab", { name: "自动维护" }));
  const loading = screen.getByRole("status", { name: "正在读取账号维护设置" });
  const options = loading.querySelector('[data-slot="maintenance-group-options"]');
  const label = loading.querySelector<HTMLElement>('[data-slot="maintenance-group-label"]');

  expect(options).toHaveClass("grid", "sm:grid-cols-2", "border", "overflow-y-auto");
  expect(label).toBeInTheDocument();
  expect(options).not.toContainElement(label);
});

it("配置模板首次读取时为四行固定详情及左对齐操作预留位置", async () => {
  renderPendingWorkbench();
  await userEvent.setup().click(screen.getByRole("tab", { name: "配置模板" }));
  const loading = screen.getByRole("status", { name: "正在读取账号配置模板" });
  const cards = loading.querySelectorAll('[data-slot="workbench-template-card"]');

  expect(cards).toHaveLength(2);
  for (const card of cards) {
    expect(card.querySelectorAll('[data-slot="workbench-template-detail"]')).toHaveLength(4);
    const actions = card.querySelector('[data-slot="workbench-template-actions"]');
    expect(actions).toHaveClass("flex", "flex-wrap");
    expect(actions).not.toHaveClass("justify-end");
  }
});

it("安全设置首次读取时按账号与安全操作双选择器占位", async () => {
  renderPendingWorkbench();
  await userEvent.setup().click(screen.getByRole("tab", { name: "账号安全" }));
  const loading = screen.getByRole("status", { name: "正在读取安全设置账号" });
  expect(loading.querySelector('[data-slot="security-selectors"]')).toHaveClass("sm:grid-cols-2");
  expect(loading.querySelectorAll('[data-slot="skeleton-control"]')).toHaveLength(2);
});

it("账号导出首次读取时保留搜索和两列账号选择位置", async () => {
  renderPendingWorkbench();
  await userEvent.setup().click(screen.getByRole("tab", { name: "私有导出" }));
  const loading = screen.getByRole("status", { name: "正在读取导出账号" });
  expect(loading.querySelector('[data-slot="export-account-options"]')).toHaveClass(
    "sm:grid-cols-2",
  );
  expect(loading.querySelector('[data-slot="skeleton-control"]')).toHaveClass("h-8");
});

it("批量安全设置首次读取时保留两列账号范围和操作选择器", async () => {
  renderPendingWorkbench();
  const user = userEvent.setup();
  await user.click(screen.getByRole("tab", { name: "账号安全" }));
  await user.click(screen.getByRole("tab", { name: "批量账号" }));
  const loading = screen.getByRole("status", { name: "正在读取批量安全设置账号" });
  expect(loading.querySelector('[data-slot="security-account-options"]')).toHaveClass(
    "sm:grid-cols-2",
  );
  expect(loading.querySelectorAll('[data-slot="skeleton-control"]')).toHaveLength(1);
});
