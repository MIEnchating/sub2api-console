import { expect, test } from "@playwright/test";
import { installWorkbenchFixture } from "./fixture";

test("授权结果安全设置确认官方身份并保留原授权，长身份在手机和桌面可完整操作", async ({
  page,
  colorScheme,
}) => {
  await page.addInitScript(
    (theme) => localStorage.setItem("sub2api-console-theme", theme ?? "light"),
    colorScheme,
  );
  const fixture = await installWorkbenchFixture(page);
  const starts: unknown[] = [];
  const deleted: string[] = [];
  const source = {
    source_oauth_id: "oauth-fixture",
    scope: "managed",
    email: "owner-" + "long-account".repeat(12) + "@example.test",
    user_id: "user-" + "stable-user".repeat(20),
    workspace_id: "workspace-" + "stable-workspace".repeat(20),
    expires_at: new Date(Date.now() + 600000).toISOString(),
  };
  const session = {
    id: "source-security",
    task_id: "source-security",
    source_oauth_id: source.source_oauth_id,
    scope: source.scope,
    email: source.email,
    operation: "totp",
    status: "succeeded",
    message: "双重验证设置完成",
    artifact_id: "private-security-result",
    expires_at: source.expires_at,
    width: 1100,
    height: 760,
  };
  await page.route("**/api/account-workbench/oauth/oauth-fixture/security-source", (route) =>
    route.fulfill({ json: source }),
  );
  await page.route("**/api/account-workbench/security", (route) => {
    starts.push(route.request().postDataJSON() as unknown);
    return route.fulfill({ json: session });
  });
  await page.route("**/api/account-workbench/security/source-security", (route) => {
    if (route.request().method() === "DELETE") deleted.push("source-security");
    return route.fulfill({
      json: route.request().method() === "DELETE" ? { cancelled: true } : session,
    });
  });
  await page.goto("/account-workbench");
  await page.getByRole("tab", { name: "授权登录", exact: true }).click();
  await page.getByRole("button", { name: "开始授权登录" }).click();
  await page.getByRole("button", { name: "登录完成，验证授权" }).click();
  await page.getByRole("button", { name: "设置账号安全" }).click();
  const dialog = page.getByRole("dialog", { name: "授权账号安全设置" });
  await expect(dialog.getByText(source.email)).toBeVisible();
  await expect(dialog.getByRole("combobox", { name: "安全设置账号" })).toHaveCount(0);
  await dialog.getByRole("button", { name: "查看操作范围" }).click();
  const confirmation = dialog.getByRole("region", { name: "确认安全操作" });
  await expect(confirmation).toContainText(source.user_id);
  expect(starts).toEqual([]);
  const submit = dialog.getByRole("button", { name: "确认并开始安全设置" });
  await submit.scrollIntoViewIfNeeded();
  await expect(submit).toBeInViewport();
  expect(await dialog.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  await expect(dialog.getByRole("button", { name: "返回授权结果" })).toBeInViewport();
  await page.screenshot({
    path: test.info().outputPath("source-security-confirm.png"),
    animations: "disabled",
  });
  await submit.click();
  await expect(dialog.getByText("私有结果 ID：private-security-result")).toBeVisible();
  expect(starts).toEqual([
    { source_oauth_id: "oauth-fixture", scope: "managed", operation: "totp", confirmed: true },
  ]);
  await dialog.getByRole("button", { name: "返回授权结果" }).click();
  await expect(page.getByRole("button", { name: "预览授权账号" })).toBeVisible();
  await expect.poll(() => deleted).toEqual(["source-security"]);
  expect(fixture.cancelled).toEqual([]);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
});
