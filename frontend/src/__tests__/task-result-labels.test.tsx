import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";

import { AccountSyncTaskStatus } from "../App";
import type { Task } from "../api";

afterEach(cleanup);

function renderResult(result: Task["result"]): void {
  const task: Task = {
    id: "account-sync-1",
    skill: "accounts",
    operation: "account-fields-sync",
    status: "succeeded",
    progress: 100,
    message: "账号字段同步完成",
    result,
    created_at: "2026-09-14T10:00:00Z",
    updated_at: "2026-09-14T10:00:01Z",
  };
  render(<AccountSyncTaskStatus task={task} accountId="1" onClose={() => {}} />);
}

it.each([
  ["__proto__", "Proto"],
  ["constructor", "constructor"],
  ["toString", "toString"],
  ["CONSTRUCTOR suffix", "CONSTRUCTOR suffix"],
])("任务结果值为 %s 时显示文本且不读取字典继承属性", (value, label) => {
  renderResult({ account_name: value });

  expect(screen.getByText(label)).toBeVisible();
  expect(screen.getByRole("button", { name: "关闭任务结果" })).toBeEnabled();
});

it("任务结果字段名与继承属性同名时保留可读字段名", () => {
  renderResult(JSON.parse('{"constructor":"账号一","__proto__":"账号二"}'));

  expect(screen.getByText("Constructor")).toBeVisible();
  expect(screen.getByText("Proto")).toBeVisible();
});

it("任务结果含已知状态和字段时继续显示中文标签", () => {
  renderResult({ status: "auth_verified" });

  expect(screen.getByText("状态")).toBeVisible();
  expect(screen.getByText("鉴权复核通过")).toBeVisible();
});
