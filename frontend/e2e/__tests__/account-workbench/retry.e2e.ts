import { expect, test } from "@playwright/test";
import type { Task, WorkbenchPreview } from "../../../src/api";
import { pageFixtures } from "../fixtures/page-shell";

test("处理记录在桌面和手机可查看只读报告并按原始条目索引重试", async ({ page, colorScheme }) => {
  await page.addInitScript(
    (theme) => localStorage.setItem("sub2api-console-theme", theme ?? "light"),
    colorScheme,
  );
  const requests: Array<{ path: string; body: unknown }> = [];
  const task: Task = {
    id: "original-import",
    skill: "account-workbench",
    operation: "account-workbench-import",
    status: "partial",
    progress: 100,
    message: "存在待核对账号",
    result: {
      items: [
        {
          index: 7,
          name: "隔离账号",
          status: "review",
          account_id: "42",
          report: {
            status: "failed",
            total: 24,
            model: "test-model",
            message: "待核对_" + "workspace".repeat(40),
          },
        },
        { index: 2, name: "完成账号", status: "succeeded", account_id: "43" },
      ],
    },
    created_at: new Date().toISOString(),
    updated_at: new Date().toISOString(),
  };
  const retryTask: Task = {
    ...task,
    id: "retry-task",
    operation: "account-workbench-retry",
    status: "queued",
    progress: 0,
    message: "等待重新处理账号",
    result: {},
  };
  const preview: WorkbenchPreview = {
    id: "retry-preview",
    target: "https://sub2api.example.test",
    expires_at: new Date(Date.now() + 600000).toISOString(),
    check_after_import: true,
    model: "test-model",
    errors: [],
    items: [
      {
        id: "7",
        index: 7,
        name: "隔离账号",
        email: "",
        plan_type: "plus",
        template_id: "",
        template_name: "",
        template_revision: 0,
        group_ids: [],
        duplicate: true,
        action: "check",
        account_id: "42",
      },
    ],
  };
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    requests.push({
      path,
      body: route.request().postData() ? (route.request().postDataJSON() as unknown) : null,
    });
    if (path.endsWith("/events"))
      return route.fulfill({ contentType: "text/event-stream", body: ": fixture\n\n" });
    const responses: Record<string, unknown> = {
      ...pageFixtures,
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "重试集成测试" },
      "/api/inspection/automation": {
        enabled: false,
        running: false,
        traffic_collection: { enabled: false },
      },
      "/api/account-workbench/history": [task],
      "/api/account-workbench/templates": [],
      "/api/account-workbench/retry-preview": preview,
      "/api/account-workbench/import": retryTask,
      "/api/tasks/original-import": task,
      "/api/tasks/retry-task": retryTask,
    };
    if (path in responses) return route.fulfill({ json: responses[path] });
    return route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
  });
  await page.goto("/account-workbench");
  await page.getByRole("tab", { name: "处理记录", exact: true }).click();
  await page.getByRole("button", { name: "查看任务 original-import" }).click();
  await page.getByRole("button", { name: "查看 隔离账号 的报告" }).click();
  const dialog = page.getByRole("dialog", { name: "隔离账号处理报告" });
  const report = dialog.getByRole("textbox", { name: "账号处理报告 JSON" });
  await expect(report).toHaveAttribute("aria-readonly", "true");
  await expect(report).toContainText('"total": 24');
  expect(await dialog.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  await page.screenshot({
    path: test.info().outputPath("retry-report.png"),
    animations: "disabled",
  });
  await page.keyboard.press("Escape");
  const selected = page.getByRole("checkbox", { name: "重试第 8 项 隔离账号" });
  await selected.focus();
  await page.keyboard.press("Space");
  await expect(selected).toBeChecked();
  await expect(page.getByRole("checkbox", { name: /^重试第/ })).toHaveCount(1);
  await page.getByRole("button", { name: "重新处理 1 项" }).click();
  await page.getByRole("textbox", { name: "重试检测模型" }).fill("test-model");
  await page.getByRole("button", { name: "预览重新处理范围" }).click();
  await expect(page.getByRole("table", { name: "账号预览" })).toBeVisible();
  expect(requests.find((item) => item.path.endsWith("/retry-preview"))?.body).toEqual({
    task_id: "original-import",
    indexes: [7],
    model: "test-model",
  });
  expect(
    await page
      .locator('[data-slot="page-content"]')
      .evaluate((element) => element.scrollWidth <= element.clientWidth),
  ).toBe(true);
  await page.getByRole("button", { name: "确认重新处理 1 个账号" }).click();
  const confirm = page.getByRole("dialog", { name: "确认重新处理账号" });
  await expect(confirm).toContainText("ID：42");
  await confirm.getByRole("button", { name: "创建重试任务" }).click();
  await expect(page.getByRole("status", { name: "等待重新处理账号" })).toBeVisible();
});
