import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it } from "vitest";
import { account } from "../../__tests__/fixtures";
import { AccountIdentityCell } from "../account-pool-cells";

afterEach(cleanup);

it("账号出现异常首字时显示标记且键盘可查看最近发现时间和处理规则", async () => {
  const user = userEvent.setup();
  const row = { ...account, probe_placeholder_at: "2026-10-11T12:00:00Z" };
  render(<AccountIdentityCell account={row} />);
  const marker = screen.getByText("探活异常首字");
  expect(marker).toBeVisible();
  marker.focus();
  expect(marker).toHaveFocus();
  const tooltip = await screen.findByRole("tooltip");
  expect(tooltip).toHaveTextContent("占位字符已忽略");
  expect(tooltip).toHaveTextContent("有效正文");
  expect(tooltip).toHaveTextContent("最近发现");
  await user.keyboard("{Escape}");
  expect(screen.queryByRole("tooltip")).not.toBeInTheDocument();
});

it("未发现异常首字的账号不显示异常标记", () => {
  render(<AccountIdentityCell account={account} />);
  expect(screen.queryByText("探活异常首字")).not.toBeInTheDocument();
});
