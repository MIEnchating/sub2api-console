import { expect, test } from "@playwright/test";
import { installWorkbenchFixture } from "./fixture";

test("自动维护重新授权在桌面手机展示会话确认并等待人工接管", async ({ page, colorScheme }) => {
  await page.addInitScript(
    (theme) => localStorage.setItem("sub2api-console-theme", theme ?? "light"),
    colorScheme,
  );
  await installWorkbenchFixture(page);
  let attached = false;
  const writes: unknown[] = [];
  await page.route("**/api/account-workbench/maintenance", (route) =>
    route.fulfill({
      json: {
        enabled: true,
        reauthorize_with_profiles: true,
        interval_minutes: 5,
        cooldown_minutes: 10,
        group_ids: ["7"],
        check_after_repair: true,
        model: "test-model",
        revision: 4,
      },
    }),
  );
  await page.route("**/api/account-workbench/maintenance/authorization", (route) => {
    if (route.request().method() === "POST") {
      attached = true;
      writes.push(route.request().postDataJSON());
    }
    return route.fulfill({
      json: { attached, current_reauthorization_id: attached ? "auto-batch" : undefined },
    });
  });
  await page.route("**/api/account-workbench/oauth-batches/auto-batch", (route) =>
    route.fulfill({
      json: {
        id: "auto-batch",
        task_id: "auto-batch",
        status: "running",
        message: "等待当前账号人工验证",
        current_oauth_id: "auto-child",
        available: 0,
        items: [
          {
            index: 0,
            account_id: "42",
            email: "owner@example.com",
            status: "running",
            message: "等待人工验证码",
          },
        ],
        expires_at: new Date(Date.now() + 600000).toISOString(),
      },
    }),
  );
  await page.route("**/api/account-workbench/oauth/auto-child", (route) =>
    route.fulfill({
      json: {
        id: "auto-child",
        task_id: "auto-child",
        host: "auth.openai.com",
        status: "waiting",
        message: "等待人工验证码",
        width: 1100,
        height: 760,
        expires_at: new Date(Date.now() + 600000).toISOString(),
      },
    }),
  );
  await page.goto("/account-workbench");
  await page.getByRole("tab", { name: "自动维护", exact: true }).click();
  await page.getByRole("button", { name: "连接本次登录会话" }).click();
  const dialog = page.getByRole("dialog", { name: "确认连接自动重新授权" });
  await expect(dialog).toContainText("分组 ID 7");
  await expect(dialog.getByRole("button", { name: "确认连接", exact: true })).toBeInViewport();
  expect(await dialog.evaluate((node) => node.scrollWidth <= node.clientWidth)).toBe(true);
  await page.screenshot({
    path: test.info().outputPath("maintenance-authorization-confirm.png"),
    animations: "disabled",
  });
  await dialog.getByRole("button", { name: "确认连接", exact: true }).click();
  await expect(page.getByRole("region", { name: "维护账号重新授权" })).toContainText(
    "owner@example.com",
  );
  expect(writes).toEqual([{ revision: 4, confirmed: true }]);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(
    true,
  );
});
