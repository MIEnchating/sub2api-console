import { QueryClient } from "@tanstack/react-query";
import { afterEach, expect, it } from "vitest";

import type { AccountStatus, Task } from "@/api";
import { account, task } from "@/features/accounts/__tests__/fixtures";
import { applyManualPriorityTaskResult } from "../manual-priority-task";

const clients: QueryClient[] = [];
afterEach(() => clients.splice(0).forEach((client) => client.clear()));

function setup(priority: number | null) {
  const client = new QueryClient();
  clients.push(client);
  const current = { ...account, manual_priority: priority, manual_sync_balance_multiplier: true };
  const other = { ...account, id: "42", manual_priority: 2 };
  client.setQueryData(["accounts"], [current, other]);
  client.setQueryData(["account-detail", "41"], { ...current, notes: "保留详情字段" });
  const completed: Task = {
    ...task("manual-41"),
    result: {
      account_id: "41",
      manual_priority: 3,
      readback_confirmed: true,
      sync_balance_multiplier: false,
      after: { priority: 3, load_factor: 100, concurrency: 100, schedulable: true },
    },
  };
  return { client, completed, current, other };
}

it("设置任务读回确认后，同步目标账号列表和详情中的实际参数", async () => {
  const view = setup(null);
  await applyManualPriorityTaskResult(view.client, view.completed, "41");
  const updated = view.client.getQueryData<AccountStatus[]>(["accounts"])!;
  expect(updated[0]).toMatchObject({
    manual_priority: 3,
    manual_sync_balance_multiplier: false,
    priority: 3,
    load_factor: "100",
    concurrency: 100,
    schedulable: true,
  });
  expect(updated[1]).toEqual(view.other);
  expect(view.client.getQueryData(["account-detail", "41"])).toMatchObject({
    ...updated[0],
    notes: "保留详情字段",
  });
});

it("取消任务确认后，清除手动控制并使用管理平台实际恢复值", async () => {
  const view = setup(3);
  view.completed.result = {
    account_id: "41",
    readback_confirmed: true,
    manual_priority: null,
    after: { priority: 50, load_factor: null, concurrency: 8, schedulable: false },
  };
  await applyManualPriorityTaskResult(view.client, view.completed, "41");
  expect(view.client.getQueryData<AccountStatus[]>(["accounts"])?.[0]).toMatchObject({
    manual_priority: null,
    manual_sync_balance_multiplier: false,
    priority: 50,
    load_factor: null,
    concurrency: 8,
    schedulable: false,
  });
});

it.each<Task["status"]>(["queued", "running", "waiting_input", "failed", "partial", "cancelled"])(
  "任务状态为 %s 时保留账号原状态，不把提交值当成成功结果",
  async (status) => {
    const view = setup(null);
    view.completed.status = status;
    await applyManualPriorityTaskResult(view.client, view.completed, "41");
    expect(view.client.getQueryData(["accounts"])).toEqual([view.current, view.other]);
  },
);

it.each([
  {
    reason: "账号不匹配",
    result: { account_id: "42", manual_priority: 3, readback_confirmed: true },
  },
  {
    reason: "未确认读回",
    result: { account_id: "41", manual_priority: 3, readback_confirmed: false },
  },
  { reason: "历史结果缺少字段", result: { account_id: "41", readback_confirmed: true } },
  {
    reason: "位置无效",
    result: { account_id: "41", manual_priority: -1, readback_confirmed: true },
  },
])("$reason 时保留缓存，交给后续账号刷新核实", async (scenario) => {
  const view = setup(null);
  view.completed.result = scenario.result;
  await applyManualPriorityTaskResult(view.client, view.completed, "41");
  expect(view.client.getQueryData(["accounts"])).toEqual([view.current, view.other]);
});

it("成功任务未提供完整账号资料时，不凭任务结果创建缺失的列表或详情", async () => {
  const view = setup(null);
  view.client.clear();
  await applyManualPriorityTaskResult(view.client, view.completed, "41");
  expect(view.client.getQueryData(["accounts"])).toBeUndefined();
  expect(view.client.getQueryData(["account-detail", "41"])).toBeUndefined();
});
