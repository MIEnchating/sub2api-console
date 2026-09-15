import { expect, test } from "@playwright/test";
import type { WorkbenchPendingUpload } from "../../../src/api";
import { installWorkbenchFixture } from "./fixture";

test("维护待上传列表长邮箱在桌面手机可读且来源任务仅执行读取", async ({ page, colorScheme }) => {
  await page.addInitScript(
    (theme) => localStorage.setItem("sub2api-console-theme", theme ?? "light"),
    colorScheme,
  );
  await installWorkbenchFixture(page);
  const uploads: WorkbenchPendingUpload[] = [
    {
      id: "pending-upload:0",
      source_task_id: "pending-upload",
      account_id: "101",
      email: `pending-${"workspace".repeat(15)}@example.test`,
      status: "cooldown",
      message: "授权已完成，等待冷却后继续原上传",
      next_retry_at: "2026-09-15T12:30:00Z",
      expires_at: "2026-09-16T12:00:00Z",
    },
    {
      id: "review-upload:0",
      source_task_id: "review-upload",
      account_id: "102",
      email: "review@example.test",
      status: "review",
      message: "原凭据写入结果不明且线上尚未确认新凭据，请人工核对",
      expires_at: "2026-09-16T12:00:00Z",
    },
  ];
  const writes: string[] = [];
  await page.route("**/api/account-workbench/maintenance", (route) => {
    if (route.request().method() !== "GET") writes.push(route.request().method());
    return route.fulfill({
      json: {
        enabled: false,
        reauthorize_with_profiles: false,
        revision: 2,
        interval_minutes: 5,
        cooldown_minutes: 10,
        group_ids: [],
        check_after_repair: false,
        model: "test-model",
        pending_uploads: uploads,
      },
    });
  });
  await page.route("**/api/tasks/pending-upload", (route) => {
    if (route.request().method() !== "GET") writes.push(route.request().method());
    return route.fulfill({
      json: {
        id: "pending-upload",
        skill: "account-workbench",
        operation: "account-workbench-retry",
        status: "failed",
        progress: 1,
        message: "凭据上传被拒绝，已保留待上传结果",
        result: {},
        created_at: "2026-09-15T12:00:00Z",
        updated_at: "2026-09-15T12:00:00Z",
      },
    });
  });
  await page.goto("/account-workbench");
  await page.getByRole("tab", { name: "自动维护", exact: true }).click();
  const region = page.getByRole("region", { name: "维护待上传账号" });
  await region.scrollIntoViewIfNeeded();
  await expect(region).toContainText("等待冷却");
  await expect(region).toContainText("待人工核对");
  await expect(region.getByRole("listitem")).toHaveCount(2);
  expect(await region.evaluate((node) => node.scrollWidth <= node.clientWidth)).toBe(true);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(
    true,
  );
  await page.screenshot({
    path: test.info().outputPath("maintenance-pending-uploads.png"),
    animations: "disabled",
  });
  const source = region.getByRole("button", { name: "查看账号 101 的来源任务" });
  await source.focus();
  await page.keyboard.press("Enter");
  const dialog = page.getByRole("dialog", { name: "待上传来源任务" });
  await expect(dialog).toContainText("凭据上传被拒绝，已保留待上传结果");
  await expect(dialog.getByRole("button", { name: "关闭", exact: true })).toBeInViewport();
  expect(await dialog.evaluate((node) => node.scrollWidth <= node.clientWidth)).toBe(true);
  await page.screenshot({
    path: test.info().outputPath("maintenance-upload-source.png"),
    animations: "disabled",
  });
  await page.keyboard.press("Escape");
  await expect(dialog).not.toBeVisible();
  expect(writes).toEqual([]);
});
