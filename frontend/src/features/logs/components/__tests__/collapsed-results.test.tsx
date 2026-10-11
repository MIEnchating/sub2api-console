import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it } from "vitest";

import { LogStructuredValue } from "../log-details-dialog";

function targets(prefix = "目标账号"): Record<string, { account_name: string }> {
  return Object.fromEntries(
    Array.from({ length: 12 }, (_, index) => [
      String(index + 1),
      { account_name: `${prefix} ${index + 1}` },
    ]),
  );
}

it("大调度集合首次展开前不构建明细，展开后可以完整翻页", async () => {
  const user = userEvent.setup();
  render(<LogStructuredValue value={{ account_targets: targets() }} />);
  expect(screen.getByText("12 项")).toBeVisible();
  expect(screen.queryByText("目标账号 1")).not.toBeInTheDocument();
  await user.click(screen.getByText("调度目标"));
  expect(await screen.findByText("目标账号 1")).toBeVisible();
  await user.click(screen.getByRole("button", { name: "转到下一页" }));
  expect(screen.getByText("目标账号 12")).toBeVisible();
  expect(screen.queryByText("目标账号 1")).not.toBeInTheDocument();
});

it("已展开集合收起再打开后保留分页并使用刷新后的记录", async () => {
  const user = userEvent.setup();
  const view = render(<LogStructuredValue value={{ account_targets: targets() }} />);
  const summary = screen.getByText("调度目标");
  await user.click(summary);
  await screen.findByText("目标账号 1");
  await user.click(screen.getByRole("button", { name: "转到下一页" }));
  await user.click(summary);
  await waitFor(() => expect(screen.getByText("目标账号 12")).not.toBeVisible());
  view.rerender(<LogStructuredValue value={{ account_targets: targets("更新账号") }} />);
  await user.click(summary);
  expect(await screen.findByText("更新账号 12")).toBeVisible();
  expect(screen.queryByText("更新账号 1")).not.toBeInTheDocument();
});

it("折叠期间更新集合时首次展开显示最新内容", async () => {
  const view = render(<LogStructuredValue value={{ account_targets: targets() }} />);
  view.rerender(<LogStructuredValue value={{ account_targets: targets("最新账号") }} />);
  expect(screen.queryByText("最新账号 1")).not.toBeInTheDocument();
  await userEvent.setup().click(screen.getByText("调度目标"));
  expect(await screen.findByText("最新账号 1")).toBeVisible();
  expect(screen.queryByText("目标账号 1")).not.toBeInTheDocument();
});

it("十条以内调度集合和执行明细仍直接展示", () => {
  render(
    <LogStructuredValue
      value={{
        account_targets: { "1": { account_name: "单项账号" } },
        items: Object.values(targets("执行账号")),
      }}
    />,
  );
  expect(screen.getByText("单项账号")).toBeVisible();
  expect(screen.getByText("执行账号 1")).toBeVisible();
  expect(screen.getByText("执行账号 10")).toBeVisible();
  expect(screen.queryByText("执行账号 11")).not.toBeInTheDocument();
});
