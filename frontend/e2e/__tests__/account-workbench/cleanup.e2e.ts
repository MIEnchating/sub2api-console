import { expect, test } from "@playwright/test";
import type { Page } from "@playwright/test";
import type { Task, WorkbenchCleanupPreview, WorkbenchLoginProfile } from "../../../src/api";
import { installWorkbenchFixture } from "./fixture";

const profile: WorkbenchLoginProfile = {
  id: "profile-42",
  account_id: "42",
  user_id: "user-42",
  workspace_id: "workspace-" + "long-identity".repeat(12),
  email: "operator-" + "long-workspace".repeat(12) + "@example.test",
  revision: 7,
  updated_at: "2026-09-14T00:00:00Z",
  has_password: true,
  has_totp: true,
  has_proxy: false,
};

async function installCleanupFixture(
  page: Page,
  blocked: boolean,
): Promise<{ previews: unknown[]; submissions: unknown[]; discarded: string[] }> {
  await installWorkbenchFixture(page);
  const requests = {
    previews: [] as unknown[],
    submissions: [] as unknown[],
    discarded: [] as string[],
  };
  const preview: WorkbenchCleanupPreview = {
    id: "cleanup-preview",
    target: "https://sub2api.example.test",
    expires_at: new Date(Date.now() + 600000).toISOString(),
    profiles: [profile],
    blocked,
    blockers: blocked
      ? [
          {
            task_id: "active-task-" + "long-identity".repeat(12),
            operation: "account-workbench-import",
            status: "running",
            message: "正在导入账号",
          },
        ]
      : [],
    items: [
      {
        id: profile.id,
        kind: "login_profile",
        account_ids: ["42"],
        count: 1,
        action: "delete",
        reason: "所选账号登录资料",
      },
      ...Array.from({ length: 6 }, (_, index) => ({
        id: `execution-${index}-` + "long-identity".repeat(12),
        kind: "execution" as const,
        account_ids: ["42"],
        count: 1,
        action: "delete" as const,
        reason: "执行资料全部属于本次选定账号",
      })),
      {
        id: "mixed-file-" + "long-identity".repeat(12),
        kind: "account_export",
        account_ids: ["42", "43"],
        count: 2,
        action: "retain",
        reason: "文件包含其他账号",
      },
    ],
  };
  const task: Task = {
    id: "cleanup-task",
    skill: "account-workbench",
    operation: "account-workbench-cleanup",
    status: "succeeded",
    progress: 100,
    message: "账号关联资料清理完成",
    result: {},
    created_at: "2026-09-14T00:00:00Z",
    updated_at: "2026-09-14T00:00:00Z",
  };
  await page.route("**/api/accounts", (route) => route.fulfill({ json: [] }));
  await page.route("**/api/account-workbench/login-profiles", (route) =>
    route.fulfill({ json: [profile] }),
  );
  await page.route("**/api/account-workbench/cleanup/preview", async (route) => {
    requests.previews.push(route.request().postDataJSON() as unknown);
    await route.fulfill({ json: preview });
  });
  await page.route("**/api/account-workbench/cleanup/preview/cleanup-preview", async (route) => {
    requests.discarded.push(route.request().method());
    await route.fulfill({ json: { deleted: true } });
  });
  await page.route("**/api/account-workbench/cleanup", async (route) => {
    requests.submissions.push(route.request().postDataJSON() as unknown);
    await route.fulfill({ status: 202, json: task });
  });
  await page.route("**/api/tasks/cleanup-task", (route) => route.fulfill({ json: task }));
  return requests;
}

async function openCleanup(page: Page): Promise<void> {
  await page.goto("/account-workbench");
  await page.getByRole("tab", { name: "授权登录", exact: true }).click();
  await page.getByRole("tab", { name: "登录资料", exact: true }).click();
  await expect(page.getByRole("button", { name: "清理关联资料（0）" })).toBeDisabled();
  await page.getByRole("checkbox", { name: `选择 ${profile.email}（ID 42）` }).check();
  await page.getByRole("button", { name: "清理关联资料（1）" }).focus();
  await page.keyboard.press("Enter");
}

test.beforeEach(async ({ page, colorScheme }) => {
  await page.addInitScript(
    (theme) => localStorage.setItem("sub2api-console-theme", theme ?? "light"),
    colorScheme,
  );
});

test("选中登录资料后展示删除和保留范围，长标识滚动时确认区仍可操作", async ({ page }) => {
  const requests = await installCleanupFixture(page, false);
  await openCleanup(page);
  const dialog = page.getByRole("dialog", { name: "确认清理账号关联资料" });
  const details = dialog.getByRole("list", { name: "关联资料清理明细" });
  await expect(details.getByText("删除", { exact: true })).toHaveCount(7);
  await expect(details.getByText("保留", { exact: true })).toHaveCount(1);
  await expect(dialog.getByText("删除 7 项，保留 1 项")).toBeVisible();
  expect(requests.previews).toEqual([{ items: [{ id: profile.id, revision: 7 }] }]);
  expect(requests.submissions).toHaveLength(0);
  const confirm = dialog.getByRole("button", { name: "确认永久删除所列资料" });
  const close = dialog.getByRole("button", { name: "返回", exact: true });
  await expect(confirm).toBeInViewport();
  await expect(close).toBeInViewport();
  await details.getByText("文件包含其他账号").scrollIntoViewIfNeeded();
  await expect(confirm).toBeInViewport();
  await expect(close).toBeInViewport();
  expect(await dialog.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(
    true,
  );
  await page.screenshot({
    path: test.info().outputPath("cleanup-confirm.png"),
    animations: "disabled",
  });
  await confirm.focus();
  await page.keyboard.press("Enter");
  await expect(dialog).not.toBeVisible();
  await expect(
    page.getByRole("article", { name: "处理任务 cleanup-task" }).getByRole("status"),
  ).toHaveText("账号关联资料清理完成");
  expect(requests.submissions).toEqual([{ preview_id: "cleanup-preview", confirmed: true }]);
});

test("活动任务阻止清理时禁用确认，长任务标识不挤出手机返回入口", async ({ page }) => {
  const requests = await installCleanupFixture(page, true);
  await openCleanup(page);
  const dialog = page.getByRole("dialog", { name: "确认清理账号关联资料" });
  const blockers = dialog.getByRole("region", { name: "阻止清理的活动任务" });
  await expect(blockers).toContainText("正在导入账号");
  await expect(blockers).toContainText("active-task-");
  await expect(dialog.getByRole("button", { name: "确认永久删除所列资料" })).toBeDisabled();
  const close = dialog.getByRole("button", { name: "返回", exact: true });
  await expect(close).toBeInViewport();
  expect(await dialog.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  await blockers.scrollIntoViewIfNeeded();
  await page.screenshot({
    path: test.info().outputPath("cleanup-blocked.png"),
    animations: "disabled",
  });
  await close.focus();
  await page.keyboard.press("Enter");
  await expect(dialog).not.toBeVisible();
  await expect.poll(() => requests.discarded).toEqual(["DELETE"]);
  expect(requests.submissions).toHaveLength(0);
});
