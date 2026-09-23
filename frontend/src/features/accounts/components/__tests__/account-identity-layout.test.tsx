import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it } from "vitest";
import { account } from "../../__tests__/fixtures";
import { AccountIdentityCell } from "../account-pool-cells";

afterEach(() => {
  cleanup();
});

function renderIdentity(long = false) {
  const row = {
    ...account,
    name: long ? "需要保留完整名称的账号".repeat(10) : "测试账号",
    upstream_host: long ? `${"long-host-".repeat(20)}example.test` : null,
    groups: long ? ["主分组".repeat(20), "备用分组"] : [],
  };
  render(<AccountIdentityCell account={row} />);
  return row;
}

it("账号管理不显示实时流量状态", () => {
  renderIdentity();
  expect(screen.queryByLabelText(/账号 41：真实请求/)).not.toBeInTheDocument();
});

it("账号名、Host 和多分组过长时保持单行省略并可键盘查看完整内容", async () => {
  const user = userEvent.setup();
  const row = renderIdentity(true);
  for (const text of [row.name, row.upstream_host!, `分组：${row.groups.join("、")}`]) {
    const field = screen.getByText(text);
    expect(field).toHaveClass("truncate");
    expect(field).toHaveAttribute("tabindex", "0");
    field.focus();
    expect(await screen.findByRole("tooltip")).toHaveTextContent(text);
    await user.keyboard("{Escape}");
  }
  const group = screen.getByText(`分组：${row.groups.join("、")}`);
  expect(group).toHaveClass("w-full");
  expect(group).not.toHaveClass("border-l", "pl-2");
});

it("Host 缺失时在账号类型同行显示占位，分组独占一行", () => {
  renderIdentity();
  const context = screen.getByText("Host 未记录").closest('[data-slot="account-identity-meta"]');
  expect(context).toHaveClass("flex", "flex-nowrap");
  expect(context).toContainElement(screen.getByText(/#41/));
  expect(context).not.toContainElement(screen.getByText("分组：未分组"));
});
