import { expect, test } from "@playwright/test";
import { installWorkbenchFixture } from "./fixture";

test("RT重新生成在桌面手机确认稳定身份且启动仅发送预览ID", async ({ page }) => {
  await installWorkbenchFixture(page);
  await page.route("**/api/accounts", (route) =>
    route.fulfill({
      json: [{ id: "42", name: "待再生账号", platform: "openai", account_type: "oauth" }],
    }),
  );
  const requests: { path: string; body: unknown }[] = [];
  await page.route("**/api/account-workbench/exports/regenerate/preview", (route) => {
    requests.push({
      path: new URL(route.request().url()).pathname,
      body: route.request().postDataJSON(),
    });
    return route.fulfill({
      json: {
        id: "regen-preview",
        target: "https://target.example.test",
        expires_at: new Date(Date.now() + 600000).toISOString(),
        items: [
          {
            index: 0,
            account_id: "42",
            name: "待再生账号_" + "long-account-name".repeat(40),
            email: "owner@example.com",
            user_id: "user-42",
            workspace_id: "workspace-" + "long-workspace".repeat(30),
            revision: "source-version",
          },
        ],
      },
    });
  });
  const task = {
    id: "regen-task",
    skill: "account-workbench",
    operation: "account-workbench-regenerate",
    status: "queued",
    progress: 0,
    message: "等待核对 RT 再生来源",
    result: {},
    created_at: "2026-09-14T00:00:00Z",
    updated_at: "2026-09-14T00:00:00Z",
  };
  await page.route("**/api/account-workbench/exports/regenerate", (route) => {
    requests.push({
      path: new URL(route.request().url()).pathname,
      body: route.request().postDataJSON(),
    });
    return route.fulfill({ json: task });
  });
  await page.route("**/api/tasks/regen-task", (route) => route.fulfill({ json: task }));
  await page.goto("/account-workbench");
  await page.getByRole("tab", { name: "私有导出", exact: true }).click();
  await page.getByRole("checkbox", { name: "待再生账号（ID 42）" }).check();
  await page.getByRole("button", { name: "重新生成授权文件", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "确认重新生成授权文件" });
  await expect(dialog).toContainText("账号 ID：42；用户：user-42");
  await expect(dialog.getByRole("button", { name: "确认刷新并生成文件" })).toBeInViewport();
  expect(requests).toHaveLength(1);
  expect(await dialog.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.screenshot({
    path: test.info().outputPath("regeneration-confirm.png"),
    animations: "disabled",
  });
  await dialog.getByRole("button", { name: "确认刷新并生成文件" }).click();
  await expect(page.getByRole("status", { name: "等待核对 RT 再生来源" })).toBeVisible();
  expect(requests[1].body).toEqual({ preview_id: "regen-preview", confirmed: true });
});
