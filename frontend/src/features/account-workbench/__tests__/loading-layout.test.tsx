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

it("导入配置首次读取时按容器宽度为账号内容及配置侧栏占位", () => {
  renderPendingWorkbench();
  const loading = screen.getByRole("status", { name: "正在读取账号导入配置" });
  expect(loading.querySelector('[data-slot="skeleton-textarea"]')).toHaveClass("h-64");
  expect(loading.querySelectorAll('[data-slot="workbench-form-skeleton"]')).toHaveLength(1);
  expect(loading.querySelector('[data-slot="workbench-import-columns"]')).toHaveClass(
    "grid",
    "@3xl/import:grid-cols-[minmax(0,1fr)_20rem]",
  );
  expect(
    loading.querySelector('[data-slot="workbench-import-options-skeleton"]'),
  ).toBeInTheDocument();
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

it("配置模板首次读取时按单列列表占位", async () => {
  renderPendingWorkbench();
  await userEvent.setup().click(screen.getByRole("tab", { name: "配置模板" }));
  const loading = screen.getByRole("status", { name: "正在读取账号配置模板" });
  expect(loading.querySelector('[data-slot="workbench-template-grid"]')).toHaveClass("grid-cols-1");
});

it("维护设置首次读取时不为收起的高级分组占据空间", async () => {
  renderPendingWorkbench();
  await userEvent.setup().click(screen.getByRole("tab", { name: "自动维护" }));
  const loading = screen.getByRole("status", { name: "正在读取账号维护设置" });
  const options = loading.querySelector('[data-slot="maintenance-group-options"]');
  const label = loading.querySelector<HTMLElement>('[data-slot="maintenance-group-label"]');

  expect(options).not.toBeInTheDocument();
  expect(label).not.toBeInTheDocument();
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
