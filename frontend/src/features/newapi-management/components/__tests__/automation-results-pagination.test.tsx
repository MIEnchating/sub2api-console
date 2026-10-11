import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it } from "vitest";

import type { Task } from "@/api";
import {
  AccountModelAutomationGroupResults,
  AccountModelAutomationResults,
} from "../account-model-automation-results";

function task(itemCount: number): Task {
  return {
    id: "automation-result",
    skill: "sub2api-account-model-automation",
    operation: "account-model-automation",
    status: "succeeded",
    progress: 100,
    message: "执行完成",
    result: {
      dry_run: false,
      result_version: 2,
      models: ["model-a"],
      pricing_plan: [{ model: "model-a", input_ratio: "1", completion_ratio: "2" }],
      items: [
        ...Array.from({ length: itemCount }, (_, index) => ({
          account_id: `account-${index + 1}`,
          status: "succeeded",
          models: ["model-a"],
        })),
        ...Array.from({ length: itemCount }, (_, index) => ({
          channel_id: `channel-${index + 1}`,
          channel_name: `渠道 ${index + 1}`,
          status: "succeeded",
          added: ["model-a"],
        })),
      ],
    },
    created_at: "2026-10-10T00:00:00Z",
    updated_at: "2026-10-10T00:00:00Z",
  };
}

it("大结果集默认只挂载20条账号和渠道记录，翻页后可读取下一页", async () => {
  const user = userEvent.setup();
  render(<AccountModelAutomationResults task={task(1000)} />);

  const accounts = screen.getByRole("region", { name: "账号模型目录" });
  const channels = screen.getByRole("region", { name: "渠道模型上架" });
  expect(within(accounts).getAllByRole("listitem")).toHaveLength(20);
  expect(within(channels).getAllByRole("listitem")).toHaveLength(20);
  expect(within(accounts).queryByText("账号 account-21 · 目录已刷新")).not.toBeInTheDocument();
  expect(within(channels).queryByText(/渠道 21/)).not.toBeInTheDocument();

  await user.click(within(accounts).getByRole("button", { name: "转到下一页" }));
  await user.click(within(channels).getByRole("button", { name: "转到下一页" }));
  expect(within(accounts).getByText("账号 account-21 · 目录已刷新")).toBeVisible();
  expect(within(channels).getByText(/渠道 21/)).toBeVisible();
});

it("任务组大结果默认只挂载20条成员，翻页后可读取下一页", async () => {
  const user = userEvent.setup();
  const groupTask = {
    ...task(0),
    operation: "account-model-automation-group",
    result: {
      items: Array.from({ length: 1000 }, (_, index) => ({
        schedule_id: `schedule-${index + 1}`,
        schedule_name: `任务 ${index + 1}`,
        status: "succeeded",
      })),
    },
  } satisfies Task;
  render(<AccountModelAutomationGroupResults task={groupTask} />);

  const region = screen.getByRole("region", { name: "运营任务组结果" });
  expect(within(region).getAllByRole("listitem")).toHaveLength(20);
  expect(within(region).queryByText("任务 21")).not.toBeInTheDocument();
  await user.click(within(region).getByRole("button", { name: "转到下一页" }));
  expect(within(region).getByText("任务 21")).toBeVisible();
});

it("1000条定价方案只挂载20条，键盘翻页可读取末条精确倍率", async () => {
  const detail = task(0);
  detail.result.pricing_plan = Array.from({ length: 1000 }, (_, index) => ({
    model: `price-${index + 1}`,
    input_ratio: "0.123456789012345678",
    completion_ratio: "2",
  }));
  render(<AccountModelAutomationResults task={detail} />);
  const prices = screen.getByLabelText("模型定价方案");
  expect(within(prices).getAllByText(/输入倍率/)).toHaveLength(20);
  within(prices).getByRole("button", { name: "转到最后一页" }).focus();
  await userEvent.keyboard("{Enter}");
  expect(within(prices).getByText("price-1000")).toBeVisible();
  expect(within(prices).getAllByText("输入倍率 0.123456789012345678 · 输出倍率 2")).toHaveLength(
    20,
  );
  expect(within(prices).getByRole("button", { name: "转到下一页" })).toBeDisabled();
});

it("同任务刷新保留有效页，切换任务从首页展示", async () => {
  const view = render(<AccountModelAutomationResults task={task(41)} />);
  const accounts = screen.getByRole("region", { name: "账号模型目录" });
  await userEvent.click(within(accounts).getByRole("button", { name: "转到下一页" }));
  view.rerender(<AccountModelAutomationResults task={task(42)} />);
  expect(within(accounts).getByText("账号 account-21 · 目录已刷新")).toBeVisible();
  view.rerender(<AccountModelAutomationResults task={{ ...task(41), id: "different-task" }} />);
  expect(screen.getByText("账号 account-1 · 目录已刷新")).toBeVisible();
});

it("选择10行后结果缩到15条仍能访问第二页，清空后恢复回首页", async () => {
  const view = render(<AccountModelAutomationResults task={task(41)} />);
  const accounts = screen.getByRole("region", { name: "账号模型目录" });
  await userEvent.click(within(accounts).getByRole("combobox", { name: "每页行数" }));
  await userEvent.click(await screen.findByRole("option", { name: "10" }));
  view.rerender(<AccountModelAutomationResults task={task(15)} />);
  await userEvent.click(within(accounts).getByRole("button", { name: "转到下一页" }));
  expect(within(accounts).getAllByRole("listitem")).toHaveLength(5);
  expect(within(accounts).getByText("账号 account-15 · 目录已刷新")).toBeVisible();
  view.rerender(<AccountModelAutomationResults task={task(0)} />);
  expect(screen.queryByRole("region", { name: "账号模型目录" })).not.toBeInTheDocument();
  view.rerender(<AccountModelAutomationResults task={task(41)} />);
  expect(screen.getByText("账号 account-1 · 目录已刷新")).toBeVisible();
});
