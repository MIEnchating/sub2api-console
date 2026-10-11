import { fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it, vi } from "vitest";

import type { AccountStatus } from "@/api";
import { account } from "../../__tests__/fixtures";
import { BaseURLCheckResults, type BaseURLCheckResultsProps } from "../base-url-check-results";

const repairable: AccountStatus = {
  ...account,
  name: "待修复账号",
  base_url_check: "official_mismatch",
  base_url_source: "platform_default",
  upstream_base_url: "https://relay.example.test/v1",
};

function props(accounts: AccountStatus[] = [repairable]): BaseURLCheckResultsProps {
  return {
    accounts,
    running: false,
    repairing: false,
    repairingAccountId: null,
    onRerun: vi.fn(),
    onRepair: vi.fn(),
  };
}

it.each(["running", "repairing"] as const)(
  "%s 期间禁止重新检查和修复，结束后恢复操作",
  async (phase) => {
    const input = props();
    const view = render(<BaseURLCheckResults {...input} {...{ [phase]: true }} />);
    const rerun = screen.getByRole("button", { name: "重新运行" });
    const repair = screen.getByRole("button", { name: "修复并恢复" });
    expect(rerun).toBeDisabled();
    expect(repair).toBeDisabled();
    fireEvent.click(rerun);
    fireEvent.click(repair);
    expect(input.onRerun).not.toHaveBeenCalled();
    expect(input.onRepair).not.toHaveBeenCalled();

    view.rerender(<BaseURLCheckResults {...input} />);
    expect(rerun).toBeEnabled();
    expect(repair).toBeEnabled();
    const user = userEvent.setup();
    repair.focus();
    await user.keyboard("{Enter}");
    expect(input.onRepair).toHaveBeenCalledWith("41", "base_url");
  },
);

it("同名账号修复按稳定 ID 和修复类型提交", async () => {
  const input = props([
    { ...repairable, id: "42", name: "同名账号", upstream_host_repairable: true },
    { ...repairable, name: "同名账号" },
  ]);
  render(<BaseURLCheckResults {...input} />);
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "修复归属" }));
  expect(input.onRepair).toHaveBeenLastCalledWith("42", "upstream_host");
  await user.click(screen.getByRole("button", { name: "修复并恢复" }));
  expect(input.onRepair).toHaveBeenLastCalledWith("41", "base_url");
});

it("第二页搜索时返回第一页并保留异常优先顺序，清空后恢复完整列表", () => {
  const accounts: AccountStatus[] = Array.from({ length: 25 }, (_, index) => ({
    ...account,
    id: String(index + 1),
    name: `账号-${String(index + 1).padStart(2, "0")}`,
    base_url_check: "matched",
    groups: ["Group-Target"],
  }));
  accounts[24] = { ...accounts[24], base_url_check: "official_mismatch" };
  const input = props(accounts);
  render(<BaseURLCheckResults {...input} />);
  fireEvent.click(screen.getByRole("button", { name: "转到下一页" }));
  expect(screen.getByLabelText("当前第 2 页，共 2 页")).toBeVisible();
  const search = screen.getByRole("textbox", { name: "搜索账号、ID、Host 或 Base URL" });
  fireEvent.change(search, { target: { value: "  GROUP-target  " } });
  expect(screen.getByLabelText("当前第 1 页，共 2 页")).toBeVisible();
  expect(within(screen.getAllByRole("row")[1]).getByText("账号-25")).toBeVisible();
  fireEvent.change(search, { target: { value: "账号-02" } });
  expect(screen.getByText("账号-02")).toBeVisible();
  expect(screen.queryByText("账号-25")).not.toBeInTheDocument();
  fireEvent.change(search, { target: { value: "没有该账号" } });
  expect(screen.getByText("没有匹配的账号")).toBeVisible();
  fireEvent.change(search, { target: { value: "" } });
  expect(screen.getByText("账号-25")).toBeVisible();
  expect(accounts[0].id).toBe("1");
});

it("目录刷新后当前搜索立即使用新字段，移除的账号不再展示", () => {
  const input = props([{ ...account, base_url: "https://old.example.test/v1" }]);
  const view = render(<BaseURLCheckResults {...input} />);
  const search = screen.getByRole("textbox", { name: "搜索账号、ID、Host 或 Base URL" });
  fireEvent.change(search, { target: { value: "new.example.test" } });
  expect(screen.getByText("没有匹配的账号")).toBeVisible();
  view.rerender(
    <BaseURLCheckResults
      {...input}
      accounts={[{ ...account, base_url: "https://new.example.test/v1" }]}
    />,
  );
  expect(screen.getByText(account.name)).toBeVisible();
  view.rerender(<BaseURLCheckResults {...input} accounts={[]} />);
  expect(screen.queryByText(account.name)).not.toBeInTheDocument();
  fireEvent.change(search, { target: { value: "" } });
  expect(screen.getByText("当前没有账号")).toBeVisible();
});
