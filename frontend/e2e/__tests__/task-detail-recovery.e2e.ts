import { expect, test } from "@playwright/test";
import type { Task, TaskSummary } from "../../src/api";
import { pageFixtures } from "./fixtures/page-shell";

for (const linked of [false, true]) {
  test(`关联正式任务=${linked}，缓存详情刷新失败后键盘重试且保留历史结果`, async ({
    page,
    colorScheme,
  }) => {
    await page.clock.install();
    await page.addInitScript(
      (theme) => localStorage.setItem("sub2api-console-theme", theme ?? "light"),
      colorScheme,
    );
    let failed = false;
    let recovered = false;
    let directoryReads = 0;
    const detail: Task = {
      id: "execution",
      operation: linked ? "account-model-automation" : "active-probe",
      skill: "console",
      status: "succeeded",
      progress: 100,
      message: "已保存的任务结果",
      result: linked
        ? { dry_run: false, result_version: 2, items: [] }
        : {
            results: [
              { account_id: "41", result: "通过" },
              { account_id: "42", account_name: "执行时名称", result: "通过" },
            ],
          },
      display_account_names: { "41": "当前账号名称", "42": "后来修改的名称" },
      created_at: "2026-10-10T00:00:00Z",
      updated_at: "2026-10-10T00:00:01Z",
    };
    const summary: TaskSummary = {
      ...detail,
      id: linked ? "preview" : detail.id,
      system_info: true,
    };
    await page.route("**/api/**", async (route) => {
      const path = new URL(route.request().url()).pathname;
      if (path === "/api/accounts") directoryReads++;
      if (path === "/api/tasks/execution")
        return route.fulfill(
          failed
            ? { status: 503, json: { detail: "任务详情暂不可读" } }
            : {
                json: { ...detail, message: recovered ? "重试后已更新的任务结果" : detail.message },
              },
        );
      const fixtures: Record<string, unknown> = {
        ...pageFixtures,
        "/api/setup/status": { initialized: true, configuration_errors: [] },
        "/api/auth/session": { authenticated: true, username: "任务详情测试" },
        "/api/tasks": [summary],
        "/api/tasks/preview": {
          ...summary,
          result: { dry_run: true, execution_task_id: detail.id },
        },
        "/api/dictionaries": { items: [] },
        "/api/system/metrics": {
          cpu: { usage_percent: 10, logical_cores: 4 },
          memory: { usage_percent: 20, used_bytes: 1024, total_bytes: 4096 },
          disk: { usage_percent: 30, used_bytes: 1024, total_bytes: 4096 },
        },
        "/api/inspection/automation": {
          enabled: false,
          running: false,
          traffic_collection: { enabled: false },
        },
      };
      if (path.endsWith("/events"))
        return route.fulfill({ contentType: "text/event-stream", body: ": isolated\n\n" });
      if (route.request().method() === "GET" && path in fixtures)
        return route.fulfill({ json: fixtures[path] });
      return route.fulfill({ status: 404, json: { detail: "未配置的隔离接口" } });
    });
    await page.goto("/system-info");
    await page.getByRole("tab", { name: "历史任务 1" }).click();
    const opener = page.getByRole("button", { name: "查看任务", exact: true });
    await opener.click();
    const dialog = page.getByRole("dialog", { name: "任务详情", exact: true });
    const resultScope = dialog;
    await expect(resultScope.getByText(detail.message, { exact: true })).toBeVisible();
    if (!linked) {
      await expect(dialog.getByText("当前账号名称", { exact: true })).toBeVisible();
      await expect(dialog.getByText("执行时名称", { exact: true })).toBeVisible();
      await expect(dialog.getByText("后来修改的名称", { exact: true })).toHaveCount(0);
    }
    await page.keyboard.press("Escape");
    await expect(dialog).not.toBeVisible();
    failed = true;
    await page.clock.fastForward(16_000);
    await opener.click();
    const retry = dialog.getByRole("button", { name: "重新读取", exact: true });
    await expect(retry).toBeVisible({ timeout: 15000 });
    await expect(resultScope.getByText(detail.message, { exact: true })).toBeVisible();
    failed = false;
    recovered = true;
    await retry.focus();
    await page.keyboard.press("Enter");
    await expect(resultScope.getByText("重试后已更新的任务结果", { exact: true })).toBeVisible();
    await expect(retry).toHaveCount(0);
    expect(directoryReads).toBe(0);
    expect(await dialog.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(
      true,
    );
    await page.screenshot({ path: test.info().outputPath("task-detail-recovered.png") });
    await page.keyboard.press("Escape");
    await expect(opener).toBeFocused();
  });
}
