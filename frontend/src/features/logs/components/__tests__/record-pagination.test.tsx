import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it } from "vitest";

import { LogStructuredValue } from "../log-details-dialog";

it("账号映射翻页时保留键中的账号 ID，并忽略非记录项", async () => {
  const user = userEvent.setup();
  const records = Object.fromEntries(
    Array.from({ length: 12 }, (_, index) => [`account-${index + 1}`, { status: "succeeded" }]),
  );
  render(
    <LogStructuredValue
      fieldKey="account_targets"
      value={{ invalid: null, scalar: 42, list: [], ...records }}
    />,
  );
  expect(screen.getByText("记录 account-1")).toBeVisible();
  expect(screen.getByText("记录 account-10")).toBeVisible();
  await user.click(screen.getByRole("button", { name: "转到下一页" }));
  expect(screen.getByText("记录 account-11")).toBeVisible();
  expect(screen.getByText("记录 account-12")).toBeVisible();
  expect(screen.getByRole("button", { name: "转到下一页" })).toBeDisabled();
  expect(records["account-11"]).toEqual({ status: "succeeded" });
});

it("映射包含明确账号 ID 时使用原 ID，刷新缩减后显示有效页", async () => {
  const user = userEvent.setup();
  const records = Object.fromEntries(
    Array.from({ length: 12 }, (_, index) => [
      `map-${index + 1}`,
      { account_id: `stable-${index + 1}` },
    ]),
  );
  const view = render(<LogStructuredValue fieldKey="account_targets" value={records} />);
  expect(screen.getByText("记录 stable-1")).toBeVisible();
  expect(screen.queryByText("记录 map-1")).not.toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "转到下一页" }));
  expect(screen.getByText("记录 stable-12")).toBeVisible();
  view.rerender(
    <LogStructuredValue fieldKey="account_targets" value={{ only: { account_id: "updated" } }} />,
  );
  expect(screen.getByText("记录 updated")).toBeVisible();
  expect(screen.queryByText("记录 stable-12")).not.toBeInTheDocument();
  expect(screen.queryByRole("navigation", { name: "表格分页" })).not.toBeInTheDocument();
});

it("倍率字段只在后续页出现时首尾页仍保持倍率表格", async () => {
  const user = userEvent.setup();
  const items = [
    ...Array.from({ length: 10 }, (_, index) => ({ account_name: `同步账号 ${index + 1}` })),
    { account_name: "倍率账号", account_multiplier: "0.5" },
  ];
  render(<LogStructuredValue value={items} />);
  expect(screen.getByRole("table", { name: "账号倍率同步执行明细" })).toBeVisible();
  await user.click(screen.getByRole("button", { name: "转到下一页" }));
  expect(screen.getByRole("table", { name: "账号倍率同步执行明细" })).toBeVisible();
  expect(screen.getByText("倍率账号")).toBeVisible();
  expect(screen.getByText("0.5")).toBeVisible();
});

it("刷新为空映射时清除旧账号，后续新记录仍能显示", () => {
  const view = render(
    <LogStructuredValue fieldKey="account_targets" value={{ original: { status: "succeeded" } }} />,
  );
  expect(screen.getByText("记录 original")).toBeVisible();
  view.rerender(<LogStructuredValue fieldKey="account_targets" value={{}} />);
  expect(screen.queryByText("记录 original")).not.toBeInTheDocument();
  expect(screen.queryByRole("navigation", { name: "表格分页" })).not.toBeInTheDocument();
  view.rerender(
    <LogStructuredValue
      fieldKey="account_targets"
      value={{ replacement: { status: "succeeded" } }}
    />,
  );
  expect(screen.getByText("记录 replacement")).toBeVisible();
});

it("数组刷新后使用新倍率字段选择表格并展示最新值", () => {
  const view = render(<LogStructuredValue value={[{ account_name: "刷新账号" }]} />);
  expect(screen.queryByRole("table", { name: "账号倍率同步执行明细" })).not.toBeInTheDocument();
  view.rerender(
    <LogStructuredValue value={[{ account_name: "刷新账号", account_multiplier: "0.75" }]} />,
  );
  expect(screen.getByRole("table", { name: "账号倍率同步执行明细" })).toBeVisible();
  expect(screen.getByText("0.75")).toBeVisible();
});

it("无 ID 的记录翻页后不继承上一页的嵌套集合展开状态", async () => {
  const user = userEvent.setup();
  const records = Array.from({ length: 11 }, (_, index) => ({
    account_name: `外层账号 ${index + 1}`,
    routing: {
      account_targets: Object.fromEntries(
        Array.from({ length: 12 }, (_, target) => [
          String(target + 1),
          { account_name: `子账号 ${index + 1}-${target + 1}` },
        ]),
      ),
    },
  }));
  render(<LogStructuredValue value={records} />);
  await user.click(screen.getAllByText("调度目标")[0]!);
  expect(await screen.findByText("子账号 1-1")).toBeVisible();
  await user.click(screen.getAllByRole("button", { name: "转到下一页" }).at(-1)!);
  expect(screen.getByText("外层账号 11")).toBeVisible();
  expect(screen.queryByText("子账号 11-1")).not.toBeInTheDocument();
  await user.click(screen.getByText("调度目标"));
  expect(await screen.findByText("子账号 11-1")).toBeVisible();
});
