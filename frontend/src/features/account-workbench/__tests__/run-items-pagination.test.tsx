import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { RunItems } from "../components/run-items";
import { runItemsFixture } from "./list-pagination-fixture";

afterEach(cleanup);

it("500 项处理结果每页挂载 20 行，后续页手动启用传递正确项目", async () => {
  const run = runItemsFixture(500);
  const enable = vi.fn();
  render(<RunItems run={run} pending={false} onEnable={enable} />);
  expect(
    within(screen.getByRole("table", { name: "账号处理结果" })).getAllByRole("row"),
  ).toHaveLength(21);
  const user = userEvent.setup();
  screen.getByRole("button", { name: "转到下一页" }).focus();
  await user.keyboard("{Enter}");
  await user.click(
    within(screen.getByRole("row", { name: /account-21@example.test/ })).getByRole("button", {
      name: "启用",
    }),
  );
  expect(enable).toHaveBeenCalledWith(run.items[20]);
});

it("轮询版本更新保留页码，批次切换重置首页，数量缩减收敛到有效页", async () => {
  const run = runItemsFixture(41);
  const enable = vi.fn();
  const view = render(<RunItems run={run} pending={false} onEnable={enable} />);
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "转到下一页" }));
  view.rerender(<RunItems run={{ ...run, revision: 2 }} pending={false} onEnable={enable} />);
  expect(screen.getByText("account-21@example.test")).toBeVisible();
  view.rerender(<RunItems run={{ ...run, id: "batch-two" }} pending={false} onEnable={enable} />);
  expect(screen.getByText("account-1@example.test")).toBeVisible();
  await user.click(screen.getByRole("button", { name: "转到最后一页" }));
  view.rerender(<RunItems run={runItemsFixture(1)} pending={false} onEnable={enable} />);
  expect(screen.getByText("account-1@example.test")).toBeVisible();
  expect(screen.getByRole("button", { name: "转到下一页" })).toBeDisabled();
});

it("写入等待期间分页和检测详情仍可读取，启用操作保持禁用", async () => {
  render(<RunItems run={runItemsFixture(21)} pending onEnable={vi.fn()} />);
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "转到下一页" }));
  expect(screen.getByRole("button", { name: "启用" })).toBeDisabled();
  await user.click(screen.getByRole("button", { name: "检测详情" }));
  expect(
    within(screen.getByRole("dialog")).getByRole("heading", {
      name: "检测详情 · account-21@example.test",
    }),
  ).toBeVisible();
});

it("空批次分页禁用，切换每页行数后可以读取全部小批次", async () => {
  const enable = vi.fn();
  const view = render(<RunItems run={runItemsFixture(0)} pending={false} onEnable={enable} />);
  expect(screen.getByRole("button", { name: "转到下一页" })).toBeDisabled();
  view.rerender(<RunItems run={runItemsFixture(41)} pending={false} onEnable={enable} />);
  const user = userEvent.setup();
  await user.click(screen.getByRole("combobox", { name: "每页行数" }));
  await user.click(await screen.findByRole("option", { name: "50" }));
  expect(within(screen.getByRole("table")).getAllByRole("row")).toHaveLength(42);
});
