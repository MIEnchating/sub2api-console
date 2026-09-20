import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it, vi } from "vitest";
import { AccountDetailDialog } from "../account-detail-dialog";

it("账号名称被截断时，键盘聚焦可读取完整名称并用 Escape 关闭提示", async () => {
  const user = userEvent.setup();
  const accountName = "upstream-account-".repeat(20);
  const onOpenChange = vi.fn();
  render(
    <AccountDetailDialog open onOpenChange={onOpenChange} accountName={accountName} accountId="42">
      <p>账号配置</p>
    </AccountDetailDialog>,
  );

  const trigger = screen.getByText(accountName);
  expect(trigger).toHaveAttribute("tabindex", "0");
  expect(trigger).not.toHaveAttribute("title");
  await waitFor(() => expect(trigger).toHaveFocus());
  await user.tab();
  expect(screen.getByRole("button", { name: "关闭" })).toHaveFocus();
  await user.tab({ shift: true });
  expect(trigger).toHaveFocus();
  expect(await screen.findByRole("tooltip")).toHaveTextContent(accountName);
  await user.keyboard("{Escape}");
  expect(screen.queryByRole("tooltip")).not.toBeInTheDocument();
  expect(onOpenChange).not.toHaveBeenCalled();
  expect(screen.getByRole("dialog", { name: "账号设置" })).toBeVisible();
});
