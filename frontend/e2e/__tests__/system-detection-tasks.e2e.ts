import { expect, test } from "@playwright/test";
import type { Task, TaskSummary } from "../../src/api";
import { account } from "../../src/features/accounts/__tests__/fixtures";
import { pageFixtures } from "./fixtures/page-shell";

for (const operation of ["account-model-animation", "managed-model-detection"]) {
  test(`${operation} 在系统信息持续显示进度，刷新后保留，完成进入历史并查看检测详情`, async ({
    page,
  }) => {
    let task: Task = {
      id: "detection-background",
      skill: "sub2api-model-animation",
      operation,
      status: "running",
      progress: 40,
      message: "已完成 2/5 项检测",
      created_at: "2026-09-24T00:00:00Z",
      updated_at: "2026-09-24T00:01:00Z",
      result: {
        account_ids: ["41"],
        completed: 2,
        total: 5,
        configuration: { animation: true, precheck: false, terminal: false },
        animations: [],
      },
    };
    await page.route("**/api/**", async (route) => {
      const path = new URL(route.request().url()).pathname;
      const summary: TaskSummary = {
        id: task.id,
        skill: task.skill,
        operation: task.operation,
        status: task.status,
        progress: task.progress,
        message: task.message,
        created_at: task.created_at,
        updated_at: task.updated_at,
        system_info: true,
      };
      const fixtures: Record<string, unknown> = {
        ...pageFixtures,
        "/api/setup/status": { initialized: true, configuration_errors: [] },
        "/api/auth/session": { authenticated: true, username: "后台检测测试" },
        "/api/accounts": [{ ...account, id: "41", name: "动画账号" }],
        "/api/groups": [],
        "/api/tasks": [summary],
        "/api/tasks/detection-background": task,
        "/api/system/metrics": {
          cpu: { usage_percent: 10, logical_cores: 4 },
          memory: { usage_percent: 20 },
          disk: { usage_percent: 30 },
        },
      };
      if (path.endsWith("/events"))
        await route.fulfill({ contentType: "text/event-stream", body: ": isolated fixture\n\n" });
      else if (path in fixtures) await route.fulfill({ json: fixtures[path] });
      else await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
    });
    await page.goto("/system-info");
    await expect(
      page
        .getByText(operation === "managed-model-detection" ? "分组检测任务" : "动画检测", {
          exact: true,
        })
        .last(),
    ).toBeVisible();
    const progress = page.getByRole("progressbar", {
      name: "detection-background 任务进度",
      exact: true,
    });
    await expect(progress).toHaveAttribute("aria-valuenow", "40");
    task = {
      ...task,
      progress: 60,
      message: "已完成 3/5 项检测",
      result: { ...task.result, completed: 3 },
    };
    await expect(progress).toHaveAttribute("aria-valuenow", "60");
    await page.reload();
    await expect(progress).toHaveAttribute("aria-valuenow", "60");
    await page.getByRole("button", { name: "查看任务", exact: true }).click();
    const detail = page.getByRole("dialog", { name: "检测任务运行详情", exact: true });
    await expect(detail).toBeVisible();
    await expect(detail.getByText("已完成 3/5 项检测", { exact: true })).toBeVisible();
    await expect(detail.getByRole("button", { name: "取消任务", exact: true })).toBeEnabled();
    await expect(detail.getByText("此任务没有可展示的账号探活明细。")).toHaveCount(0);
    await page.keyboard.press("Escape");
    task = {
      ...task,
      status: "succeeded",
      progress: 100,
      message: "检测任务完成",
      result: { ...task.result, completed: 5 },
    };
    await expect(page.getByRole("tab", { name: "历史任务 1" })).toBeVisible();
    await page.getByRole("tab", { name: "历史任务 1" }).click();
    await expect(page.getByText("检测任务完成", { exact: true })).toBeVisible();
    await expect(progress).toHaveAttribute("aria-valuenow", "100");
  });
}
