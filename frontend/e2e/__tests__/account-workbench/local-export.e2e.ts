import { expect, test } from "@playwright/test";
import { installWorkbenchFixture } from "./fixture";

test("初始化可通过键盘选择本地模式并且不提交隐藏的管理凭据", async ({ page, colorScheme }) => {
  await page.addInitScript(
    (theme) => localStorage.setItem("sub2api-console-theme", theme ?? "light"),
    colorScheme,
  );
  await installWorkbenchFixture(page);
  let initialized = false;
  const requests: unknown[] = [];
  await page.route("**/api/setup/status", (route) =>
    route.fulfill({
      json: {
        initialized,
        target_configured: false,
        setup_token_required: false,
        configuration_errors: [],
      },
    }),
  );
  await page.route("**/api/setup/initialize", async (route) => {
    requests.push(route.request().postDataJSON() as unknown);
    initialized = true;
    await route.fulfill({ json: { initialized: true, target_configured: false } });
  });
  await page.goto("/account-workbench");
  await page.getByLabel("控制台账号", { exact: true }).fill("local-operator");
  await page.getByLabel("控制台密码", { exact: true }).fill("isolated-local-password");
  await page.getByLabel("确认控制台密码", { exact: true }).fill("isolated-local-password");
  await page.getByLabel("Admin Base URL", { exact: true }).fill("unfinished-target");
  await page.getByLabel("Admin Key", { exact: true }).fill("isolated-managed-key");
  const local = page.getByRole("checkbox", { name: "仅使用本地账号工作台（不配置线上管理目标）" });
  await local.focus();
  await page.keyboard.press("Space");
  await expect(local).toBeChecked();
  await expect(page.getByLabel("Admin Base URL", { exact: true })).toHaveCount(0);
  await page.getByRole("button", { name: "完成初始化" }).scrollIntoViewIfNeeded();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(
    true,
  );
  await page.screenshot({
    path: test.info().outputPath("local-export-setup.png"),
    animations: "disabled",
  });
  await page.getByRole("button", { name: "完成初始化" }).click();
  await expect(page.getByRole("button", { name: "仅导出 JSON" })).toHaveAttribute(
    "aria-pressed",
    "true",
  );
  await expect(page.getByRole("button", { name: "导入站点" })).toBeDisabled();
  await expect(page.getByRole("combobox", { name: "配置模板" })).toHaveCount(0);
  expect(requests).toEqual([
    {
      username: "local-operator",
      password: "isolated-local-password",
      local_export_only: true,
      admin_base_url: "",
      admin_key: "",
    },
  ]);
});
