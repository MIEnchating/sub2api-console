import { expect, test } from "@playwright/test";
import { installWorkbenchFixture } from "./fixture";

test("安全设置长账号范围在桌面手机可确认并进入当前账号验证", async ({ page, colorScheme }) => {
  await page.addInitScript(
    (theme) => localStorage.setItem("sub2api-console-theme", theme ?? "light"),
    colorScheme,
  );
  await installWorkbenchFixture(page);
  const name = "团队安全账号_" + "LongAccountName".repeat(8);
  const writes: unknown[] = [];
  await page.route("**/api/accounts", (route) =>
    route.fulfill({ json: [{ id: "42", name, platform: "openai", account_type: "oauth" }] }),
  );
  await page.route("**/api/account-workbench/security-batches**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (route.request().method() === "DELETE") return route.fulfill({ json: { cancelled: true } });
    if (route.request().method() === "POST") writes.push(route.request().postDataJSON());
    const items = [
      {
        index: 0,
        account_id: "42",
        user_id: "user-42",
        email: "owner@example.com",
        status: "queued",
        message: "等待验证",
      },
    ];
    if (path.endsWith("/preview"))
      return route.fulfill({
        json: {
          id: "scope-1",
          target: "https://sub2api.example",
          operation: "totp",
          expires_at: new Date(Date.now() + 600000).toISOString(),
          items,
          errors: [],
        },
      });
    return route.fulfill({
      json: {
        id: "batch-1",
        task_id: "batch-1",
        operation: "totp",
        status: "running",
        message: "等待当前账号验证",
        current_security_id: "security-1",
        completed: 0,
        succeeded: 0,
        expires_at: new Date(Date.now() + 7200000).toISOString(),
        items,
      },
    });
  });
  await page.route("**/api/account-workbench/security/security-1", (route) =>
    route.fulfill({
      json: {
        id: "security-1",
        task_id: "security-1",
        account_id: "42",
        operation: "totp",
        email: "owner@example.com",
        status: "waiting",
        message: "请完成官方登录后继续",
        width: 1100,
        height: 760,
        expires_at: new Date(Date.now() + 900000).toISOString(),
      },
    }),
  );
  await page.goto("/account-workbench");
  await page.getByRole("tab", { name: "账号安全", exact: true }).click();
  await page.getByRole("tab", { name: "批量账号", exact: true }).click();
  await page.getByRole("checkbox", { name: `${name}（ID 42）` }).check();
  const form = page.getByRole("form", { name: "批量安全设置范围" });
  expect(await form.evaluate((node) => node.scrollWidth <= node.clientWidth)).toBe(true);
  await page.getByRole("button", { name: "预览批量安全操作" }).click();
  const preview = page.getByRole("region", { name: "批量安全操作预览" });
  await expect(preview).toBeVisible();
  expect(writes).toHaveLength(1);
  await page.screenshot({
    path: test.info().outputPath("security-batch-confirmation.png"),
    animations: "disabled",
  });
  await page.getByRole("button", { name: "确认并执行批量安全操作" }).click();
  await expect(page.getByRole("button", { name: "验证完成，继续当前账号" })).toBeEnabled();
  await expect(page.getByRole("region", { name: "当前账号安全验证" })).toContainText(
    "owner@example.com",
  );
  expect(writes[1]).toEqual({ preview_id: "scope-1", confirmed: true });
});
