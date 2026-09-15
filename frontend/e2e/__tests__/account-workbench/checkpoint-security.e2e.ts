import { expect, test } from "@playwright/test";
import { installWorkbenchFixture } from "./fixture";

test("暂停授权转入安全设置，确认官方身份后执行并返回全新授权", async ({ page, colorScheme }) => {
  await page.addInitScript(
    (theme) => localStorage.setItem("sub2api-console-theme", theme ?? "light"),
    colorScheme,
  );
  await installWorkbenchFixture(page);
  const checkpoint = {
    id: "paused-source",
    source_task_id: "old-task",
    scope: "managed",
    revision: 3,
    checkpoint_revision: 5,
    stage: "phone",
    status: "ready",
    can_restore: true,
    expires_at: new Date(Date.now() + 600000).toISOString(),
  };
  const identity = {
    email: "verified-" + "long-account".repeat(12) + "@example.test",
    user_id: "user-" + "long-official-id".repeat(15),
  };
  let status = "waiting",
    confirmed = false;
  const writes: Array<{ path: string; body: unknown }> = [];
  const security = () => ({
    id: "checkpoint-security",
    task_id: "checkpoint-security",
    source_checkpoint_id: checkpoint.id,
    scope: "managed",
    operation: "totp",
    status,
    ...identity,
    identity_confirmed: confirmed,
    expires_at: checkpoint.expires_at,
    width: 1100,
    height: 760,
    message: "请核对官方身份",
    artifact_id: status === "succeeded" ? "private-artifact" : undefined,
  });
  await page.route("**/api/account-workbench/oauth-checkpoints", (route) =>
    route.fulfill({ json: [checkpoint] }),
  );
  await page.route("**/api/account-workbench/oauth-checkpoints/paused-source/security", (route) => {
    writes.push({ path: "transfer", body: route.request().postDataJSON() as unknown });
    return route.fulfill({ json: security() });
  });
  await page.route("**/api/account-workbench/security/checkpoint-security**", (route) => {
    const path = new URL(route.request().url()).pathname;
    if (route.request().method() === "DELETE") return route.fulfill({ json: { cancelled: true } });
    if (route.request().method() === "POST")
      writes.push({ path, body: route.request().postDataJSON() as unknown });
    if (path.endsWith("/confirm-identity")) {
      confirmed = true;
      status = "waiting";
    }
    if (path.endsWith("/continue")) status = confirmed ? "succeeded" : "awaiting_confirmation";
    if (path.endsWith("/oauth"))
      return route.fulfill({
        json: {
          id: "fresh-security-oauth",
          task_id: "fresh-security-oauth",
          scope: "managed",
          host: "auth.openai.com",
          status: "waiting",
          message: "新授权已建立",
          expires_at: checkpoint.expires_at,
          width: 1100,
          height: 760,
        },
      });
    return route.fulfill({
      json: route.request().method() === "POST" ? { accepted: true } : security(),
    });
  });
  await page.route("**/api/account-workbench/oauth/fresh-security-oauth", (route) =>
    route.fulfill({
      json: {
        id: "fresh-security-oauth",
        task_id: "fresh-security-oauth",
        scope: "managed",
        host: "auth.openai.com",
        status: "waiting",
        message: "新授权已建立",
        expires_at: checkpoint.expires_at,
        width: 1100,
        height: 760,
      },
    }),
  );
  await page.goto("/account-workbench");
  await page.getByRole("tab", { name: "授权登录", exact: true }).click();
  await page.getByRole("button", { name: "已暂停的授权" }).click();
  await page.getByRole("button", { name: "转入安全设置" }).click();
  const dialog = page.getByRole("dialog", { name: "授权检查点安全设置" });
  await dialog.getByRole("button", { name: "查看操作范围" }).click();
  await expect(dialog).toContainText("原授权不能再恢复");
  expect(writes).toEqual([]);
  await dialog.getByRole("button", { name: "确认并开始安全设置" }).click();
  expect(writes[0]).toEqual({
    path: "transfer",
    body: {
      scope: "managed",
      revision: 3,
      checkpoint_revision: 5,
      operation: "totp",
      confirmed: true,
    },
  });
  await dialog.getByRole("button", { name: "验证完成，继续" }).click();
  await expect(dialog.getByRole("region", { name: "确认官方账号身份" })).toContainText(
    identity.user_id,
  );
  await expect(dialog.getByRole("button", { name: "验证完成，继续" })).toBeDisabled();
  const confirm = dialog.getByRole("button", { name: "确认此官方账号并继续" });
  await confirm.scrollIntoViewIfNeeded();
  await expect(confirm).toBeInViewport();
  expect(await dialog.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  await page.screenshot({
    path: test.info().outputPath("checkpoint-official-identity.png"),
    animations: "disabled",
  });
  await confirm.click();
  await dialog.getByRole("button", { name: "验证完成，继续" }).click();
  await expect(dialog.getByText("私有结果 ID：private-artifact")).toBeVisible();
  await dialog.getByRole("button", { name: "确认发起新的 OAuth 授权" }).click();
  await expect(dialog).toHaveCount(0);
  await expect(page.getByText("新授权已建立")).toBeVisible();
  expect(writes.find((row) => row.path.endsWith("/confirm-identity"))?.body).toEqual({
    ...identity,
    confirmed: true,
  });
});
