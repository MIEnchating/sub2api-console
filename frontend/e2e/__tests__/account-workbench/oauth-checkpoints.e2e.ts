import { expect, test } from "@playwright/test";
import type { WorkbenchOAuthCheckpoint, WorkbenchOAuthSession } from "../../../src/api";
import { installWorkbenchFixture } from "./fixture";

test("暂停授权确认后可人工恢复，检查点长标识在手机和桌面不溢出", async ({ page, colorScheme }) => {
  await page.addInitScript(
    (theme) => localStorage.setItem("sub2api-console-theme", theme ?? "light"),
    colorScheme,
  );
  const fixture = await installWorkbenchFixture(page);
  const saved: unknown[] = [];
  const restored: unknown[] = [];
  let bitmap = "";
  const checkpoint: WorkbenchOAuthCheckpoint = {
    id: "checkpoint-" + "long-id".repeat(28),
    source_task_id: "oauth-task-" + "original-task".repeat(25),
    status: "ready",
    revision: 3,
    created_at: new Date().toISOString(),
    expires_at: new Date(Date.now() + 600000).toISOString(),
    can_restore: true,
  };
  await page.route("**/api/account-workbench/oauth/oauth-fixture/checkpoint", async (route) => {
    saved.push(route.request().postDataJSON() as unknown);
    await route.fulfill({ json: checkpoint });
  });
  await page.route("**/api/account-workbench/oauth-checkpoints", (route) =>
    route.fulfill({
      json: [checkpoint, { ...checkpoint, id: "expired", expires_at: "2020-01-01T00:00:00Z" }],
    }),
  );
  await page.route("**/api/account-workbench/oauth-checkpoints/*/restore", async (route) => {
    restored.push(route.request().postDataJSON() as unknown);
    const session: WorkbenchOAuthSession = {
      id: "restored-session",
      task_id: "recovery-task",
      host: "auth.openai.com",
      status: "waiting",
      message: "已恢复，等待人工继续登录",
      expires_at: checkpoint.expires_at,
      width: 1100,
      height: 760,
      image: bitmap,
    };
    await route.fulfill({ json: session });
  });
  await page.route("**/api/account-workbench/oauth/restored-session", (route) =>
    route.fulfill({
      json: {
        id: "restored-session",
        task_id: "recovery-task",
        host: "auth.openai.com",
        status: "waiting",
        message: "已恢复，等待人工继续登录",
        expires_at: checkpoint.expires_at,
        width: 1100,
        height: 760,
        image: bitmap,
      },
    }),
  );
  await page.goto("/account-workbench");
  await page.getByRole("tab", { name: "授权登录", exact: true }).click();
  await page.getByRole("button", { name: "开始授权登录" }).click();
  const surface = page.getByRole("button", { name: "上游登录页面" });
  await expect(surface).toBeVisible();
  bitmap = (await surface.getByRole("img").getAttribute("src")) ?? "";
  await page.getByRole("button", { name: "暂停并保存授权" }).click();
  const suspension = page.getByRole("dialog", { name: "暂停并保存授权" });
  expect(saved).toHaveLength(0);
  await expect(suspension.getByRole("button", { name: "确认暂停授权" })).toBeInViewport();
  await suspension.getByRole("button", { name: "确认暂停授权" }).click();
  await expect(surface).toHaveCount(0);
  expect(saved).toEqual([{ confirmed: true }]);
  expect(fixture.cancelled).toEqual([]);
  await page.getByRole("button", { name: "已暂停的授权" }).click();
  const list = page.getByRole("dialog", { name: "已暂停的授权" });
  await expect(list.getByRole("button", { name: "恢复授权" }).last()).toBeDisabled();
  expect(await list.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  await expect(list.getByRole("button", { name: "返回" })).toBeInViewport();
  await page.screenshot({
    path: test.info().outputPath("oauth-checkpoints.png"),
    animations: "disabled",
  });
  await list.getByRole("button", { name: "恢复授权" }).first().click();
  const confirmation = page.getByRole("dialog", { name: "确认恢复授权" });
  expect(restored).toHaveLength(0);
  await expect(confirmation.getByRole("button", { name: "确认人工恢复" })).toBeInViewport();
  await confirmation.getByRole("button", { name: "确认人工恢复" }).click();
  await expect(surface).toHaveAttribute("aria-disabled", "false");
  await expect(surface.getByRole("img")).toHaveJSProperty("naturalWidth", 1100);
  await expect(page.getByRole("dialog")).toHaveCount(0);
  expect(restored).toEqual([{ revision: 3, confirmed: true }]);
  expect(fixture.inputs).toEqual([]);
  const region = page.getByRole("region", { name: "账号授权登录" });
  expect((await region.boundingBox())?.x).toBeGreaterThanOrEqual(0);
  await page.getByRole("button", { name: "退出登录", exact: true }).focus();
  expect((await region.boundingBox())?.x).toBeGreaterThanOrEqual(0);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.screenshot({
    path: test.info().outputPath("oauth-restored.png"),
    animations: "disabled",
  });
});
