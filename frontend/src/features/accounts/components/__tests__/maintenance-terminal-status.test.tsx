import { render, screen } from "@testing-library/react";
import { expect, it } from "vitest";

import {
  AccountDefaultsRepairTaskStatus,
  AccountMaintenanceTaskStatus,
  AccountRateSyncTaskStatus,
} from "@/App";
import { task } from "../../__tests__/fixtures";

it.each([
  ["倍率同步", AccountRateSyncTaskStatus],
  ["参数修复", AccountDefaultsRepairTaskStatus],
  ["绑定维护", AccountMaintenanceTaskStatus],
])("%s 取消后显示取消原因", (_name, Component) => {
  render(
    <Component task={{ ...task("maintenance", "cancelled"), message: "用户已取消当前维护任务" }} />,
  );
  expect(screen.getByText("用户已取消当前维护任务")).toBeVisible();
});

it("倍率同步已有部分明细但整体失败时仍展示任务失败原因", () => {
  render(
    <AccountRateSyncTaskStatus
      task={{
        ...task("rates", "failed"),
        result: {
          error: "管理目标已变化，后续账号未处理",
          items: [{ account_id: "41", account_name: "已同步账号", status: "已同步" }],
        },
      }}
    />,
  );
  expect(screen.getByText("管理目标已变化，后续账号未处理")).toBeVisible();
  expect(screen.getByText("已同步账号")).toBeVisible();
});
