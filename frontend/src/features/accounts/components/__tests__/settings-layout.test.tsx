import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it } from "vitest";

import type { AccountDetail } from "@/api";
import { account } from "../../__tests__/fixtures";
import { AccountDetailDialog } from "../account-detail-dialog";
import { AccountSettingsPanel } from "../account-settings-panel";

const detail: AccountDetail = {
  ...account,
  base_url: "https://api.example.test/v1",
  upstream_base_url: "https://api.example.test",
  metadata: {},
  group_rates: {},
  group_ids: {},
  bindings: [],
  test_models: [],
};
let client: QueryClient;

afterEach(() => client.clear());

function renderSettings(): void {
  client = new QueryClient();
  render(
    <QueryClientProvider client={client}>
      <AccountDetailDialog
        open
        onOpenChange={() => undefined}
        accountId={detail.id}
        accountName={detail.name}
      >
        <AccountSettingsPanel
          accountId={detail.id}
          query={{ data: detail, isLoading: false, isError: false, error: null }}
          onCancel={() => undefined}
          onSaved={() => undefined}
        />
      </AccountDetailDialog>
    </QueryClientProvider>,
  );
}

it("账号设置无校验错误时不保留空白提示行，调度字段保持响应式两列布局", () => {
  renderSettings();
  const dialog = screen.getByRole("dialog", { name: "账号设置" });
  expect(dialog.querySelector('[data-slot="field-error"]')).not.toBeInTheDocument();
  expect(screen.getByTestId("account-routing-grid")).toHaveClass("gap-y-3", "sm:grid-cols-2");
});

it("内容超过弹窗高度时由内容区滚动，操作区保持在滚动区域外", () => {
  renderSettings();
  const dialog = screen.getByRole("dialog", { name: "账号设置" });
  const body = dialog.querySelector('[data-slot="dialog-body"]');
  expect(body).toHaveClass("min-h-0", "overflow-y-auto");
  expect(body).not.toHaveClass("overflow-visible");
  expect(body).not.toContainElement(screen.getByRole("button", { name: "保存" }));
  expect(body).not.toContainElement(screen.getByRole("button", { name: "取消" }));
});

it("账号管控使用紧凑的48px开关行，分区之间保持16px留白", () => {
  renderSettings();
  const control = screen.getByRole("region", { name: "账号管控" });
  expect(control).toHaveClass("gap-3", "pt-4");
  expect(control.closest("form")).toHaveClass("gap-4");
  expect(screen.getByRole("region", { name: "探测模型" })).toHaveClass("gap-3", "pt-4");
  for (const label of ["暂停调度", "排除该账号", "无视成本墙"]) {
    const row = screen
      .getByRole("switch", { name: label })
      .closest('[data-slot="settings-switch-row"]');
    expect(row).toHaveClass("min-h-12", "py-2");
  }
});

it("探测模型过长提交时显示字段错误，修正后移除提示占位", async () => {
  const user = userEvent.setup();
  renderSettings();
  const model = screen.getByRole("textbox", { name: "探测模型" });
  await user.click(model);
  await user.paste("a".repeat(257));
  await user.click(screen.getByRole("button", { name: "保存" }));
  expect(await screen.findByRole("alert")).toHaveTextContent("探测模型不能超过 256 个字符");
  expect(model).toHaveAttribute("aria-invalid", "true");
  await user.clear(model);
  expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  expect(
    screen.getByRole("dialog").querySelector('[data-slot="field-error"]'),
  ).not.toBeInTheDocument();
});
