import { expect, test } from "@playwright/test";
import { installWorkbenchFixture } from "./fixture";

test("批次恢复展示账号范围，接回当前任务不重复启动且可主动结束", async ({ page, colorScheme }) => {
  await page.addInitScript(
    (theme) => localStorage.setItem("sub2api-console-theme", theme ?? "light"),
    colorScheme,
  );
  await installWorkbenchFixture(page);
  const resumed: unknown[] = [];
  const cancelled: string[] = [];
  const expiry = new Date(Date.now() + 600000).toISOString();
  const taskID = "batch-" + "long-task-id".repeat(20);
  const email = "account-" + "long-name".repeat(12) + "@example.com";
  const queue = {
    id: "private-queue",
    kind: "oauth-batch",
    scope: "managed",
    active: true,
    task_id: taskID,
    status: "running",
    revision: 7,
    expires_at: expiry,
    can_resume: true,
    pending: 1,
    succeeded: 1,
    review: 0,
    items: [
      { index: 0, email, workspace_id: "workspace-" + "stable-id".repeat(15), status: "succeeded" },
      { index: 1, email: "pending@example.com", status: "queued" },
    ],
  };
  const batch = {
    id: taskID,
    task_id: taskID,
    status: "running",
    message: "等待下一个授权账号",
    recovery_enabled: true,
    recovery_id: queue.id,
    expires_at: expiry,
    available: 1,
    items: queue.items.map((item) => ({
      ...item,
      has_password: false,
      has_totp: false,
      message: "等待处理",
    })),
  };
  await page.route("**/api/account-workbench/queue-recoveries", (route) =>
    route.fulfill({ json: [queue] }),
  );
  await page.route("**/api/account-workbench/queue-recoveries/private-queue/oauth", (route) => {
    resumed.push(route.request().postDataJSON() as unknown);
    return route.fulfill({ json: batch });
  });
  await page.route("**/api/account-workbench/oauth-batches/*", (route) => {
    if (route.request().method() === "DELETE") cancelled.push(route.request().url());
    return route.fulfill({
      json: route.request().method() === "DELETE" ? { cancelled: true } : batch,
    });
  });
  await page.goto("/account-workbench");
  await page.getByRole("tab", { name: "授权登录", exact: true }).click();
  await page.getByRole("tab", { name: "批量授权", exact: true }).click();
  await page.getByRole("button", { name: "恢复已保存批次" }).click();
  const list = page.getByRole("dialog", { name: "恢复已保存批次" });
  await expect(list.getByText("待执行 1 项；已成功 1 项；待核对 0 项")).toBeVisible();
  await list.getByText("账号范围（2 项）").click();
  await expect(list.getByRole("list", { name: "恢复账号范围" })).toContainText(email);
  await expect(list.getByRole("button", { name: "删除恢复资料" })).toBeDisabled();
  expect(await list.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  await page.screenshot({
    path: test.info().outputPath("queue-scope.png"),
    animations: "disabled",
  });
  await list.getByRole("button", { name: "恢复本批" }).click();
  expect(resumed).toEqual([]);
  await page.getByRole("button", { name: "确认继续本批" }).click();
  await expect(page.getByRole("region", { name: "批量授权进度" })).toContainText(taskID);
  expect(resumed).toEqual([{ scope: "managed", revision: 7, confirmed: true }]);
  await page.getByRole("tab", { name: "单个授权", exact: true }).click();
  expect(cancelled).toEqual([]);
  await page.getByRole("tab", { name: "批量授权", exact: true }).click();
  await page.getByRole("button", { name: "恢复已保存批次" }).click();
  await page.getByRole("button", { name: "恢复本批" }).click();
  await page.getByRole("button", { name: "确认继续本批" }).click();
  await page.getByRole("button", { name: "结束批量授权" }).click();
  await page.getByRole("button", { name: "结束并清除本批" }).click();
  await expect.poll(() => cancelled.length).toBe(1);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
});
