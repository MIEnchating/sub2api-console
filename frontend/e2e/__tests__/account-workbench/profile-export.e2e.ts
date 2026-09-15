import { expect, test } from "@playwright/test";
import type { Task, WorkbenchLoginProfile } from "../../../src/api";
import { installWorkbenchFixture } from "./fixture";

test("登录资料私有导出按范围确认，手机桌面区分产物类型并支持删除确认", async ({
  page,
  colorScheme,
}) => {
  await page.addInitScript(
    (theme) => localStorage.setItem("sub2api-console-theme", theme ?? "light"),
    colorScheme,
  );
  await installWorkbenchFixture(page);
  const profile: WorkbenchLoginProfile = {
    id: "profile-a",
    account_id: "42",
    user_id: "user-a",
    workspace_id: "workspace-a",
    email: "operator-" + "long-workspace".repeat(12) + "@example.test",
    revision: 5,
    updated_at: "2026-09-14T00:00:00Z",
    has_password: true,
    has_totp: true,
    has_proxy: true,
  };
  const previews: unknown[] = [];
  const exports: unknown[] = [];
  let removed = false;
  const task: Task = {
    id: "profile-export-task",
    skill: "account-workbench",
    operation: "account-workbench-profile-export",
    status: "queued",
    progress: 0,
    message: "等待生成登录资料文件",
    result: {},
    created_at: "2026-09-14T00:00:00Z",
    updated_at: "2026-09-14T00:00:00Z",
  };
  await page.route("**/api/accounts", (route) => route.fulfill({ json: [] }));
  await page.route("**/api/account-workbench/login-profiles", (route) =>
    route.fulfill({ json: [profile] }),
  );
  await page.route("**/api/account-workbench/exports/profiles/preview", async (route) => {
    previews.push(route.request().postDataJSON() as unknown);
    await route.fulfill({
      json: {
        id: "profile-export-preview",
        kind: "login-profiles",
        target: "https://sub2api.example.test",
        expires_at: new Date(Date.now() + 600000).toISOString(),
        items: [profile],
      },
    });
  });
  await page.route("**/api/account-workbench/exports/profiles", async (route) => {
    exports.push(route.request().postDataJSON() as unknown);
    await route.fulfill({ json: task });
  });
  await page.route("**/api/tasks/profile-export-task", (route) => route.fulfill({ json: task }));
  await page.route("**/api/account-workbench/exports", (route) =>
    route.fulfill({
      json: removed
        ? []
        : [
            {
              id: "profile-private-file",
              kind: "login-profiles",
              count: 1,
              created_at: "2026-09-14T00:00:00Z",
              expires_at: "2026-09-15T00:00:00Z",
            },
          ],
    }),
  );
  await page.route("**/api/account-workbench/exports/profile-private-file", async (route) => {
    removed = true;
    await route.fulfill({ json: { deleted: true } });
  });
  await page.goto("/account-workbench");
  await page.getByRole("tab", { name: "授权登录", exact: true }).click();
  await page.getByRole("tab", { name: "登录资料", exact: true }).click();
  await page.getByRole("checkbox", { name: `选择 ${profile.email}（ID 42）` }).check();
  const open = page.getByRole("button", { name: "私有导出资料（1）" });
  await open.focus();
  await page.keyboard.press("Enter");
  const dialog = page.getByRole("dialog", { name: "确认导出登录资料" });
  await expect(dialog.getByRole("list", { name: "资料导出范围" })).toContainText("版本：5");
  expect(previews).toEqual([{ items: [{ id: "profile-a", revision: 5 }] }]);
  expect(exports).toHaveLength(0);
  await expect(dialog.getByRole("button", { name: "确认生成资料文件" })).toBeInViewport();
  await expect(dialog.getByRole("button", { name: "返回" })).toBeInViewport();
  expect(await dialog.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  await page.screenshot({
    path: test.info().outputPath("profile-export-confirm.png"),
    animations: "disabled",
  });
  await dialog.getByRole("button", { name: "确认生成资料文件" }).click();
  await expect(page.getByRole("status", { name: "等待生成登录资料文件" })).toBeVisible();
  expect(exports).toEqual([{ preview_id: "profile-export-preview", confirmed: true }]);
  await page.getByRole("tab", { name: "私有导出", exact: true }).click();
  const artifacts = page.getByRole("region", { name: "私有导出文件" });
  await expect(artifacts).toContainText("登录资料 1 份");
  await expect(artifacts.getByRole("link")).toHaveCount(0);
  await artifacts.getByRole("button", { name: "删除私有文件 profile-private-file" }).click();
  expect(removed).toBe(false);
  await page
    .getByRole("dialog", { name: "删除私有文件" })
    .getByRole("button", { name: "删除文件" })
    .click();
  await expect(artifacts).toContainText("暂无私有导出文件");
});
